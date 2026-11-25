package db

// Backing the database up before its schema is changed: a pg_dump of the
// whole database into a file, made by the service itself just before it
// applies migrations (main.go passes Backup to MigrateWith).
//
// The dump is PostgreSQL's custom format. To go back to it:
//
//	pg_restore --clean --if-exists --no-owner -d packrat <file>
//
// (and install the packrat binary the dump was made with: its name says
// which schema version it holds).

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Backup dumps the database at dsn into dir and returns the file it wrote:
// dir/packrat-<UTC time>-<label>.dump. The file only appears under that name
// once pg_dump has finished and the dump has been read back, so a file that
// is there is a whole one.
//
// It needs pg_dump and pg_restore (Debian: postgresql-client, which comes
// with postgresql) no older than the server.
func Backup(ctx context.Context, dsn, dir, label string) (string, error) {
	return backup(ctx, dsn, dir, label)
}

// backup is Backup with further arguments for pg_dump (the tests use them to
// dump one schema only).
func backup(ctx context.Context, dsn, dir, label string, extra ...string) (string, error) {
	pgDump, err := exec.LookPath("pg_dump")
	if err != nil {
		return "", errors.New("pg_dump was not found: install the PostgreSQL client programs (Debian: apt install postgresql-client)")
	}
	if dir == "" {
		return "", errors.New("no backup directory is set")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create the backup directory: %w", err)
	}
	name := fmt.Sprintf("packrat-%s-%s.dump", time.Now().UTC().Format("20060102T150405Z"), label)
	path := filepath.Join(dir, name)
	partial := path + ".partial"
	defer os.Remove(partial) // gone already when all went well

	args := append([]string{"--format=custom", "--file=" + partial, "--dbname=" + dsn}, extra...)
	if out, err := run(ctx, pgDump, args...); err != nil {
		return "", fmt.Errorf("pg_dump: %w%s", err, out)
	}
	info, err := os.Stat(partial)
	if err != nil || info.Size() == 0 {
		return "", fmt.Errorf("pg_dump left no dump in %s", dir)
	}
	// Read it back: a dump that cannot be listed could not be restored.
	if pgRestore, err := exec.LookPath("pg_restore"); err == nil {
		if out, err := run(ctx, pgRestore, "--list", partial); err != nil {
			return "", fmt.Errorf("the dump cannot be read back (pg_restore --list): %w%s", err, out)
		}
	}
	if err := os.Chmod(partial, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(partial, path); err != nil {
		return "", err
	}
	return path, nil
}

// run runs a program and returns what it wrote to stderr, as ": text" ready
// to follow an error (or "" when it said nothing).
func run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if msg := strings.TrimSpace(stderr.String()); msg != "" {
		return ": " + msg, err
	}
	return "", err
}
