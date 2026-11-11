//go:build darwin || linux

package host

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/emms-garcia/shipq/internal/domain/identity"
)

func profileID(t *testing.T) identity.ID {
	t.Helper()
	id, err := identity.ParseID("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestExclusiveOwnershipAndIdempotentClose(t *testing.T) {
	directory := t.TempDir()
	first, err := AcquireOwnership(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	if second, err := AcquireOwnership(directory); second != nil || !errors.Is(err, ErrProfileOwned) {
		t.Fatalf("second owner accepted: %v, %v", second, err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := AcquireOwnership(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if third, err := AcquireOwnership(directory); third != nil || !errors.Is(err, ErrProfileOwned) {
		t.Fatalf("closing old owner released new owner's lock: %v, %v", third, err)
	}
	info, err := os.Stat(filepath.Join(directory, ownershipName))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("ownership file must remain private and persistent: %v, %v", info, err)
	}
}

func TestProfileInitializationOwnsDatabase(t *testing.T) {
	root := t.TempDir()
	first, err := OpenProfile(context.Background(), root, profileID(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	if first.Database == nil {
		t.Fatal("successful profile has no database")
	}
	otherID, err := identity.ParseID("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")
	if err != nil {
		t.Fatal(err)
	}
	other, err := OpenProfile(context.Background(), root, otherID)
	if err != nil {
		t.Fatalf("independent profile blocked by first profile: %v", err)
	}
	defer other.Close()
	directory := filepath.Join(root, profileID(t).String())
	info, err := os.Stat(directory)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("profile directory not private: %v, %v", info, err)
	}
	if second, err := OpenProfile(context.Background(), root, profileID(t)); second != nil || !errors.Is(err, ErrProfileOwned) {
		t.Fatalf("second host initialized: %v, %v", second, err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := OpenProfile(context.Background(), root, profileID(t))
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if third, err := OpenProfile(context.Background(), root, profileID(t)); third != nil || !errors.Is(err, ErrProfileOwned) {
		t.Fatalf("old profile close bypassed live ownership: %v, %v", third, err)
	}
}

func TestFailedInitializationNeverExposesReadyProfile(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, profileID(t).String())
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	corrupt := []byte("not a SQLite database")
	path := filepath.Join(directory, databaseName)
	if err := os.WriteFile(path, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	if p, err := OpenProfile(context.Background(), root, profileID(t)); p != nil || err == nil {
		t.Fatalf("failed initialization exposed ready profile: %v, %v", p, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, corrupt) {
		t.Fatal("corrupt input silently overwritten")
	}
	lock, err := AcquireOwnership(directory)
	if err != nil {
		t.Fatalf("initialization failure leaked ownership: %v", err)
	}
	defer lock.Close()
	if p, err := OpenProfile(context.Background(), root, profileID(t)); p != nil || !errors.Is(err, ErrProfileOwned) {
		t.Fatalf("migration/open failure bypassed held ownership: %v, %v", p, err)
	}
}

func TestCancelledAndInvalidProfiles(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if p, err := OpenProfile(ctx, t.TempDir(), profileID(t)); p != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled initialization: %v, %v", p, err)
	}
	if p, err := OpenProfile(context.Background(), t.TempDir(), identity.ID{}); p != nil || err == nil {
		t.Fatal("nil profile UUID accepted")
	}
	if p, err := OpenProfile(context.Background(), "", profileID(t)); p != nil || err == nil {
		t.Fatal("empty data root accepted")
	}
	root := t.TempDir()
	path := filepath.Join(root, profileID(t).String())
	if err := os.WriteFile(path, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if p, err := OpenProfile(context.Background(), root, profileID(t)); p != nil || err == nil {
		t.Fatal("file accepted as profile directory")
	}
}

func TestOwnershipRejectsSymlink(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(directory, ownershipName)); err != nil {
		t.Fatal(err)
	}
	if lock, err := AcquireOwnership(directory); lock != nil || err == nil {
		t.Fatal("symlink accepted as ownership file")
	}
	data, _ := os.ReadFile(target)
	if string(data) != "untouched" {
		t.Fatal("lock modified unrelated target")
	}
}

func TestOwnershipHelper(t *testing.T) {
	if os.Getenv("SHIPQ_TEST_OWNERSHIP_HELPER") != "hold" {
		t.Skip("subprocess helper")
	}
	profile, err := OpenProfile(context.Background(), os.Getenv("SHIPQ_TEST_PROFILE_ROOT"), profileID(t))
	if err != nil {
		t.Fatal(err)
	}
	defer profile.Close()
	fmt.Println("ready")
	_, _ = io.Copy(io.Discard, os.Stdin)
}

func TestOwnershipAcrossProcessesAndCrashRecovery(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestOwnershipHelper$")
	child.Env = append(os.Environ(), "SHIPQ_TEST_OWNERSHIP_HELPER=hold", "SHIPQ_TEST_PROFILE_ROOT="+root)
	stdin, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	child.Stderr = &stderr
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stdin.Close(); _ = child.Process.Kill(); _ = child.Wait() })
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || line != "ready\n" {
		_ = child.Process.Kill()
		_ = child.Wait()
		t.Fatalf("child not ready: %q, %v, %s", line, err, stderr.String())
	}
	if p, err := OpenProfile(context.Background(), root, profileID(t)); p != nil || !errors.Is(err, ErrProfileOwned) {
		t.Fatalf("separate process bypassed ownership: %v, %v", p, err)
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := child.Wait(); err == nil {
		t.Fatal("expected abrupt child termination")
	}
	recovered, err := OpenProfile(context.Background(), root, profileID(t))
	if err != nil {
		t.Fatalf("stale lock survived process death: %v", err)
	}
	defer recovered.Close()
}
