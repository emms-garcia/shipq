//go:build darwin

package macos

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/emms-garcia/shipq/internal/domain/identity"
)

const (
	protocolVersion      = 1
	maxBytes             = 1024 * 1024
	maxCredentialBytes   = 64 * 1024
	helperTimeout        = 10 * time.Second
	authorizationTimeout = 2 * time.Minute
)

type Failure struct {
	Code  string
	cause error
}

func (e *Failure) Error() string { return "macOS capability: " + e.Code }
func (e *Failure) Unwrap() error { return e.cause }

type Notification struct {
	ID    identity.ID
	Title string
	Body  string
}

type Platform struct{ helperPath string }

func New(helperPath string) (*Platform, error) {
	if !filepath.IsAbs(helperPath) {
		return nil, &Failure{Code: "invalid_request"}
	}
	return &Platform{helperPath: helperPath}, nil
}

type request struct {
	ProtocolVersion int    `json:"protocol_version"`
	RequestID       string `json:"request_id"`
	Operation       string `json:"operation"`
	ProfileID       string `json:"profile_id,omitempty"`
	Credential      string `json:"credential,omitempty"`
	UserInitiated   bool   `json:"user_initiated,omitempty"`
	NotificationID  string `json:"notification_id,omitempty"`
	Title           string `json:"title,omitempty"`
	Body            string `json:"body,omitempty"`
}

type result struct {
	State      string  `json:"state"`
	Credential *string `json:"credential,omitempty"`
}

type wireError struct {
	Code string `json:"code"`
}
type response struct {
	ProtocolVersion int             `json:"protocol_version"`
	RequestID       string          `json:"request_id"`
	Data            json.RawMessage `json:"data"`
	Error           *wireError      `json:"error"`
}

func (p *Platform) ReadCredential(ctx context.Context, profile identity.ID) ([]byte, error) {
	if profile == (identity.ID{}) {
		return nil, &Failure{Code: "invalid_request"}
	}
	data, err := p.call(ctx, request{Operation: "credential.read", ProfileID: profile.String()}, "found")
	if err != nil {
		return nil, err
	}
	if data.Credential == nil {
		return nil, &Failure{Code: "invalid_response"}
	}
	value, err := base64.StdEncoding.Strict().DecodeString(*data.Credential)
	if err != nil || len(value) == 0 || len(value) > maxCredentialBytes {
		return nil, &Failure{Code: "invalid_response"}
	}
	return value, nil
}

func (p *Platform) WriteCredential(ctx context.Context, profile identity.ID, value []byte) error {
	if profile == (identity.ID{}) || len(value) == 0 || len(value) > maxCredentialBytes {
		return &Failure{Code: "invalid_request"}
	}
	_, err := p.call(ctx, request{Operation: "credential.write", ProfileID: profile.String(), Credential: base64.StdEncoding.EncodeToString(value)}, "stored")
	return err
}

func (p *Platform) DeleteCredential(ctx context.Context, profile identity.ID) error {
	if profile == (identity.ID{}) {
		return &Failure{Code: "invalid_request"}
	}
	_, err := p.call(ctx, request{Operation: "credential.delete", ProfileID: profile.String()}, "deleted")
	return err
}

var permissionStates = []string{"not_determined", "denied", "authorized", "provisional", "unavailable"}
var serviceStates = []string{"not_registered", "enabled", "requires_approval", "not_found", "unavailable"}

func (p *Platform) NotificationStatus(ctx context.Context) (string, error) {
	data, err := p.call(ctx, request{Operation: "notification.status"}, permissionStates...)
	return data.State, err
}

func (p *Platform) AuthorizeNotifications(ctx context.Context, userInitiated bool) (string, error) {
	if !userInitiated {
		return "", &Failure{Code: "denied"}
	}
	data, err := p.call(ctx, request{Operation: "notification.authorize", UserInitiated: true}, permissionStates...)
	return data.State, err
}

