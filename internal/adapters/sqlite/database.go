package sqlite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/emms-garcia/shipq/internal/domain"
	"github.com/emms-garcia/shipq/internal/domain/identity"
	"golang.org/x/sys/unix"
	_ "modernc.org/sqlite"
)

var (
	ErrProfileMismatch   = errors.New("database belongs to another profile")
	ErrUnsupportedSchema = errors.New("unsupported database schema version")
	ErrMigrationChanged  = errors.New("applied migration has changed")
)

//go:embed migrations/*.sql
var migrations embed.FS

type Database struct {
	db        *sql.DB
	closeOnce sync.Once
	closeErr  error
}

// Open requires the host to hold exclusive profile ownership until Close.
func Open(ctx context.Context, path string, profileID identity.ID) (*Database, error) {
	return openWithMigrations(ctx, path, profileID, migrations)
}

func openWithMigrations(ctx context.Context, path string, profileID identity.ID, source fs.FS) (*Database, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(path) {
		return nil, &domain.ValidationError{Field: "database_path", Message: "must be absolute"}
	}
	if profileID == (identity.ID{}) {
		return nil, &domain.ValidationError{Field: "profile_id", Message: "must be a nonzero UUID"}
	}
	pending, err := loadMigrations(source)
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open database file: %w", err)
	}
	info, err := file.Stat()
	if err == nil && !info.Mode().IsRegular() {
		err = errors.New("database must be a regular file")
	}
	if err == nil {
		err = file.Chmod(0o600)
	}
	err = errors.Join(err, file.Close())
	if err != nil {
		return nil, fmt.Errorf("prepare database file: %w", err)
	}
	dsn := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	query := url.Values{}
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "busy_timeout(1000)")
	dsn.RawQuery = query.Encode()
	db, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		return nil, fmt.Errorf("open SQLite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &Database{db: db}
	ready := false
	defer func() {
		if !ready {
			_ = store.Close()
		}
	}()
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("connect SQLite: %w", err)
	}
	var journal string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode=WAL").Scan(&journal); err != nil {
		return nil, fmt.Errorf("enable WAL: %w", err)
	}
	if journal != "wal" {
		return nil, errors.New("SQLite did not enable WAL")
	}
	if err := migrate(ctx, db, profileID, pending); err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return nil, fmt.Errorf("initialize database: %w", err)
	}
	ready = true
	return store, nil
}

func (d *Database) Close() error {
	d.closeOnce.Do(func() {
		// Reserve the sole connection before closing the pool. An outstanding
		// transaction must finish before the host can release profile ownership.
		connection, err := d.db.Conn(context.Background())
		if err != nil {
			d.closeErr = errors.Join(err, d.db.Close())
			return
		}
		d.closeErr = errors.Join(d.db.Close(), connection.Close())
	})
	return d.closeErr
}

type migration struct {
	version  int
	name     string
	sql      string
	checksum string
}

func loadMigrations(source fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(source, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	if len(entries) == 0 {
		return nil, errors.New("no embedded migrations")
	}
	result := make([]migration, 0, len(entries))
	for i, entry := range entries {
		prefix, suffix, ok := strings.Cut(entry.Name(), "_")
		version, err := strconv.Atoi(prefix)
		if entry.IsDir() || !ok || len(prefix) != 3 || !strings.HasSuffix(suffix, ".sql") ||
			suffix == ".sql" || err != nil || version != i+1 {
			return nil, fmt.Errorf("migration %q must have a consecutive NNN_name.sql version", entry.Name())
		}
		content, err := fs.ReadFile(source, "migrations/"+entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", entry.Name(), err)
		}
		if strings.TrimSpace(string(content)) == "" {
			return nil, fmt.Errorf("migration %q is empty", entry.Name())
		}
		result = append(result, migration{
			version: version, name: entry.Name(), sql: string(content),
			checksum: fmt.Sprintf("%x", sha256.Sum256(content)),
		})
	}
	return result, nil
}

func migrate(ctx context.Context, db *sql.DB, profileID identity.ID, migrations []migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY CHECK(version > 0),
		name TEXT NOT NULL UNIQUE,
		checksum TEXT NOT NULL CHECK(length(checksum) = 64),
		applied_at TEXT NOT NULL
	) STRICT`); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, "SELECT version, name, checksum FROM schema_migrations ORDER BY version")
	if err != nil {
		return err
	}
	applied := 0
	for rows.Next() {
		var version int
		var name, checksum string
		if err := rows.Scan(&version, &name, &checksum); err != nil {
			_ = rows.Close()
			return err
		}
		if version > len(migrations) {
			_ = rows.Close()
			return ErrUnsupportedSchema
		}
		if version != applied+1 {
			_ = rows.Close()
			return errors.New("database migration journal is not consecutive")
		}
		expected := migrations[applied]
		if name != expected.name || checksum != expected.checksum {
			_ = rows.Close()
			return ErrMigrationChanged
		}
		applied++
	}
	err = errors.Join(rows.Err(), rows.Close())
	if err != nil {
		return err
	}
	for _, step := range migrations[applied:] {
		if _, err := tx.ExecContext(ctx, step.sql); err != nil {
			return fmt.Errorf("apply migration %s: %w", step.name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version,name,checksum,applied_at)
			VALUES(?,?,?,strftime('%Y-%m-%dT%H:%M:%fZ','now'))`, step.version, step.name, step.checksum); err != nil {
			return err
		}
	}
	var stored []byte
	err = tx.QueryRowContext(ctx, "SELECT profile_id FROM profile_metadata WHERE singleton=1").Scan(&stored)
	switch {
	case errors.Is(err, sql.ErrNoRows) && applied == 0:
		if _, err := tx.ExecContext(ctx, "INSERT INTO profile_metadata(singleton,profile_id) VALUES(1,?)", profileID[:]); err != nil {
			return err
		}
	case err != nil:
		return fmt.Errorf("read profile binding: %w", err)
	case !bytes.Equal(stored, profileID[:]):
		return ErrProfileMismatch
	}
	return tx.Commit()
}
