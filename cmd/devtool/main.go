// Command devtool prepares throwaway databases for development and the
// end-to-end tests. It is not part of the installed service.
//
//	go run ./cmd/devtool reset <database-url>   drop everything in the database and re-apply the migrations
//	go run ./cmd/devtool demo  <database-url>   load scripts/demo-data.sql (a made-up library and a few lists)
//
// Both refuse to touch a database whose name does not end in _dev, _test or
// _e2e, so a slip of the hand cannot wipe the real lists.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"packrat/internal/db"
)

func main() {
	if len(os.Args) != 3 || (os.Args[1] != "reset" && os.Args[1] != "demo") {
		fmt.Fprintln(os.Stderr, "usage: devtool reset|demo <database-url>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "devtool:", err)
		os.Exit(1)
	}
}

func run(cmd, dsn string) error {
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return err
	}
	name := cfg.ConnConfig.Database
	if !strings.HasSuffix(name, "_dev") && !strings.HasSuffix(name, "_test") && !strings.HasSuffix(name, "_e2e") {
		return fmt.Errorf("refusing to touch database %q: its name must end in _dev, _test or _e2e", name)
	}
	pool, err := db.Open(ctx, dsn, "")
	if err != nil {
		return err
	}
	defer pool.Close()

	switch cmd {
	case "reset":
		if _, err := pool.Exec(ctx, "DROP SCHEMA public CASCADE"); err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, "CREATE SCHEMA public"); err != nil {
			return err
		}
		applied, err := db.Migrate(ctx, pool)
		if err != nil {
			return err
		}
		fmt.Printf("reset %s (%d migrations applied)\n", name, len(applied))
	case "demo":
		if _, err := db.Migrate(ctx, pool); err != nil {
			return err
		}
		sql, err := os.ReadFile("scripts/demo-data.sql")
		if err != nil {
			return fmt.Errorf("%w (run from the repository root)", err)
		}
		conn, err := pool.Acquire(ctx)
		if err != nil {
			return err
		}
		defer conn.Release()
		if _, err := conn.Conn().PgConn().Exec(ctx, string(sql)).ReadAll(); err != nil {
			return fmt.Errorf("scripts/demo-data.sql: %w", err)
		}
		fmt.Printf("loaded demo data into %s\n", name)
	}
	return nil
}
