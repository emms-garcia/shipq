//go:build darwin || linux

package localapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	v1 "github.com/emms-garcia/shipq/api/local/v1"
	"github.com/emms-garcia/shipq/internal/application"
	"github.com/emms-garcia/shipq/internal/application/ports"
	"github.com/emms-garcia/shipq/internal/domain"
	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

const (
	MaxBodyBytes  = 1024 * 1024
	MaxSocketPath = 103
	socketName    = "api.sock"
	lockName      = "socket.lock"
)

var ErrSocketOwned = errors.New("runtime socket is already owned")

func NewHandler(status ports.StatusReader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := uuid.NewString()
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("X-Shipq-Protocol-Version", fmt.Sprint(v1.ProtocolVersion))
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		defer func() {
			if recover() != nil {
				failure(w, 500, id, "internal", "Unable to complete request.", true, "")
			}
		}()
		if len(r.Header.Values("Origin")) != 0 || len(r.Header.Values("Sec-Fetch-Site")) != 0 {
			failure(w, 403, id, "forbidden_origin", "Browser-origin requests are not allowed.", false, "")
			return
		}
		if r.ContentLength > MaxBodyBytes {
			failure(w, 413, id, "payload_too_large", "Maximum request body size is 1 MiB.", false, "")
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/v1/") {
			failure(w, 400, id, "unsupported_version", "Use the v1 protocol.", false, "")
			return
		}
		switch r.URL.Path {
		case "/v1/status":
			if !method(w, r, id, http.MethodGet) {
				return
			}
			if r.ContentLength != 0 {
				failure(w, 400, id, "invalid_request", "Status requests must not contain a body.", false, "")
				return
			}
			snapshot, err := application.GetStatus(r.Context(), status)
			if err != nil {
				if errors.Is(err, application.ErrHostUnavailable) {
					failure(w, 503, id, "host_unavailable", "The host is not ready.", true, "")
				} else {
					failure(w, 500, id, "internal", "Unable to read host status.", true, "")
				}
				return
			}
			success(w, id, v1.Status{
				ProtocolVersion: v1.ProtocolVersion, HostVersion: snapshot.HostVersion,
				ActiveProfileID: snapshot.ActiveProfileID.String(), MonitorState: snapshot.MonitorState,
				NotificationPermission: snapshot.NotificationPermission,
				Coverage:               v1.Coverage{State: snapshot.CoverageState},
			})
		case "/v1/rules/validate":
			if !method(w, r, id, http.MethodPost) {
				return
			}
			contentType, parameters, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			charset := parameters["charset"]
			if err != nil || contentType != "application/json" || len(parameters) > 1 ||
				(len(parameters) == 1 && charset == "") ||
				(charset != "" && !strings.EqualFold(charset, "utf-8")) ||
				r.Header.Get("Content-Encoding") != "" {
				failure(w, 415, id, "unsupported_media_type", "Use uncompressed UTF-8 application/json.", false, "")
				return
			}
			body := http.MaxBytesReader(w, r.Body, MaxBodyBytes)
			defer body.Close()
			data, err := io.ReadAll(body)
			if err != nil {
				var oversized *http.MaxBytesError
				if errors.As(err, &oversized) {
					failure(w, 413, id, "payload_too_large", "Maximum request body size is 1 MiB.", false, "")
				} else {
					failure(w, 400, id, "invalid_request", "Unable to read request body.", false, "")
				}
				return
			}
			result, err := validateRule(data)
			if err != nil {
				var invalid *v1.SchemaError
				var validation *domain.ValidationError
				switch {
				case errors.As(err, &invalid):
					failure(w, 400, id, "invalid_request", invalid.Message, false, invalid.Path)
				case errors.As(err, &validation):
					code := "validation_failed"
					if validation.Field == "format_version" {
						code = "unsupported_version"
					}
					failure(w, 422, id, code, validation.Message, false, validation.Field)
				default:
					failure(w, 500, id, "internal", "Unable to validate rule.", true, "")
				}
				return
			}
			fields := make([]string, len(result.RequiredFields))
			for i, field := range result.RequiredFields {
				fields[i] = string(field)
			}
			success(w, id, v1.RuleValidation{Summary: result.Summary, RequiredFields: fields})
		default:
			failure(w, 404, id, "not_found", "Unknown operation.", false, "")
		}
	})
}

