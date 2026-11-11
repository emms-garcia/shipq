package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/emms-garcia/shipq/internal/domain/identity"
	_ "modernc.org/sqlite"
)

func testID(t *testing.T, value string) identity.ID {
	t.Helper()
	id, err := identity.ParseID(value)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func firstID(t *testing.T) identity.ID {
	return testID(t, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
}

func secondID(t *testing.T) identity.ID {
	return testID(t, "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")
}

func openTest(t *testing.T, path string, id identity.ID) *Database {
	t.Helper()
	db, err := Open(context.Background(), path, id)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	return db
}

func execSQL(t *testing.T, db *sql.DB, statement string, args ...any) {
	t.Helper()
	if _, err := db.Exec(statement, args...); err != nil {
		t.Fatal(err)
	}
}

func scalar[T any](t *testing.T, db *sql.DB, statement string, args ...any) T {
	t.Helper()
	var result T
	if err := db.QueryRow(statement, args...).Scan(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

const insertRepository = `INSERT INTO repositories (github_host, node_id, owner_login, name, organization_node_id)
	VALUES ('github.com', ?, 'fixture', 'core', 'ORG')`

func TestInitializationAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache #1?mode=memory%.sqlite3")
	id := firstID(t)
	db := openTest(t, path, id)
	if got := scalar[string](t, db.db, "PRAGMA journal_mode"); got != "wal" {
		t.Fatalf("journal mode %q", got)
	}
	if got := scalar[int](t, db.db, "PRAGMA foreign_keys"); got != 1 {
		t.Fatal("foreign keys disabled")
	}
	if got := scalar[int](t, db.db, "PRAGMA busy_timeout"); got <= 0 || got > 5000 {
		t.Fatalf("unbounded busy timeout: %d", got)
	}
	if got := scalar[int](t, db.db, "SELECT count(*) FROM schema_migrations"); got != 1 {
		t.Fatalf("migration count %d", got)
	}
	stored := scalar[[]byte](t, db.db, "SELECT profile_id FROM profile_metadata")
	if string(stored) != string(id[:]) {
		t.Fatal("profile UUID not retained")
	}
	execSQL(t, db.db, insertRepository, "REPO")
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openTest(t, path, id)
	if scalar[int](t, reopened.db, "SELECT count(*) FROM repositories") != 1 ||
		scalar[int](t, reopened.db, "SELECT count(*) FROM schema_migrations") != 1 {
		t.Fatal("reopen reran migration or lost cache data")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 || info.Size() == 0 {
		t.Fatalf("database permissions: %v, %v", info, err)
	}
	// Force replacement connections, so connection-scoped pragmas cannot be
	// accidentally configured only on the first connection.
	reopened.db.SetMaxIdleConns(0)
	if scalar[int](t, reopened.db, "PRAGMA foreign_keys") != 1 ||
		scalar[int](t, reopened.db, "PRAGMA busy_timeout") <= 0 {
		t.Fatal("replacement connection lost pragmas")
	}
}

func TestProfileIsolationAndBinding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.sqlite3")
	a := openTest(t, path, firstID(t))
	b := openTest(t, filepath.Join(t.TempDir(), "b.sqlite3"), secondID(t))
	execSQL(t, a.db, insertRepository, "REPO")
	if scalar[int](t, b.db, "SELECT count(*) FROM repositories") != 0 {
		t.Fatal("cache leaked between profiles")
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := Open(context.Background(), path, secondID(t))
	if got != nil || !errors.Is(err, ErrProfileMismatch) {
		t.Fatalf("identity replacement accepted: %v, %v", got, err)
	}
	a = openTest(t, path, firstID(t))
	if scalar[int](t, a.db, "SELECT count(*) FROM repositories") != 1 {
		t.Fatal("failed profile binding modified existing cache")
	}
}

func TestDatabaseConstraints(t *testing.T) {
	db := openTest(t, filepath.Join(t.TempDir(), "cache.sqlite3"), firstID(t))
	otherID := secondID(t)
	execSQL(t, db.db, insertRepository, "REPO")
	for _, tc := range []struct {
		name string
		sql  string
		args []any
	}{
		{"duplicate_remote_identity", insertRepository, []any{"REPO"}},
		{"empty_remote_identity", insertRepository, []any{""}},
		{"noncanonical_host", `INSERT INTO repositories(github_host,node_id,owner_login,name) VALUES('GitHub.com','OTHER','fixture','core')`, nil},
		{"invalid_authorization", `UPDATE repositories SET authorization_state='invented'`, nil},
		{"orphan_progress", `INSERT INTO repository_refresh_progress(github_host,repository_node_id) VALUES('github.com','MISSING')`, nil},
		{"invalid_progress", `INSERT INTO repository_refresh_progress(github_host,repository_node_id,state) VALUES('github.com','REPO','invented')`, nil},
		{"second_profile", `INSERT INTO profile_metadata(singleton,profile_id) VALUES(2,?)`, []any{otherID[:]}},
		{"invalid_uuid", `UPDATE profile_metadata SET profile_id=?`, []any{[]byte("not-a-uuid")}},
		{"nil_uuid", `UPDATE profile_metadata SET profile_id=zeroblob(16)`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := db.db.Exec(tc.sql, tc.args...); err == nil {
				t.Fatal("invalid persistent state accepted")
			}
		})
	}
	execSQL(t, db.db, `INSERT INTO repositories(github_host,node_id,owner_login,name) VALUES('ghe.example','REPO','fixture','core')`)
	execSQL(t, db.db, `INSERT INTO repository_refresh_progress(github_host,repository_node_id) VALUES('github.com','REPO')`)
	execSQL(t, db.db, `DELETE FROM repositories WHERE github_host='github.com' AND node_id='REPO'`)
	if scalar[int](t, db.db, "SELECT count(*) FROM repository_refresh_progress") != 0 ||
		scalar[int](t, db.db, "SELECT count(*) FROM repositories") != 1 {
		t.Fatal("foreign-key cascade confused host-scoped identities")
	}
}

func migrationSource(t *testing.T, extra string) fstest.MapFS {
	t.Helper()
	base, err := fs.ReadFile(migrations, "migrations/001_base.sql")
	if err != nil {
		t.Fatal(err)
	}
	result := fstest.MapFS{"migrations/001_base.sql": &fstest.MapFile{Data: base}}
	if extra != "" {
		result["migrations/002_upgrade.sql"] = &fstest.MapFile{Data: []byte(extra)}
	}
	return result
}

func TestMigrationsAreAtomicAndOrdered(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "fresh_database", true: "existing_database"}[existing], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "cache.sqlite3")
			if existing {
				db := openTest(t, path, firstID(t))
				execSQL(t, db.db, insertRepository, "SAVED")
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
			}
			broken := migrationSource(t, `CREATE TABLE upgrade_probe(value TEXT); INSERT INTO upgrade_probe VALUES('partial'); INVALID SQL;`)
			db, err := openWithMigrations(context.Background(), path, firstID(t), broken)
			if db != nil || err == nil {
				t.Fatalf("failed migration exposed ready database: %v, %v", db, err)
			}
			if !existing {
				raw, err := sql.Open("sqlite", (&url.URL{Scheme: "file", Path: path}).String())
				if err != nil {
					t.Fatal(err)
				}
				count := scalar[int](t, raw, "SELECT count(*) FROM sqlite_master WHERE type='table'")
				if err := raw.Close(); err != nil {
					t.Fatal(err)
				}
				if count != 0 {
					t.Fatal("fresh initialization partially committed its pending migrations")
				}
			}
			recovered := openTest(t, path, firstID(t))
			if scalar[int](t, recovered.db, "SELECT count(*) FROM sqlite_master WHERE name='upgrade_probe'") != 0 ||
				scalar[int](t, recovered.db, "SELECT count(*) FROM schema_migrations") != 1 {
				t.Fatal("failed migration left partial schema or journal state")
			}
			want := 0
			if existing {
				want = 1
			}
			if scalar[int](t, recovered.db, "SELECT count(*) FROM repositories") != want {
				t.Fatal("failed migration corrupted existing data")
			}
			if err := recovered.Close(); err != nil {
				t.Fatal(err)
			}
			valid := migrationSource(t, `CREATE TABLE upgrade_probe(value TEXT NOT NULL); INSERT INTO upgrade_probe SELECT node_id FROM repositories;`)
			upgraded, err := openWithMigrations(context.Background(), path, firstID(t), valid)
			if err != nil {
				t.Fatal(err)
			}
			if scalar[int](t, upgraded.db, "SELECT count(*) FROM schema_migrations") != 2 ||
				scalar[int](t, upgraded.db, "SELECT count(*) FROM upgrade_probe") != want {
				t.Fatal("upgrade did not observe preceding migration and retained data")
			}
			if err := upgraded.Close(); err != nil {
				t.Fatal(err)
			}
			db, err = Open(context.Background(), path, firstID(t))
			if db != nil || !errors.Is(err, ErrUnsupportedSchema) {
				t.Fatalf("downgrade silently accepted: %v, %v", db, err)
			}
		})
	}
}

func TestInterruptedMigrationDoesNotPublishSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.sqlite3")
	source := migrationSource(t, `CREATE TABLE interrupted(value TEXT);
		WITH RECURSIVE forever(n) AS (VALUES(0) UNION ALL SELECT n+1 FROM forever) SELECT max(n) FROM forever;`)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	db, err := openWithMigrations(ctx, path, firstID(t), source)
	if db != nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("interrupted initialization returned %v, %v", db, err)
	}
	recovered := openTest(t, path, firstID(t))
	if scalar[int](t, recovered.db, "SELECT count(*) FROM sqlite_master WHERE name='interrupted'") != 0 {
		t.Fatal("interrupted DDL survived rollback")
	}
}

func TestMigrationIntegrityAndInvalidSources(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.sqlite3")
	db := openTest(t, path, firstID(t))
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	changed := migrationSource(t, "")
	changed["migrations/001_base.sql"].Data = append(changed["migrations/001_base.sql"].Data, []byte("\n-- changed")...)
	result, err := openWithMigrations(context.Background(), path, firstID(t), changed)
	if result != nil || !errors.Is(err, ErrMigrationChanged) {
		t.Fatalf("changed applied migration accepted: %v, %v", result, err)
	}
	for _, source := range []fs.FS{
		fstest.MapFS{},
		fstest.MapFS{"migrations/002_gap.sql": &fstest.MapFile{Data: []byte("SELECT 1;")}},
		fstest.MapFS{"migrations/bad.sql": &fstest.MapFile{Data: []byte("SELECT 1;")}},
	} {
		if result, err := openWithMigrations(context.Background(), filepath.Join(t.TempDir(), "invalid.sqlite3"), firstID(t), source); result != nil || err == nil {
			t.Fatalf("invalid migration source accepted: %v, %v", result, err)
		}
	}
}