func (p *Platform) SubmitNotification(ctx context.Context, notification Notification) error {
	if notification.ID == (identity.ID{}) || notification.Title == "" || notification.Body == "" ||
		!utf8.ValidString(notification.Title) || !utf8.ValidString(notification.Body) {
		return &Failure{Code: "invalid_request"}
	}
	_, err := p.call(ctx, request{Operation: "notification.submit", NotificationID: notification.ID.String(), Title: notification.Title, Body: notification.Body}, "submitted")
	return err
}

func (p *Platform) ServiceStatus(ctx context.Context) (string, error) {
	data, err := p.call(ctx, request{Operation: "service.status"}, serviceStates...)
	return data.State, err
}

func (p *Platform) RegisterService(ctx context.Context, userInitiated bool) (string, error) {
	return p.changeService(ctx, "service.register", userInitiated)
}

func (p *Platform) UnregisterService(ctx context.Context, userInitiated bool) (string, error) {
	return p.changeService(ctx, "service.unregister", userInitiated)
}

func (p *Platform) changeService(ctx context.Context, operation string, userInitiated bool) (string, error) {
	if !userInitiated {
		return "", &Failure{Code: "denied"}
	}
	data, err := p.call(ctx, request{Operation: operation, UserInitiated: true}, serviceStates...)
	return data.State, err
}

func (p *Platform) call(ctx context.Context, input request, states ...string) (result, error) {
	input.ProtocolVersion = protocolVersion
	input.RequestID = uuid.NewString()
	encoded, err := json.Marshal(input)
	if err != nil || len(encoded) > maxBytes {
		return result{}, &Failure{Code: "invalid_request"}
	}
	timeout := helperTimeout
	if input.Operation == "notification.authorize" {
		timeout = authorizationTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, p.helperPath)
	cmd.Env = []string{"LANG=en_US.UTF-8"}
	cmd.Stdin = bytes.NewReader(encoded)
	output := &boundedOutput{}
	cmd.Stdout = output
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	if err := cmd.Start(); err != nil {
		return result{}, &Failure{Code: "unavailable", cause: ctx.Err()}
	}
	err = cmd.Wait()
	failure := func(code string) (result, error) {
		// OS acceptance may precede a lost helper acknowledgment.
		if input.Operation == "notification.submit" {
			code = "ambiguous"
		}
		return result{}, &Failure{Code: code, cause: ctx.Err()}
	}
	if output.overflow {
		return failure("invalid_response")
	}
	if err != nil {
		return failure("unavailable")
	}
	var envelope response
	if strictDecode(output.Bytes(), &envelope) != nil {
		return failure("invalid_response")
	}
	if envelope.ProtocolVersion != protocolVersion {
		return failure("unsupported_version")
	}
	if envelope.RequestID != input.RequestID {
		return failure("invalid_response")
	}
	hasData := len(envelope.Data) != 0 && !bytes.Equal(bytes.TrimSpace(envelope.Data), []byte("null"))
	if hasData == (envelope.Error != nil) {
		return failure("invalid_response")
	}
	if envelope.Error != nil {
		if !slices.Contains([]string{"invalid_request", "unsupported_version", "denied", "locked", "unavailable", "failed", "not_found"}, envelope.Error.Code) {
			return failure("invalid_response")
		}
		return result{}, &Failure{Code: envelope.Error.Code}
	}
	var data result
	if strictDecode(envelope.Data, &data) != nil || !slices.Contains(states, data.State) ||
		(input.Operation != "credential.read" && data.Credential != nil) {
		return failure("invalid_response")
	}
	return data, nil
}

func strictDecode(data []byte, target any) error {
	if !utf8.Valid(data) {
		return errors.New("invalid UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}

type boundedOutput struct {
	bytes.Buffer
	overflow bool
}

func (w *boundedOutput) Write(data []byte) (int, error) {
	remaining := maxBytes - w.Len()
	if len(data) > remaining {
		w.overflow = true
		w.Buffer.Write(data[:remaining])
	} else {
		w.Buffer.Write(data)
	}
	return len(data), nil
}
