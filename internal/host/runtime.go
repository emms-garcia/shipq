//go:build darwin || linux

package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/emms-garcia/shipq/internal/adapters/localapi"
	"github.com/emms-garcia/shipq/internal/application/ports"
	"github.com/emms-garcia/shipq/internal/domain"
	"github.com/emms-garcia/shipq/internal/domain/identity"
	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

const (
	activeProfileName = "active-profile-id"
	hostVersion       = "0.1.0"
	pidName           = "host.pid"
)

type Config struct {
	DataRoot         string
	RuntimeDirectory string
	ProfileID        identity.ID
	Status           ports.StatusReader
}

type Runtime struct {
	profile   *Profile
	server    *localapi.Server
	pidPath   string
	pidInfo   os.FileInfo
	closeOnce sync.Once
	closeErr  error
}

func Start(ctx context.Context, config Config) (*Runtime, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(config.DataRoot) {
		return nil, &domain.ValidationError{Field: "data_root", Message: "must be absolute"}
	}
	if !filepath.IsAbs(config.RuntimeDirectory) {
		return nil, &domain.ValidationError{Field: "runtime_directory", Message: "must be absolute"}
	}
	if config.ProfileID == (identity.ID{}) {
		return nil, &domain.ValidationError{Field: "profile_id", Message: "must be a nonzero UUID"}
	}
	if config.Status == nil {
		config.Status = StaticStatus(config.ProfileID)
	}
	status, err := config.Status.ReadStatus(ctx)
	if err != nil || status.ActiveProfileID != config.ProfileID {
		return nil, errors.New("status provider must describe the configured profile")
	}
	profile, err := OpenProfile(ctx, config.DataRoot, config.ProfileID)
	if err != nil {
		return nil, err
	}
	server, err := localapi.Start(config.RuntimeDirectory, localapi.NewHandler(config.Status))
	if err != nil {
		return nil, errors.Join(err, profile.Close())
	}
	pidPath := filepath.Join(config.RuntimeDirectory, pidName)
	pidFile, err := os.OpenFile(pidPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		_ = server.Shutdown(context.Background())
		return nil, errors.Join(err, profile.Close())
	}
	_, writeErr := fmt.Fprintf(pidFile, "%d\n", os.Getpid())
	pidInfo, statErr := pidFile.Stat()
	closeErr := pidFile.Close()
	if err := errors.Join(writeErr, statErr, closeErr); err != nil {
		_ = server.Shutdown(context.Background())
		return nil, errors.Join(err, profile.Close())
	}
	return &Runtime{profile: profile, server: server, pidPath: pidPath, pidInfo: pidInfo}, nil
}

func (r *Runtime) SocketPath() string { return r.server.SocketPath() }

func (r *Runtime) Wait() error { return r.server.Wait() }

func (r *Runtime) Shutdown(ctx context.Context) error {
	r.closeOnce.Do(func() {
		r.closeErr = r.server.Shutdown(ctx)
		if r.closeErr == nil {
			info, err := os.Lstat(r.pidPath)
			switch {
			case errors.Is(err, os.ErrNotExist):
				err = nil
			case err == nil && info.Mode().IsRegular() && os.SameFile(info, r.pidInfo):
				err = os.Remove(r.pidPath)
			case err == nil:
				err = errors.New("host pid path changed; refusing to remove replacement")
			}
			r.closeErr = errors.Join(err, r.profile.Close())
		}
	})
	return r.closeErr
}

type staticStatus struct{ profileID identity.ID }

func StaticStatus(profileID identity.ID) ports.StatusReader {
	return staticStatus{profileID: profileID}
}

func (s staticStatus) ReadStatus(context.Context) (ports.HostStatus, error) {
	return ports.HostStatus{
		HostVersion: hostVersion, ActiveProfileID: s.profileID, MonitorState: "unavailable",
		NotificationPermission: "unavailable", CoverageState: "unknown",
	}, nil
}

type NotificationCapability interface {
	NotificationStatus(context.Context) (string, error)
}

type capabilityStatus struct {
	profileID identity.ID
	platform  NotificationCapability
}

func CapabilityStatus(profileID identity.ID, platform NotificationCapability) ports.StatusReader {
	return capabilityStatus{profileID: profileID, platform: platform}
}

func (s capabilityStatus) ReadStatus(ctx context.Context) (ports.HostStatus, error) {
	permission := "unavailable"
	if s.platform != nil {
		if value, err := s.platform.NotificationStatus(ctx); err == nil {
			permission = value
		}
	}
	return ports.HostStatus{
		HostVersion: hostVersion, ActiveProfileID: s.profileID, MonitorState: "unavailable",
		NotificationPermission: permission, CoverageState: "unknown",
	}, nil
}

func DefaultConfig() (Config, error) {
	dataRoot := os.Getenv("SHIPQ_DATA_ROOT")
	if dataRoot == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return Config{}, fmt.Errorf("resolve application support: %w", err)
		}
		dataRoot = filepath.Join(base, "Shipq")
	}
	runtimeDirectory := os.Getenv("SHIPQ_RUNTIME_DIR")
	if runtimeDirectory == "" {
		runtimeDirectory = filepath.Join("/tmp", "shipq-"+strconv.Itoa(os.Getuid()))
	}
	if !filepath.IsAbs(dataRoot) || !filepath.IsAbs(runtimeDirectory) {
		return Config{}, errors.New("host paths must be absolute")
	}
	profileID, err := configuredProfileID(dataRoot)
	if err != nil {
		return Config{}, err
	}
	return Config{DataRoot: dataRoot, RuntimeDirectory: runtimeDirectory, ProfileID: profileID}, nil
}

func configuredProfileID(dataRoot string) (identity.ID, error) {
	if text := os.Getenv("SHIPQ_PROFILE_ID"); text != "" {
		return identity.ParseID(text)
	}
	if err := secureDirectory(dataRoot); err != nil {
		return identity.ID{}, err
	}
	path := filepath.Join(dataRoot, activeProfileName)
	read := func() (identity.ID, error) {
		file, err := os.OpenFile(path, os.O_RDONLY|unix.O_NOFOLLOW, 0)
		if err != nil {
			return identity.ID{}, err
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
			return identity.ID{}, errors.New("active profile file must be private and regular")
		}
		data, err := io.ReadAll(io.LimitReader(file, 65))
		if err != nil {
			return identity.ID{}, err
		}
		if len(data) > 64 {
			return identity.ID{}, errors.New("active profile file is too large")
		}
		return identity.ParseID(strings.TrimSpace(string(data)))
	}
	if id, err := read(); err == nil {
		return id, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return identity.ID{}, err
	}
	id := identity.ID(uuid.New())
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|unix.O_NOFOLLOW, 0o600)
	if errors.Is(err, os.ErrExist) {
		return read()
	}
	if err != nil {
		return identity.ID{}, fmt.Errorf("create active profile: %w", err)
	}
	_, writeErr := file.WriteString(id.String() + "\n")
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return identity.ID{}, fmt.Errorf("persist active profile: %w", err)
	}
	return id, nil
}

func secureDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create data root: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("data root must be a nonsymlink directory")
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return err
	}
	return nil
}