func TestInvalidInitialization(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name string
		ctx  context.Context
		path string
		id   identity.ID
	}{
		{"cancelled", cancelled, filepath.Join(t.TempDir(), "cancelled.sqlite3"), firstID(t)},
		{"missing_path", context.Background(), "", firstID(t)},
		{"relative_path", context.Background(), "relative.sqlite3", firstID(t)},
		{"nil_profile", context.Background(), filepath.Join(t.TempDir(), "invalid.sqlite3"), identity.ID{}},
		{"directory", context.Background(), t.TempDir(), firstID(t)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if db, err := Open(tc.ctx, tc.path, tc.id); db != nil || err == nil {
				t.Fatalf("invalid initialization accepted: %v, %v", db, err)
			}
		})
	}
}

func TestWALReadersAndBoundedWriterWait(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.sqlite3")
	db := openTest(t, path, firstID(t))
	execSQL(t, db.db, insertRepository, "BEFORE")
	tx, err := db.db.Begin()
	if err != nil {
		t.Fatal(err)
	}

	defer tx.Rollback()
	if _, err := tx.Exec(insertRepository, "UNCOMMITTED"); err != nil {
		t.Fatal(err)
	}
	dsn := (&url.URL{Scheme: "file", Path: path}).String()
	reader, err := sql.Open("sqlite", dsn+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if scalar[int](t, reader, "SELECT count(*) FROM repositories") != 1 {
		t.Fatal("WAL reader observed uncommitted changes")
	}
	writer, err := sql.Open("sqlite", dsn+"?_pragma=busy_timeout(30)")
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	start := time.Now()
	if _, err := writer.Exec(insertRepository, "CONTENDING"); err == nil {
		t.Fatal("second writer bypassed transaction lock")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("writer wait was not bounded")
	}
}

func TestCloseWaitsForOutstandingTransaction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.sqlite3")
	db := openTest(t, path, firstID(t))
	tx, err := db.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(insertRepository, "COMMITTED_BEFORE_CLOSE"); err != nil {
		t.Fatal(err)
	}
	closed := make(chan error, 1)
	go func() { closed <- db.Close() }()
	timeout := time.NewTimer(2 * time.Second)
	defer timeout.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for db.db.Stats().WaitCount == 0 {
		select {
		case err := <-closed:
			t.Fatalf("pool closed while a transaction could still write: %v", err)
		case <-timeout.C:
			t.Fatal("close did not wait for outstanding connection")
		case <-ticker.C:
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-timeout.C:
		t.Fatal("close did not finish after transaction committed")
	}
	if _, err := db.db.Exec(insertRepository, "TOO_LATE"); err == nil {
		t.Fatal("closed pool accepted another write")
	}
	reopened := openTest(t, path, firstID(t))
	if scalar[int](t, reopened.db, "SELECT count(*) FROM repositories") != 1 {
		t.Fatal("close lost an outstanding transaction")
	}
}

func TestDatabaseRejectsSymlink(t *testing.T) {
	target := filepath.Join(t.TempDir(), "unrelated")
	if err := os.WriteFile(target, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "cache.sqlite3")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if db, err := Open(context.Background(), path, firstID(t)); db != nil || err == nil {
		t.Fatal("database followed a symlink")
	}
	data, _ := os.ReadFile(target)
	if string(data) != "untouched" {
		t.Fatal("initialization modified unrelated target")
	}
}
