//go:build darwin || linux

package localapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	v1 "github.com/emms-garcia/shipq/api/local/v1"
	"github.com/emms-garcia/shipq/internal/application/ports"
	"github.com/emms-garcia/shipq/internal/domain/identity"
	"github.com/emms-garcia/shipq/internal/domain/segments"
)

type fakeStatus struct{ err error }

func (f fakeStatus) ReadStatus(context.Context) (ports.HostStatus, error) {
	id, _ := identity.ParseID("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	return ports.HostStatus{HostVersion: "0.0.0-dev", ActiveProfileID: id, MonitorState: "unavailable", NotificationPermission: "unavailable", CoverageState: "unknown"}, f.err
}

const countRule = `{"format_version":1,"condition":{"field":"approval_count","op":"gte","value":3}}`

func request(t *testing.T, handler http.Handler, method, path, body, contentType string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	for key, value := range headers {
		r.Header.Set(key, value)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func envelope(t *testing.T, w *httptest.ResponseRecorder) v1.Envelope {
	t.Helper()
	e, err := v1.DecodeEnvelope(w.Body.Bytes())
	if err != nil {
		t.Fatalf("invalid response: %s, %v", w.Body.String(), err)
	}
	if e.RequestID != w.Header().Get("X-Request-ID") || w.Header().Get("X-Shipq-Protocol-Version") != "1" ||
		!strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		t.Fatal("missing protocol/correlation headers")
	}
	return e
}

func TestStatusAndRuleValidation(t *testing.T) {
	handler := NewHandler(fakeStatus{})
	w := request(t, handler, "GET", "/v1/status", "", "", nil)
	if w.Code != 200 {
		t.Fatalf("status: %d %s", w.Code, w.Body.String())
	}
	e := envelope(t, w)
	var status v1.Status
	if err := json.Unmarshal(e.Data, &status); err != nil {
		t.Fatal(err)
	}
	if status.ProtocolVersion != 1 || status.MonitorState != "unavailable" || status.Coverage.State != "unknown" || status.ActiveProfileID == "" {
		t.Fatalf("false status: %+v", status)
	}
	w = request(t, handler, "POST", "/v1/rules/validate", countRule, "application/json; charset=utf-8", nil)
	if w.Code != 200 {
		t.Fatalf("validation: %d %s", w.Code, w.Body.String())
	}
	e = envelope(t, w)
	var result v1.RuleValidation
	if err := json.Unmarshal(e.Data, &result); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Summary, "at least 3") || len(result.RequiredFields) != 1 || result.RequiredFields[0] != "approval_count" {
		t.Fatalf("missing core result: %+v", result)
	}
}

func TestHTTPBoundaryRejections(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, body, contentType string
		headers                               map[string]string
		status                                int
		code                                  string
	}{
		{"missing_type", "POST", "/v1/rules/validate", countRule, "", nil, 415, "unsupported_media_type"},
		{"wrong_type", "POST", "/v1/rules/validate", countRule, "text/plain", nil, 415, "unsupported_media_type"},
		{"wrong_charset", "POST", "/v1/rules/validate", countRule, "application/json; charset=latin1", nil, 415, "unsupported_media_type"},
		{"compressed", "POST", "/v1/rules/validate", countRule, "application/json", map[string]string{"Content-Encoding": "gzip"}, 415, "unsupported_media_type"},
		{"origin", "GET", "/v1/status", "", "", map[string]string{"Origin": "https://example.test"}, 403, "forbidden_origin"},
		{"empty_origin", "GET", "/v1/status", "", "", map[string]string{"Origin": ""}, 403, "forbidden_origin"},
		{"browser_fetch", "GET", "/v1/status", "", "", map[string]string{"Sec-Fetch-Site": "same-origin"}, 403, "forbidden_origin"},
		{"unknown_version", "GET", "/v2/status", "", "", nil, 400, "unsupported_version"},
		{"unknown_route", "GET", "/v1/missing", "", "", nil, 404, "not_found"},
		{"wrong_method", "POST", "/v1/status", "", "", nil, 405, "method_not_allowed"},
		{"malformed", "POST", "/v1/rules/validate", `{"format_version":`, "application/json", nil, 400, "invalid_request"},
		{"unknown_root", "POST", "/v1/rules/validate", strings.TrimSuffix(countRule, "}") + `,"secret":"do-not-echo"}`, "application/json", nil, 400, "invalid_request"},
		{"null_number", "POST", "/v1/rules/validate", strings.Replace(countRule, `"value":3`, `"value":null`, 1), "application/json", nil, 400, "invalid_request"},
		{"decimal_number", "POST", "/v1/rules/validate", strings.Replace(countRule, `"value":3`, `"value":3.0`, 1), "application/json", nil, 400, "invalid_request"},
		{"wrong_field_type", "POST", "/v1/rules/validate", strings.Replace(countRule, `"approval_count"`, `[]`, 1), "application/json", nil, 400, "invalid_request"},
		{"duplicate_field", "POST", "/v1/rules/validate", strings.Replace(countRule, `"op":"gte"`, `"op":"gte","op":"eq"`, 1), "application/json", nil, 400, "invalid_request"},
		{"rule_version", "POST", "/v1/rules/validate", strings.Replace(countRule, `"format_version":1`, `"format_version":2`, 1), "application/json", nil, 422, "unsupported_version"},
		{"negative_count", "POST", "/v1/rules/validate", strings.Replace(countRule, `"value":3`, `"value":-1`, 1), "application/json", nil, 422, "validation_failed"},
		{"unknown_field", "POST", "/v1/rules/validate", strings.Replace(countRule, `"approval_count"`, `"invented"`, 1), "application/json", nil, 422, "validation_failed"},
		{"unknown_op", "POST", "/v1/rules/validate", strings.Replace(countRule, `"gte"`, `"invented"`, 1), "application/json", nil, 422, "validation_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := request(t, NewHandler(fakeStatus{}), tc.method, tc.path, tc.body, tc.contentType, tc.headers)
			e := envelope(t, w)
			if w.Code != tc.status || e.Error == nil || e.Error.Code != tc.code {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "do-not-echo") || w.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Fatal("unsafe error/CORS response")
			}
		})
	}
	w := request(t, NewHandler(fakeStatus{err: errors.New("do-not-echo")}), "GET", "/v1/status", "", "", nil)
	if w.Code != 500 || envelope(t, w).Error.Code != "internal" || strings.Contains(w.Body.String(), "do-not-echo") {
		t.Fatal("provider error leaked")
	}
	w = request(t, NewHandler(nil), "GET", "/v1/status", "", "", nil)
	if w.Code != 503 || envelope(t, w).Error.Code != "host_unavailable" {
		t.Fatal("missing host reported success")
	}
}

