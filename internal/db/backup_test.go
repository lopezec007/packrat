package db

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Before migrations are applied to a database that is in use, MigrateWith
// calls the function it was given: once, with the versions it is going from
// and to, while nothing has been changed yet.
func TestMigrateWithCallsBeforeOnADatabaseInUse(t *testing.T) {
	ctx := context.Background()
	migs, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	latest := len(migs)
	pool := scratchSchema(t)
	migrateTo(t, pool, latest-1)
	at := func() int {
		t.Helper()
		var v int
		if err := pool.QueryRow(ctx, `SELECT COALESCE(max(Version), 0) FROM Schema_Migrations`).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}

	calls := 0
	applied, err := MigrateWith(ctx, pool, func(ctx context.Context, from, to int) error {
		calls++
		if from != latest-1 || to != latest {
			t.Errorf("before(%d, %d); want (%d, %d)", from, to, latest-1, latest)
		}
		if got := at(); got != latest-1 {
			t.Errorf("the schema is at version %d when before is called; want %d (nothing applied yet)", got, latest-1)
		}
		return nil
	})
	if err != nil || len(applied) != 1 || calls != 1 {
		t.Fatalf("MigrateWith: applied %v, %v, before called %d times; want one migration, nil, once", applied, err, calls)
	}
	if got := at(); got != latest {
		t.Fatalf("the schema is at version %d after migrating; want %d", got, latest)
	}

	// Up to date: nothing to migrate, so nothing to do first.
	applied, err = MigrateWith(ctx, pool, func(context.Context, int, int) error { calls++; return nil })
	if err != nil || len(applied) != 0 || calls != 1 {
		t.Fatalf("MigrateWith on a current database: applied %v, %v, before called %d times in all; want none, nil, 1", applied, err, calls)
	}
}

// When the function fails, nothing is migrated.
func TestMigrateWithStopsWhenBeforeFails(t *testing.T) {
	ctx := context.Background()
	migs, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	pool := scratchSchema(t)
	migrateTo(t, pool, len(migs)-2)

	failure := errors.New("the disk is full")
	applied, err := MigrateWith(ctx, pool, func(context.Context, int, int) error { return failure })
	if !errors.Is(err, failure) || len(applied) != 0 {
		t.Fatalf("MigrateWith: applied %v, %v; want nothing applied and the failure", applied, err)
	}
	if !strings.Contains(err.Error(), "before upgrading the schema") {
		t.Errorf("the error does not say when it happened: %v", err)
	}
	var v int
	if err := pool.QueryRow(ctx, `SELECT max(Version) FROM Schema_Migrations`).Scan(&v); err != nil || v != len(migs)-2 {
		t.Fatalf("the schema is at version %d, %v; want %d (untouched)", v, err, len(migs)-2)
	}
	// With the failure out of the way it carries on from where it was.
	applied, err = MigrateWith(ctx, pool, func(context.Context, int, int) error { return nil })
	if err != nil || len(applied) != 2 {
		t.Fatalf("MigrateWith afterwards: applied %v, %v; want the two that were left", applied, err)
	}
}

// A fresh database has nothing to lose: it is migrated without the step before.
func TestMigrateWithSkipsBeforeOnAFreshDatabase(t *testing.T) {
	ctx := context.Background()
	pool := scratchSchema(t)
	calls := 0
	applied, err := MigrateWith(ctx, pool, func(context.Context, int, int) error { calls++; return errors.New("not wanted") })
	if err != nil || len(applied) == 0 || calls != 0 {
		t.Fatalf("MigrateWith on a fresh database: applied %v, %v, before called %d times; want every migration, nil, never", applied, err, calls)
	}
}

// Backup writes a dump that pg_restore can read, holding the tables and the
// rows that were there, under a name that says what it is; and leaves
// nothing behind when it fails.
func TestBackup(t *testing.T) {
	for _, tool := range []string{"pg_dump", "pg_restore"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not installed", tool)
		}
	}
	ctx := context.Background()
	pool := scratchSchema(t)
	if _, err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO Item_Categories (Name) VALUES ('Kept In The Backup')`); err != nil {
		t.Fatal(err)
	}
	var schema string
	if err := pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	dsn := os.Getenv("PACKRAT_TEST_DATABASE_URL")
	dir := filepath.Join(t.TempDir(), "backups", "nested") // made if it is not there

	// (Only this test's schema: other tests are making and dropping theirs in the same database.)
	path, err := backup(ctx, dsn, dir, "schema-3-to-4", "--schema="+schema)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if filepath.Dir(path) != dir || !strings.HasPrefix(filepath.Base(path), "packrat-") || !strings.HasSuffix(path, "-schema-3-to-4.dump") {
		t.Errorf("the dump is at %s; want %s/packrat-<time>-schema-3-to-4.dump", path, dir)
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 {
		t.Fatalf("the dump: %v, %v", info, err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("the dump's mode is %o; want 600 (it holds everything)", mode)
	}
	left, _ := filepath.Glob(filepath.Join(dir, "*.partial"))
	if len(left) != 0 {
		t.Errorf("left behind: %v", left)
	}
	// What is in it: the tables, and the row.
	toc, err := exec.Command("pg_restore", "--list", path).Output()
	if err != nil {
		t.Fatalf("pg_restore --list: %v", err)
	}
	for _, table := range []string{"list_contents", "items", "statuses", "compartments", "schema_migrations"} {
		if !strings.Contains(string(toc), "TABLE DATA "+schema+" "+table) {
			t.Errorf("the dump has no data for %s:\n%s", table, toc)
		}
	}
	sql, err := exec.Command("pg_restore", "--data-only", "--file=-", path).Output()
	if err != nil || !strings.Contains(string(sql), "Kept In The Backup") {
		t.Errorf("the dump does not hold the row that was inserted (%v)", err)
	}

	// A database that cannot be reached: an error that says so, and no file.
	bad := filepath.Join(t.TempDir(), "bad")
	if _, err := backup(ctx, "postgres:///no_such_database_packrat?host=/var/run/postgresql", bad, "x"); err == nil || !strings.Contains(err.Error(), "pg_dump") {
		t.Errorf("backup of a database that is not there: %v; want a pg_dump error", err)
	}
	if files, _ := os.ReadDir(bad); len(files) != 0 {
		t.Errorf("a failed backup left %d file(s) behind", len(files))
	}
	if _, err := backup(ctx, dsn, "", "x"); err == nil {
		t.Error("backup with no directory: no error")
	}
}
