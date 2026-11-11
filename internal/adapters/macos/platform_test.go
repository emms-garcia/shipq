//go:build darwin

package macos

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/emms-garcia/shipq/internal/domain/identity"
)

func TestMain(m *testing.M) {
	name := filepath.Base(os.Args[0])
	if strings.HasPrefix(name, "ShipqPlatformFixture-") {
		runFixture(strings.TrimPrefix(name, "ShipqPlatformFixture-"))
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func runFixture(mode string) {
	if len(os.Args) != 1 || os.Getenv("SHIPQ_TEST_PRIVATE") != "" {
		os.Exit(2)
	}
	if mode == "hang" {
		time.Sleep(10 * time.Second)
		return
	}
	if mode == "fail" {
		fmt.Fprint(os.Stderr, "synthetic-private: helper crash")
		os.Exit(2)
	}
	if mode == "large" {
		fmt.Print(strings.Repeat("x", 1024*1024+1))
		return
	}
	var request map[string]any
	if json.NewDecoder(os.Stdin).Decode(&request) != nil {
		os.Exit(2)
	}
	version := 1
	if mode == "version" {
		version = 2
	}
	response := map[string]any{"protocol_version": version, "request_id": request["request_id"]}
	data := map[string]any{}
	switch request["operation"] {
	case "credential.read":
		data["state"] = "found"
		data["credential"] = base64.StdEncoding.EncodeToString([]byte("synthetic-private"))
	case "credential.write":
		if request["credential"] != base64.StdEncoding.EncodeToString([]byte("synthetic-private")) {
			os.Exit(2)
		}
		data["state"] = "stored"
	case "credential.delete":
		data["state"] = "deleted"
	case "notification.status", "notification.authorize":
		data["state"] = "authorized"
	case "notification.submit":
		data["state"] = "submitted"
	case "service.status", "service.register":
		data["state"] = "requires_approval"
	case "service.unregister":
		data["state"] = "not_registered"
	default:
		os.Exit(2)
	}
	response["data"] = data
	if mode == "error" {
		delete(response, "data")
		response["error"] = map[string]any{"code": "locked"}
	}
	if mode == "unknown" {
		data["access_token"] = "synthetic-private"
	}
	if mode == "correlation" {
		response["request_id"] = "11111111-1111-4111-8111-111111111111"
	}
	if mode == "both" {
		response["error"] = map[string]any{"code": "failed"}
	}
	if mode == "bad-state" {
		data["state"] = "synthetic-private"
	}
	json.NewEncoder(os.Stdout).Encode(response)
}

func fixturePlatform(t *testing.T, mode string) *Platform {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "ShipqPlatformFixture-"+mode)
	if err := os.Link(executable, path); err != nil {
		t.Fatal(err)
	}
	platform, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	return platform
}

func profileID(t *testing.T) identity.ID {
	t.Helper()
	id, err := identity.ParseID("22222222-2222-4222-8222-222222222222")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func requireFailure(t *testing.T, err error, code string) {
	t.Helper()
	var failure *Failure
	if !errors.As(err, &failure) || failure.Code != code {
		t.Fatalf("expected safe %s failure; got %T", code, err)
	}
	if strings.Contains(err.Error(), "synthetic-private") {
		t.Fatal("failure exposed private text")
	}
}

func TestCredentialsUsePrivatePipes(t *testing.T) {
	t.Setenv("SHIPQ_TEST_PRIVATE", "synthetic-private")
	platform := fixturePlatform(t, "ok")
	ctx := context.Background()
	id := profileID(t)
	if err := platform.WriteCredential(ctx, id, []byte("synthetic-private")); err != nil {
		t.Fatal(err)
	}
	value, err := platform.ReadCredential(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if string(value) != "synthetic-private" {
		t.Fatal("credential did not round trip")
	}
	if err := platform.DeleteCredential(ctx, id); err != nil {
		t.Fatal(err)
	}
}

func TestNativeStatesAndExplicitIntent(t *testing.T) {
	platform := fixturePlatform(t, "ok")
	ctx := context.Background()
	state, err := platform.NotificationStatus(ctx)
	if err != nil || state != "authorized" {
		t.Fatalf("notification status: %q, %v", state, err)
	}
	_, err = platform.AuthorizeNotifications(ctx, false)
	requireFailure(t, err, "denied")
	state, err = platform.AuthorizeNotifications(ctx, true)
	if err != nil || state != "authorized" {
		t.Fatalf("authorization: %q, %v", state, err)
	}
	_, err = platform.RegisterService(ctx, false)
	requireFailure(t, err, "denied")
	state, err = platform.RegisterService(ctx, true)
	if err != nil || state != "requires_approval" {
		t.Fatalf("registration: %q, %v", state, err)
	}
	state, err = platform.UnregisterService(ctx, true)
	if err != nil || state != "not_registered" {
		t.Fatalf("unregister: %q, %v", state, err)
	}
	if err := platform.SubmitNotification(ctx, Notification{ID: profileID(t), Title: "Synthetic PR", Body: "Entered"}); err != nil {
		t.Fatal(err)
	}
}

func TestHelperFailureAndWireValidation(t *testing.T) {
	for mode, code := range map[string]string{
		"fail": "unavailable", "error": "locked", "version": "unsupported_version",
		"unknown": "invalid_response", "correlation": "invalid_response", "both": "invalid_response",
		"large": "invalid_response", "bad-state": "invalid_response",
	} {
		t.Run(mode, func(t *testing.T) {
			_, err := fixturePlatform(t, mode).NotificationStatus(context.Background())
			requireFailure(t, err, code)
		})
	}
}

func TestHelperDeadlineAndAmbiguousNotification(t *testing.T) {
	platform := fixturePlatform(t, "hang")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := platform.NotificationStatus(ctx)
	requireFailure(t, err, "unavailable")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("deadline cause lost")
	}
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err = platform.SubmitNotification(ctx, Notification{ID: profileID(t), Title: "Synthetic PR", Body: "Entered"})
	requireFailure(t, err, "ambiguous")
	platform = fixturePlatform(t, "fail")
	err = platform.SubmitNotification(context.Background(), Notification{ID: profileID(t), Title: "Synthetic PR", Body: "Entered"})
	requireFailure(t, err, "ambiguous")
}

func TestInvalidInputNeverLaunchesHelper(t *testing.T) {
	platform, err := New(filepath.Join(t.TempDir(), "absent"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = platform.NotificationStatus(context.Background())
	requireFailure(t, err, "unavailable")
	_, err = platform.AuthorizeNotifications(context.Background(), false)
	requireFailure(t, err, "denied")
	err = platform.WriteCredential(context.Background(), identity.ID{}, []byte("synthetic-private"))
	requireFailure(t, err, "invalid_request")
	if _, err := New("relative/helper"); err == nil {
		t.Fatal("relative helper path accepted")
	}
}

func TestUnbundledSwiftHelper(t *testing.T) {
	path := os.Getenv("SHIPQ_TEST_SWIFT_HELPER")
	if path == "" {
		t.Skip("set explicit unbundled build path for offline Go/Swift IPC check")
	}
	platform, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	state, err := platform.NotificationStatus(context.Background())
	if err != nil || state != "unavailable" {
		t.Fatal("unbundled native notifications must remain unavailable")
	}
	state, err = platform.ServiceStatus(context.Background())
	if err != nil || state != "unavailable" {
		t.Fatal("unbundled native service must remain unavailable")
	}
	_, err = platform.ReadCredential(context.Background(), profileID(t))
	requireFailure(t, err, "unavailable")
}
