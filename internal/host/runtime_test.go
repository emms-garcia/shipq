//go:build darwin || linux

package host

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	v1 "github.com/emms-garcia/shipq/api/local/v1"
	"github.com/emms-garcia/shipq/internal/application/ports"
	"github.com/emms-garcia/shipq/internal/domain/identity"
)

type fixedStatus struct{ status ports.HostStatus }

func (s fixedStatus) ReadStatus(context.Context) (ports.HostStatus, error) { return s.status, nil }

func TestRuntimeOwnsProfileBeforeServingAndRestartsCleanly(t *testing.T) {
	profileID, err := identity.ParseID("22222222-2222-4222-8222-222222222222")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	runtimeDirectory, err := os.MkdirTemp("/tmp", "shipq-runtime-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(runtimeDirectory) })
	config := Config{
		DataRoot: root, RuntimeDirectory: runtimeDirectory, ProfileID: profileID,
		Status: fixedStatus{ports.HostStatus{
			HostVersion: "test", ActiveProfileID: profileID, MonitorState: "unavailable",
			NotificationPermission: "unavailable", CoverageState: "unknown",
		}},
	}
	runtime, err := Start(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Start(context.Background(), config); err == nil {
		t.Fatal("a second runtime acquired the active profile or socket")
	}
	if info, err := os.Stat(filepath.Join(runtimeDirectory, pidName)); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal("runtime did not publish a private lifecycle pid")
	}

	client := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", runtime.SocketPath())
	}}}
	response, err := client.Get("http://shipq.local/v1/status")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var envelope v1.Envelope
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || len(envelope.Data) == 0 {
		t.Fatalf("unexpected status response: %d %#v", response.StatusCode, envelope)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := runtime.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(runtimeDirectory, pidName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("runtime did not remove its lifecycle pid")
	}
	restarted, err := Start(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultConfigPersistsPrivateProfileIdentity(t *testing.T) {
	root := t.TempDir()
	runtimeDirectory, err := os.MkdirTemp("/tmp", "shipq-config-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(runtimeDirectory) })
	t.Setenv("SHIPQ_DATA_ROOT", root)
	t.Setenv("SHIPQ_RUNTIME_DIR", runtimeDirectory)
	t.Setenv("SHIPQ_PROFILE_ID", "")
	t.Setenv("SHIPQ_PLATFORM_HELPER", filepath.Join(t.TempDir(), "ShipqPlatform"))

	first, err := DefaultConfig()
	if err != nil {
		t.Fatal(err)
	}
	second, err := DefaultConfig()
	if err != nil {
		t.Fatal(err)
	}
	if first.ProfileID == (identity.ID{}) || first.ProfileID != second.ProfileID {
		t.Fatal("default profile identity was zero or unstable")
	}
	info, err := os.Stat(filepath.Join(root, activeProfileName))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("active profile file mode = %o", info.Mode().Perm())
	}
}

func TestConfigRejectsUnsafeOrMismatchedStatus(t *testing.T) {
	id, _ := identity.ParseID("22222222-2222-4222-8222-222222222222")
	other, _ := identity.ParseID("33333333-3333-4333-8333-333333333333")
	root := t.TempDir()
	cases := []Config{
		{DataRoot: "relative", RuntimeDirectory: filepath.Join(t.TempDir(), "runtime"), ProfileID: id},
		{DataRoot: root, RuntimeDirectory: "relative", ProfileID: id},
		{DataRoot: root, RuntimeDirectory: filepath.Join(t.TempDir(), "runtime")},
		{DataRoot: root, RuntimeDirectory: filepath.Join(t.TempDir(), "runtime"), ProfileID: id,
			Status: fixedStatus{ports.HostStatus{HostVersion: "test", ActiveProfileID: other}}},
	}
	for index, config := range cases {
		if runtime, err := Start(context.Background(), config); err == nil {
			_ = runtime.Shutdown(context.Background())
			t.Fatalf("unsafe config %d accepted", index)
		}
	}
}
