package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// These tests stop the migrations part of the way, to see what a later one
// does to a database that already has data in it. They are in the package
// itself for loadMigrations (and so cannot use internal/testdb, which
// imports this package).

// scratchSchema is testdb.NewPool without the migrations: a pool over a
// fresh, empty schema that is dropped when the test ends.
func scratchSchema(t *testing.T) *pgxpool.Pool {
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
	admin, err := Open(ctx, dsn, "")
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatalf("create schema: %v", err)
	}
	pool, err := Open(ctx, dsn, schema)
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
	return pool
}

// migrateTo applies the first n migrations as Migrate does, and records
// them as Migrate does, so that Migrate carries on from there.
func migrateTo(t *testing.T, pool *pgxpool.Pool, n int) {
	t.Helper()
	ctx := context.Background()
	migs, err := loadMigrations()
	if err != nil || len(migs) < n {
		t.Fatalf("%d migrations, %v; want at least %d", len(migs), err, n)
	}
	if _, err := pool.Exec(ctx, `
		CREATE TABLE Schema_Migrations (
		    Version     int          NOT NULL,
		    Name        varchar(127) NOT NULL,
		    Applied_UTC timestamp(0) NOT NULL DEFAULT (now() AT TIME ZONE 'UTC'),
		    CONSTRAINT PK_Schema_Migrations PRIMARY KEY (Version)
		)`); err != nil {
		t.Fatal(err)
	}
	for _, m := range migs[:n] {
		err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			if _, err := tx.Conn().PgConn().Exec(ctx, m.sql).ReadAll(); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO Schema_Migrations (Version, Name) VALUES ($1, $2)`, m.version, m.name)
			return err
		})
		if err != nil {
			t.Fatalf("migration %s: %v", m.name, err)
		}
	}
}

// 0004_status_rank.sql on a database that has been in use: whatever statuses
// there are get the ranks 1, 2, 3, ... with those named "Missing" first,
// those named "Packed" last (whatever the case of the name), and the rest
// between them in the order they were made.
func TestStatusRankMigrationRanksTheStatusesThereAre(t *testing.T) {
	const before = 3 // the migrations applied first: 0004 is the next
	migs, err := loadMigrations()
	if err != nil || len(migs) <= before || migs[before].name != "0004_status_rank.sql" {
		t.Fatalf("migration %d should be 0004_status_rank.sql: %v, %v", before+1, migs, err)
	}
	var rest []string
	for _, m := range migs[before:] {
		rest = append(rest, m.name)
	}

	cases := []struct {
		name  string
		setup []string // run against schema version 3, which has the seeded statuses 1 to 3
		want  []string // the names, from rank 1 up
	}{
		{"the seeded three", nil,
			[]string{"Missing", "In Storage Location", "Packed"}},
		{"statuses added since",
			[]string{`INSERT INTO Statuses (Name, Color, Shortcut) VALUES
				('Bought', '112233', 'b'), ('PACKED', '112233', 'k'), ('Worn', '112233', 'w'), ('missing', '112233', 'x'), ('Packed Away', '112233', 'y')`},
			[]string{"Missing", "missing", "In Storage Location", "Bought", "Worn", "Packed Away", "Packed", "PACKED"}},
		{"renamed and deleted",
			[]string{
				`DELETE FROM Statuses WHERE ID = 1`,
				`UPDATE Statuses SET Name = 'Lost' WHERE ID = 3`,
				`INSERT INTO Statuses (Name, Color, Shortcut) VALUES ('Bought', '112233', 'b')`,
				`UPDATE Statuses SET Name = 'Done' WHERE ID = 2`,
			},
			[]string{"Done", "Lost", "Bought"}},
		{"only the best is left",
			[]string{`DELETE FROM Statuses WHERE ID <> 2`},
			[]string{"Packed"}},
		{"no statuses at all",
			[]string{`DELETE FROM Statuses`},
			[]string{}},
		// Statuses in use are ranked like any others, and stay in use.
		{"in use on a list",
			[]string{
				`INSERT INTO Item_Categories (Name) VALUES ('Gear')`,
				`INSERT INTO Storage_Locations (Name) VALUES ('Garage')`,
				`INSERT INTO Items (Item_Category_ID, Name, Storage_Location_ID, Is_Container) VALUES (1, 'socks', 1, false)`,
				`INSERT INTO Lists (Name, Is_Template) VALUES ('Trip', false)`,
				`INSERT INTO Statuses (Name, Color, Shortcut) VALUES ('Bought', '112233', 'b')`,
				`INSERT INTO List_Contents (List_ID, Item_ID, Quantity, Status_ID) VALUES (1, 1, 1, 4)`,
			},
			[]string{"Missing", "In Storage Location", "Bought", "Packed"}},
	}
	ctx := context.Background()
	for _, c := range cases {
		pool := scratchSchema(t)
		migrateTo(t, pool, before)
		for _, sql := range c.setup {
			if _, err := pool.Exec(ctx, sql); err != nil {
				t.Fatalf("%s: %s: %v", c.name, sql, err)
			}
		}

		applied, err := Migrate(ctx, pool)
		if err != nil || !reflect.DeepEqual(applied, rest) {
			t.Fatalf("%s: Migrate applied %v, %v; want %v", c.name, applied, err, rest)
		}

		rows, err := pool.Query(ctx, `SELECT Name, Rank FROM Statuses ORDER BY Rank, ID`)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		got := []string{}
		for rows.Next() {
			var name string
			var rank int
			if err := rows.Scan(&name, &rank); err != nil {
				t.Fatal(err)
			}
			if rank != len(got)+1 {
				t.Errorf("%s: %q has rank %d, want %d", c.name, name, rank, len(got)+1)
			}
			got = append(got, name)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: from rank 1 up %q, want %q", c.name, got, c.want)
		}

		// The column is finished: a status needs a rank from here on, and
		// the next one (as the store makes it) goes on top.
		if _, err := pool.Exec(ctx, `INSERT INTO Statuses (Name, Color, Shortcut) VALUES ('New', '112233', 'n')`); err == nil {
			t.Errorf("%s: a status without a rank was accepted after the migration", c.name)
		}
		var rank int
		err = pool.QueryRow(ctx, `
			INSERT INTO Statuses (Name, Color, Shortcut, Rank)
			VALUES ('New', '112233', 'n', (SELECT COALESCE(max(Rank), 0) + 1 FROM Statuses)) RETURNING Rank`).Scan(&rank)
		if err != nil || rank != len(c.want)+1 {
			t.Errorf("%s: a new status got rank %d, %v; want %d", c.name, rank, err, len(c.want)+1)
		}
		if again, err := Migrate(ctx, pool); err != nil || len(again) != 0 {
			t.Errorf("%s: a second Migrate applied %v, %v", c.name, again, err)
		}
	}
}
