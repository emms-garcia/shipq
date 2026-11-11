//go:build darwin

package platform_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/emms-garcia/shipq/internal/adapters/macos"
	"github.com/emms-garcia/shipq/internal/domain/identity"
)

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