func TestEncodedRuleAndBodyLimits(t *testing.T) {
	handler := NewHandler(fakeStatus{})
	for _, size := range []int{segments.MaxEncodedBytes, segments.MaxEncodedBytes + 1, MaxBodyBytes, MaxBodyBytes + 1} {
		body := countRule + strings.Repeat(" ", size-len(countRule))
		w := request(t, handler, "POST", "/v1/rules/validate", body, "application/json", nil)
		want := 422
		if size == segments.MaxEncodedBytes {
			want = 200
		} else if size > MaxBodyBytes {
			want = 413
		}
		if w.Code != want {
			t.Fatalf("size %d: %d %s", size, w.Code, w.Body.String())
		}
		envelope(t, w)
		r := httptest.NewRequest("POST", "/v1/rules/validate", strings.NewReader(body))
		r.ContentLength = -1
		r.Header.Set("Content-Type", "application/json")
		chunked := httptest.NewRecorder()
		handler.ServeHTTP(chunked, r)
		if chunked.Code != want {
			t.Fatalf("streamed size %d: %d", size, chunked.Code)
		}
	}
}

func runtimeDirectory(t *testing.T) string {
	t.Helper()
	directory, err := os.MkdirTemp("/tmp", "shipq-api-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	return directory
}

func TestPrivateUnixSocketLifecycle(t *testing.T) {
	directory := runtimeDirectory(t)
	s, err := Start(directory, NewHandler(fakeStatus{}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Shutdown(context.Background()) })
	info, err := os.Stat(directory)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatal("runtime directory not private")
	}
	info, err = os.Stat(s.SocketPath())
	if err != nil || info.Mode()&os.ModeSocket == 0 || info.Mode().Perm() != 0o600 {
		t.Fatal("socket not private")
	}
	if other, err := Start(directory, NewHandler(fakeStatus{})); other != nil || !errors.Is(err, ErrSocketOwned) {
		t.Fatalf("socket collision: %v %v", other, err)
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", s.SocketPath())
	}}
	defer transport.CloseIdleConnections()
	client := http.Client{Transport: transport, Timeout: 3 * time.Second}
	response, err := client.Get("http://shipq/v1/status")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("Unix response: %s", data)
	}
	if _, err := v1.DecodeEnvelope(data); err != nil {
		t.Fatal(err)
	}
	response, err = client.Post("http://shipq/v1/rules/validate", "application/json", bytes.NewBufferString(countRule))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("Unix validation failed")
	}
	transport.CloseIdleConnections()
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Wait(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(s.SocketPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("owned socket not cleaned")
	}
	next, err := Start(directory, NewHandler(fakeStatus{}))
	if err != nil {
		t.Fatal(err)
	}
	defer next.Shutdown(context.Background())
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(next.SocketPath()); err != nil {
		t.Fatal("old shutdown removed new socket")
	}
}

