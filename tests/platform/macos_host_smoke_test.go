//go:build darwin

package platform_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	v1 "github.com/emms-garcia/shipq/api/local/v1"
	"github.com/google/uuid"

	"github.com/emms-garcia/shipq/internal/adapters/macos"
	"github.com/emms-garcia/shipq/internal/domain/identity"
)

func TestComposedBundleHostRoundTrip(t *testing.T) {
	bundle := os.Getenv("SHIPQ_COMPOSED_APP")
	if bundle == "" {
		t.Skip("set SHIPQ_COMPOSED_APP to an explicit built app bundle")
	}
	if !filepath.IsAbs(bundle) || filepath.Ext(bundle) != ".app" {
		t.Fatal("SHIPQ_COMPOSED_APP must be an absolute app bundle")
	}
	if output, err := exec.Command("codesign", "--verify", "--deep", "--strict", bundle).CombinedOutput(); err != nil {
		t.Fatalf("bundle signature verification failed: %s", output)
	}
	hostPath := filepath.Join(bundle, "Contents", "MacOS", "shipq-host")
	helperPath := filepath.Join(bundle, "Contents", "Library", "Helpers", "ShipqPlatform.app", "Contents", "MacOS", "ShipqPlatform")
	runtimeDirectory, err := os.MkdirTemp("/tmp", "shipq-bundle-smoke-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(runtimeDirectory)
	dataRoot := filepath.Join(t.TempDir(), "profiles")
	profileID := "55555555-5555-4555-8555-555555555555"
	start := func() *exec.Cmd {
		command := exec.Command(hostPath)
		command.Env = []string{
			"HOME=" + os.Getenv("HOME"), "LANG=en_US.UTF-8", "PATH=/usr/bin:/bin",
			"SHIPQ_DATA_ROOT=" + dataRoot, "SHIPQ_RUNTIME_DIR=" + runtimeDirectory,
			"SHIPQ_PROFILE_ID=" + profileID, "SHIPQ_PLATFORM_HELPER=" + helperPath,
		}
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		return command
	}
	waitForStatus := func() {
		socketPath := filepath.Join(runtimeDirectory, "api.sock")
		transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
		}}
		defer transport.CloseIdleConnections()
		client := &http.Client{Transport: transport, Timeout: time.Second}
		deadline := time.Now().Add(10 * time.Second)
		for {
			response, err := client.Get("http://shipq.local/v1/status")
			if err == nil {
				var envelope v1.Envelope
				decodeErr := json.NewDecoder(response.Body).Decode(&envelope)
				_ = response.Body.Close()
				if response.StatusCode == http.StatusOK && decodeErr == nil && len(envelope.Data) != 0 {
					return
				}
			}
			if time.Now().After(deadline) {
				t.Fatal("bundled host did not expose a status round trip")
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	stop := func(command *exec.Cmd) {
		if err := command.Process.Signal(os.Interrupt); err != nil {
			t.Fatal(err)
		}
		if err := command.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	first := start()
	waitForStatus()
	stop(first)
	second := start()
	waitForStatus()
	stop(second)
}

func TestSignedPlatformSmoke(t *testing.T) {
	if os.Getenv("SHIPQ_PLATFORM_SMOKE") != "1" {
		t.Skip("opt-in signed helper capability smoke; not T019 lifecycle acceptance")
	}
	bundle := os.Getenv("SHIPQ_SIGNED_APP")
	if !filepath.IsAbs(bundle) || filepath.Ext(bundle) != ".app" {
		t.Fatal("SHIPQ_SIGNED_APP must identify an explicit signed development app bundle")
	}
	helperBundle := filepath.Join(bundle, "Contents", "Library", "Helpers", "ShipqPlatform.app")
	for _, path := range []string{bundle, helperBundle} {
		if exec.Command("codesign", "--verify", "--strict", path).Run() != nil {
			t.Fatal("bundle signature verification failed")
		}
		signature, err := exec.Command("codesign", "--display", "--verbose=4", path).CombinedOutput()
		if err != nil || strings.Contains(string(signature), "Signature=adhoc") ||
			!strings.Contains(string(signature), "TeamIdentifier=") ||
			strings.Contains(string(signature), "TeamIdentifier=not set") {
			t.Fatal("a genuine development signature is required, not ad-hoc signing")
		}
	}
	adapter, err := macos.New(filepath.Join(helperBundle, "Contents", "MacOS", "ShipqPlatform"))
	if err != nil {
		t.Fatal("invalid explicit helper path")
	}
	id := identity.ID(uuid.New())
	ctx := context.Background()
	t.Cleanup(func() {
		if adapter.DeleteCredential(context.Background(), id) != nil {
			t.Error("failed to remove isolated smoke credential")
		}
	})
	value := []byte("shipq-synthetic-platform-smoke")
	if adapter.WriteCredential(ctx, id, value) != nil {
		t.Fatal("isolated Keychain write failed")
	}
	read, err := adapter.ReadCredential(ctx, id)
	if err != nil || string(read) != string(value) {
		t.Fatal("isolated Keychain round trip failed")
	}
	permission, err := adapter.NotificationStatus(ctx)
	if err != nil || permission == "unavailable" {
		t.Fatal("signed helper notification status unavailable")
	}
	if os.Getenv("SHIPQ_PLATFORM_SMOKE_REQUEST_NOTIFICATIONS") == "1" {
		permission, err = adapter.AuthorizeNotifications(ctx, true)
		if err != nil {
			t.Fatal("explicit authorization request failed")
		}
	}
	if os.Getenv("SHIPQ_PLATFORM_SMOKE_SUBMIT_NOTIFICATION") == "1" {
		if permission != "authorized" && permission != "provisional" {
			t.Fatal("notification permission not granted; no submission attempted")
		}
		if adapter.SubmitNotification(ctx, macos.Notification{
			ID: id, Title: "Shipq capability smoke", Body: "Synthetic OS submission, not a PR transition.",
		}) != nil {
			t.Fatal("native notification submission failed")
		}
	}
	// Service registration belongs to the calling desktop app; never install it here.
	state, err := adapter.ServiceStatus(ctx)
	if err != nil || state != "unavailable" {
		t.Fatal("helper must not pretend to own the parent app's service registration")
	}
	t.Log("signed helper capability checks completed; service ownership/lifecycle and visible delivery remain T019")
}
