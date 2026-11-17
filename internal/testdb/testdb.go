// Package testdb gives tests a migrated, empty (but for the seeded statuses) Packrat database.
//
// Tests need a PostgreSQL server: set PACKRAT_TEST_DATABASE_URL to a
// database the tests may create schemas in (they skip when it is unset), e.g.
//
//	PACKRAT_TEST_DATABASE_URL=postgres://packrat@127.0.0.1:5432/packrat_test go test ./...
//
// Every call to New works in its own throwaway schema, dropped when the test
// ends, so tests can run in parallel against one database.
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"packrat/internal/db"
	"packrat/internal/store"
)

// New returns a store over a fresh schema with all migrations applied.
func New(t testing.TB) *store.Store {
	t.Helper()
	return store.New(NewPool(t))
}

// NewPool is New without the store wrapper.
func NewPool(t testing.TB) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("PACKRAT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PACKRAT_TEST_DATABASE_URL is not set; skipping database test")
	}
	ctx := context.Background()

	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	schema := "t_" + hex.EncodeToString(b[:])

	admin, err := db.Open(ctx, dsn, "")
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatalf("create schema: %v", err)
	}

	pool, err := db.Open(ctx, dsn, schema)
	if err != nil {
		admin.Close()
		t.Fatalf("connect to test schema: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Logf("drop schema %s: %v", schema, err)
		}
		admin.Close()
	})
	if _, err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool
}