func TestSocketPathSafetyAndStaleRecovery(t *testing.T) {
	for _, directory := range []string{"relative", strings.Repeat("/long", 30)} {
		if s, err := Start(directory, NewHandler(fakeStatus{})); s != nil || err == nil {
			t.Fatal("invalid path accepted")
		}
	}
	directory := runtimeDirectory(t)
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if s, err := Start(directory, NewHandler(fakeStatus{})); s != nil || err == nil {
		t.Fatal("public directory accepted")
	}
	os.Chmod(directory, 0o700)
	path := filepath.Join(directory, socketName)
	if err := os.WriteFile(path, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	if s, err := Start(directory, NewHandler(fakeStatus{})); s != nil || err == nil {
		t.Fatal("unrelated file replaced")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "untouched" {
		t.Fatal("unrelated file destroyed")
	}
	os.Remove(path)
	stale, err := net.ListenUnix("unix", &net.UnixAddr{Net: "unix", Name: path})
	if err != nil {
		t.Fatal(err)
	}
	stale.SetUnlinkOnClose(false)
	stale.Close()
	s, err := Start(directory, NewHandler(fakeStatus{}))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown(context.Background())
}

func TestRuleWireMapping(t *testing.T) {
	for _, tc := range []struct{ field, op, value, summary string }{
		{"organization", "in", `["ORG"]`, "github.com/ORG"},
		{"repository", "not_in", `["REPO"]`, "github.com/REPO"},
		{"author", "in", `["me"]`, "connected account"},
		{"assignees", "all", `["BOB"]`, "github.com/BOB"},
		{"requested_reviewers", "any", `[{"kind":"team","node_id":"TEAM","display_hint":"Review team"}]`, "Review team"},
		{"reviewed_by", "none", `["BOB"]`, "includes none of"},
		{"approvers", "all", `[{"kind":"user","github_host":"GITHUB.COM","node_id":"BOB","display_hint":"Bob"}]`, "github.com/BOB"},
		{"approval_count", "eq", `2`, "exactly 2"},
		{"state", "in", `["closed","merged"]`, "merged"},
		{"review_state", "not_in", `["none"]`, "is not one of"},
		{"draft", "eq", `true`, "exactly true"},
		{"local_watched", "eq", `false`, "exactly false"},
		{"repository_watched", "eq", `true`, "exactly true"},
		{"title", "contains", `"Straße"`, "Straße"},
		{"created_at", "gte", `"2026-01-02T10:00:00Z"`, "on or after"},
		{"updated_at", "lt", `"2026-01-02T10:00:00+00:00"`, "2026-01-02T10:00:00Z"},
		{"labels", "any", `[{"node_id":"LABEL","repository_node_id":"REPO","display_hint":"bug"}]`, "github.com/REPO"},
	} {
		t.Run(tc.field, func(t *testing.T) {
			data := fmt.Sprintf(`{"format_version":1,"condition":{"field":%q,"op":%q,"value":%s}}`, tc.field, tc.op, tc.value)
			result, err := validateRule([]byte(data))
			if err != nil || !strings.Contains(result.Summary, tc.summary) {
				t.Fatalf("mapping: %+v %v", result, err)
			}
		})
	}
	for _, node := range []string{
		`{"field":"labels","op":"any","value":["bug"]}`,
		`{"field":"author","op":"in","value":[null]}`,
		`{"field":"state","op":"in","value":[null]}`,
		`{"field":"draft","op":"eq","value":null}`,
		`{"field":"title","op":"contains","value":false}`,
		`{"field":"created_at","op":"gte","value":"2026-01-02T10:00:00+01:00"}`,
		`{"field":"updated_at","op":"lt","value":"not-a-date"}`,
		`{"field":"approvers","op":"all","value":[{"kind":"team","node_id":"TEAM"}]}`,
		`{"field":"labels","op":"any","value":[{"node_id":"LABEL"}]}`,
		`{"field":"requested_reviewers","op":"any","value":[{"kind":"team","node_id":"TEAM","extra":true}]}`,
		`{"field":"author","op":"in","value":[{"kind":"user","node_id":"BOB","display_hint":null}]}`,
		`{"field":"author","op":"in","value":[{"kind":"user","node_id":"BOB","github_host":null}]}`,
		`{"field":"author","op":"in","value":[{"kind":"user","node_id":" "}]}`,
		`{"all":[],"field":"state","op":"in","value":["open"]}`,
		`{"not":null}`,
		`{"any":null}`,
		`{"all":[]}`,
		`{"field":"state","op":"in"}`,
	} {
		if _, err := validateRule([]byte(`{"format_version":1,"condition":` + node + `}`)); err == nil {
			t.Errorf("invalid mapping accepted: %s", node)
		}
	}
	group := `{"format_version":1,"condition":{"all":[{"any":[{"field":"draft","op":"eq","value":false}]},{"not":{"field":"author","op":"in","value":["me"]}}]}}`
	result, err := validateRule([]byte(group))
	if err != nil || !strings.Contains(result.Summary, "any of (") || !strings.Contains(result.Summary, "not (") {
		t.Fatalf("grouping lost: %v", err)
	}
}

func TestWireResourceLimits(t *testing.T) {
	condition := `{"field":"state","op":"in","value":["merged"]}`
	for _, n := range []int{segments.MaxDepth, segments.MaxDepth + 1} {
		node := condition
		for i := 1; i < n; i++ {
			node = `{"not":` + node + `}`
		}
		_, err := validateRule([]byte(`{"format_version":1,"condition":` + node + `}`))
		if (err == nil) != (n == segments.MaxDepth) {
			t.Fatalf("depth %d: %v", n, err)
		}
	}
	for _, n := range []int{segments.MaxNodes, segments.MaxNodes + 1} {
		children := make([]string, n-1)
		for i := range children {
			children[i] = condition
		}
		_, err := validateRule([]byte(`{"format_version":1,"condition":{"all":[` + strings.Join(children, ",") + `]}}`))
		if (err == nil) != (n == segments.MaxNodes) {
			t.Fatalf("nodes %d: %v", n, err)
		}
	}
	for _, n := range []int{segments.MaxSetValues, segments.MaxSetValues + 1} {
		values := make([]string, n)
		for i := range values {
			values[i] = `"merged"`
		}
		_, err := validateRule([]byte(`{"format_version":1,"condition":{"field":"state","op":"in","value":[` + strings.Join(values, ",") + `]}}`))
		if (err == nil) != (n == segments.MaxSetValues) {
			t.Fatalf("set %d: %v", n, err)
		}
	}
	unicode := `{"format_version":1,"condition":{"field":"title","op":"contains","value":"` + strings.Repeat("é", segments.MaxEncodedBytes/2) + `"}}`
	w := request(t, NewHandler(fakeStatus{}), "POST", "/v1/rules/validate", unicode, "application/json", nil)
	e := envelope(t, w)
	if w.Code != 422 || e.Error.Details["path"] != "encoded_bytes" {
		t.Fatal("size measured in runes rather than encoded bytes")
	}
}

func TestSocketRejectsLiveUnownedListenerAndSymlinks(t *testing.T) {
	directory := runtimeDirectory(t)
	path := filepath.Join(directory, socketName)
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Net: "unix", Name: path})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if s, err := Start(directory, NewHandler(fakeStatus{})); s != nil || !errors.Is(err, ErrSocketOwned) {
		t.Fatalf("live listener replaced: %v %v", s, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("live socket removed")
	}
	listener.Close()
	target := filepath.Join(directory, "untouched")
	if err := os.WriteFile(target, []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if s, err := Start(directory, NewHandler(fakeStatus{})); s != nil || err == nil {
		t.Fatal("socket symlink replaced")
	}
	os.Remove(path)
	os.Remove(filepath.Join(directory, lockName))
	if err := os.Symlink(target, filepath.Join(directory, lockName)); err != nil {
		t.Fatal(err)
	}
	if s, err := Start(directory, NewHandler(fakeStatus{})); s != nil || err == nil {
		t.Fatal("ownership symlink followed")
	}
	data, _ := os.ReadFile(target)
	if string(data) != "safe" {
		t.Fatal("unrelated target modified")
	}
}

func TestSocketLengthBoundary(t *testing.T) {
	root := runtimeDirectory(t)
	directory := filepath.Join(root, strings.Repeat("d", MaxSocketPath-len(root)-len(socketName)-2))
	s, err := Start(directory, NewHandler(fakeStatus{}))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown(context.Background())
	if len(s.SocketPath()) != MaxSocketPath {
		t.Fatal("boundary case was not exercised")
	}
	if next, err := Start(directory+"x", NewHandler(fakeStatus{})); next != nil || err == nil {
		t.Fatal("oversized socket path accepted")
	}
}

type waitingStatus struct{ entered, release chan struct{} }

func (s waitingStatus) ReadStatus(ctx context.Context) (ports.HostStatus, error) {
	close(s.entered)
	<-s.release
	return (fakeStatus{}).ReadStatus(ctx)
}

func TestShutdownTimeoutRetainsSocketOwnership(t *testing.T) {
	directory := runtimeDirectory(t)
	waiting := waitingStatus{entered: make(chan struct{}), release: make(chan struct{})}
	defer func() {
		select {
		case <-waiting.release:
		default:
			close(waiting.release)
		}
	}()
	s, err := Start(directory, NewHandler(waiting))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Shutdown(context.Background()) })
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", s.SocketPath())
	}}
	defer transport.CloseIdleConnections()
	completed := make(chan error, 1)
	go func() {
		client := http.Client{Transport: transport, Timeout: 3 * time.Second}
		response, err := client.Get("http://shipq/v1/status")
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			response.Body.Close()
		}
		completed <- err
	}()
	select {
	case <-waiting.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("request not active")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := s.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown: %v", err)
	}
	if other, err := Start(directory, NewHandler(fakeStatus{})); other != nil || !errors.Is(err, ErrSocketOwned) {
		t.Fatal("shutdown timeout released ownership")
	}
	close(waiting.release)
	if err := <-completed; err != nil {
		t.Fatal(err)
	}
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