func method(w http.ResponseWriter, r *http.Request, id, expected string) bool {
	if r.Method == expected {
		return true
	}
	w.Header().Set("Allow", expected)
	failure(w, 405, id, "method_not_allowed", "Unsupported method for this operation.", false, "")
	return false
}

func success(w http.ResponseWriter, id string, data any) {
	encoded, err := json.Marshal(data)
	if err != nil {
		failure(w, 500, id, "internal", "Unable to encode response.", true, "")
		return
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(v1.Envelope{RequestID: id, Data: encoded})
}

func failure(w http.ResponseWriter, status int, id, code, message string, retryable bool, path string) {
	details := make(map[string]any)
	if path != "" {
		details["path"] = path
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v1.Envelope{RequestID: id, Error: &v1.Error{
		Code: code, Message: message, Retryable: retryable, Details: details,
	}})
}

type Server struct {
	http       *http.Server
	path       string
	socketInfo os.FileInfo
	lock       *os.File
	done       chan struct{}
	serveErr   error
	closeMu    sync.Mutex
	closed     bool
}

func Start(directory string, handler http.Handler) (*Server, error) {
	path := filepath.Join(directory, socketName)
	if !filepath.IsAbs(directory) || len(path) > MaxSocketPath || handler == nil {
		return nil, errors.New("runtime directory must be absolute, fit the Unix socket path limit, and have a handler")
	}
	if err := os.Mkdir(directory, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, fmt.Errorf("create runtime directory: %w", err)
	}
	var info unix.Stat_t
	if err := unix.Lstat(directory, &info); err != nil {
		return nil, err
	}
	if info.Mode&unix.S_IFMT != unix.S_IFDIR || info.Mode&0o777 != 0o700 || int(info.Uid) != os.Getuid() {
		return nil, errors.New("runtime directory must be an owned, nonsymlink directory with mode 0700")
	}
	lock, err := os.OpenFile(filepath.Join(directory, lockName), os.O_CREATE|os.O_RDWR|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	owned := false
	defer func() {
		if !owned {
			_ = lock.Close()
		}
	}()
	lockInfo, err := lock.Stat()
	if err != nil || !lockInfo.Mode().IsRegular() {
		return nil, errors.New("socket ownership must use a regular file")
	}
	if err := lock.Chmod(0o600); err != nil {
		return nil, err
	}
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, ErrSocketOwned
		}
		return nil, err
	}
	if err := removeStaleSocket(path); err != nil {
		return nil, err
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Net: "unix", Name: path})
	if err != nil {
		return nil, err
	}
	listener.SetUnlinkOnClose(false)
	if err := os.Chmod(path, 0o600); err != nil {
		_ = listener.Close()
		_ = os.Remove(path)
		return nil, err
	}
	socketInfo, err := os.Lstat(path)
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	server := &Server{
		path: path, socketInfo: socketInfo, lock: lock, done: make(chan struct{}),
		http: &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
			IdleTimeout: 30 * time.Second, WriteTimeout: 10 * time.Second, MaxHeaderBytes: 16 * 1024},
	}
	owned = true
	go func() {
		server.serveErr = server.http.Serve(listener)
		if errors.Is(server.serveErr, http.ErrServerClosed) {
			server.serveErr = nil
		}
		close(server.done)
	}()
	return server, nil
}

func removeStaleSocket(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSocket == 0 {
		return errors.New("existing socket path is not a socket; refusing replacement")
	}
	connection, err := net.DialTimeout("unix", path, 100*time.Millisecond)
	if err == nil {
		_ = connection.Close()
		return ErrSocketOwned
	}
	if !errors.Is(err, unix.ECONNREFUSED) {
		return fmt.Errorf("cannot establish that socket is stale: %w", err)
	}
	return os.Remove(path)
}

func (s *Server) SocketPath() string { return s.path }

func (s *Server) Wait() error {
	<-s.done
	return s.serveErr
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	if s.closed {
		return nil
	}
	if err := s.http.Shutdown(ctx); err != nil {
		// Keep ownership while any request remains in flight; callers may retry.
		return err
	}
	<-s.done
	info, err := os.Lstat(s.path)
	if err == nil {
		if info.Mode()&os.ModeSocket == 0 || !os.SameFile(info, s.socketInfo) {
			err = errors.New("socket path changed; refusing to remove replacement")
		} else {
			err = os.Remove(s.path)
		}
	} else if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	// Keep the lock file itself, so every contender locks the same inode.
	err = errors.Join(err, s.lock.Close())
	s.closed = true
	return err
}
