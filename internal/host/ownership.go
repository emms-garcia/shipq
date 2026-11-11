//go:build darwin || linux

package host

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/emms-garcia/shipq/internal/adapters/sqlite"
	"github.com/emms-garcia/shipq/internal/domain"
	"github.com/emms-garcia/shipq/internal/domain/identity"
	"golang.org/x/sys/unix"
)

const (
	databaseName  = "shipq.sqlite3"
	ownershipName = "host.lock"
)

var ErrProfileOwned = errors.New("profile already owned by another host")

type Ownership struct {
	file      *os.File
	closeOnce sync.Once
	closeErr  error
}

func AcquireOwnership(directory string) (*Ownership, error) {
	file, err := os.OpenFile(filepath.Join(directory, ownershipName), os.O_CREATE|os.O_RDWR|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open profile ownership: %w", err)
	}
	info, err := file.Stat()
	if err == nil && !info.Mode().IsRegular() {
		err = errors.New("ownership must use a regular file")
	}
	if err == nil {
		err = file.Chmod(0o600)
	}
	if err == nil {
		err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	}
	if err != nil {
		_ = file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, ErrProfileOwned
		}
		return nil, fmt.Errorf("acquire profile ownership: %w", err)
	}
	return &Ownership{file: file}, nil
}

func (o *Ownership) Close() error {
	// Never unlink the lock file: existing contenders must share its inode.
	o.closeOnce.Do(func() { o.closeErr = o.file.Close() })
	return o.closeErr
}

type Profile struct {
	Database  *sqlite.Database
	ownership *Ownership
	closeOnce sync.Once
	closeErr  error
}

func OpenProfile(ctx context.Context, root string, profileID identity.ID) (*Profile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(root) {
		return nil, &domain.ValidationError{Field: "data_root", Message: "must be absolute"}
	}
	if profileID == (identity.ID{}) {
		return nil, &domain.ValidationError{Field: "profile_id", Message: "must be a nonzero UUID"}
	}
	directory := filepath.Join(root, profileID.String())
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create profile directory: %w", err)
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("profile directory must not be a symlink or other file")
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return nil, fmt.Errorf("secure profile directory: %w", err)
	}
	ownership, err := AcquireOwnership(directory)
	if err != nil {
		return nil, err
	}
	database, err := sqlite.Open(ctx, filepath.Join(directory, databaseName), profileID)
	if err != nil {
		return nil, errors.Join(err, ownership.Close())
	}
	return &Profile{Database: database, ownership: ownership}, nil
}

func (p *Profile) Close() error {
	p.closeOnce.Do(func() {
		p.closeErr = p.Database.Close()
		if p.closeErr == nil {
			p.closeErr = p.ownership.Close()
		}
	})
	return p.closeErr
}
