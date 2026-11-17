// Package db opens the PostgreSQL connection pool and applies the embedded
// schema migrations.
//
// Migrations are the numbered .sql files in migrations/, applied in order,
// each in its own transaction, and recorded in the Schema_Migrations table.
// A released migration is never edited: schema changes are new files.
package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Open connects to the database at dsn (a postgres:// URL or a key=value
// connection string). searchPath, when not empty, replaces the session's
// search_path (the tests use it to work inside a throwaway schema).
func Open(ctx context.Context, dsn string, searchPath string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams["application_name"] = "packrat"
	if searchPath != "" {
		cfg.ConnConfig.RuntimeParams["search_path"] = searchPath
	}
	if cfg.MaxConns < 4 {
		cfg.MaxConns = 8
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

type migration struct {
	version int
	name    string
	sql     string
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return nil, err
	}
	var out []migration
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		num, _, ok := strings.Cut(name, "_")
		v, err := strconv.Atoi(num)
		if !ok || err != nil {
			return nil, fmt.Errorf("migration %q: name must start with a number and an underscore", name)
		}
		body, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return nil, err
		}
		out = append(out, migration{version: v, name: name, sql: string(body)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	for i, m := range out {
		if m.version != i+1 {
			return nil, fmt.Errorf("migration %q: expected version %d (versions must be 1, 2, 3, ...)", m.name, i+1)
		}
	}
	return out, nil
}

// migrationLock is the advisory lock that keeps two starting processes from
// migrating at once ("PKRT" in ASCII).
const migrationLock = 0x504b5254

// Migrate brings the schema up to date and returns the names of the
// migrations it applied (none when already current).
func Migrate(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	migs, err := loadMigrations()
	if err != nil {
		return nil, err
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrationLock); err != nil {
		return nil, err
	}
	defer conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, migrationLock)

	if _, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS Schema_Migrations (
		    Version     int          NOT NULL,
		    Name        varchar(127) NOT NULL,
		    Applied_UTC timestamp(0) NOT NULL DEFAULT (now() AT TIME ZONE 'UTC'),
		    CONSTRAINT PK_Schema_Migrations PRIMARY KEY (Version)
		)`); err != nil {
		return nil, fmt.Errorf("create Schema_Migrations: %w", err)
	}

	var current int
	if err := conn.QueryRow(ctx, `SELECT COALESCE(max(Version), 0) FROM Schema_Migrations`).Scan(&current); err != nil {
		return nil, err
	}
	if current > len(migs) {
		return nil, fmt.Errorf("database is at schema version %d but this build only knows up to %d; install a newer packrat", current, len(migs))
	}

	var applied []string
	for _, m := range migs[current:] {
		err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			// The simple protocol lets one Exec carry a whole multi-statement file.
			if _, err := tx.Conn().PgConn().Exec(ctx, m.sql).ReadAll(); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO Schema_Migrations (Version, Name) VALUES ($1, $2)`, m.version, m.name)
			return err
		})
		if err != nil {
			return applied, fmt.Errorf("migration %s: %w", m.name, err)
		}
		applied = append(applied, m.name)
	}
	return applied, nil
}