type panickingStatus struct{}

func (panickingStatus) ReadStatus(context.Context) (ports.HostStatus, error) { panic("do-not-echo") }

func TestStatusPanicIsSafeError(t *testing.T) {
	w := request(t, NewHandler(panickingStatus{}), "GET", "/v1/status", "", "", nil)
	e := envelope(t, w)
	if w.Code != 500 || e.Error.Code != "internal" || strings.Contains(w.Body.String(), "do-not-echo") {
		t.Fatal("status panic leaked")
	}
}

func TestShutdownPreservesReplacementAndReleasesOwnership(t *testing.T) {
	directory := runtimeDirectory(t)
	s, err := Start(directory, NewHandler(fakeStatus{}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Shutdown(context.Background()) })
	if err := os.Remove(s.SocketPath()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.SocketPath(), []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.Shutdown(context.Background()); err == nil {
		t.Fatal("replacement was not reported")
	}
	data, _ := os.ReadFile(s.SocketPath())
	if string(data) != "replacement" {
		t.Fatal("shutdown removed an unrelated replacement")
	}
	if err := os.Remove(s.SocketPath()); err != nil {
		t.Fatal(err)
	}
	next, err := Start(directory, NewHandler(fakeStatus{}))
	if err != nil {
		t.Fatalf("finished shutdown leaked runtime ownership: %v", err)
	}
	defer next.Shutdown(context.Background())
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(next.SocketPath()); err != nil {
		t.Fatal("old shutdown removed a new owner")
	}
}
