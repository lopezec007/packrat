package db_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"packrat/internal/db"
	"packrat/internal/testdb"
)

// Applying every migration to an empty schema works (testdb does just that),
// records each one, and leaves nothing to apply a second time.
func TestMigrateFromScratch(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()

	rows, err := pool.Query(ctx, `SELECT Version, Name FROM Schema_Migrations ORDER BY Version`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var version int
		var name string
		if err := rows.Scan(&version, &name); err != nil {
			t.Fatal(err)
		}
		if version != len(names)+1 {
			t.Fatalf("migration %q is recorded as version %d, want %d", name, version, len(names)+1)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := []string{"0001_schema.sql", "0002_seed.sql", "0003_compartments.sql", "0004_status_rank.sql"}
	if len(names) < len(want) || !reflect.DeepEqual(names[:len(want)], want) {
		t.Fatalf("applied migrations %v, want %v first", names, want)
	}

	applied, err := db.Migrate(ctx, pool)
	if err != nil || len(applied) != 0 {
		t.Fatalf("a second Migrate applied %v, %v; want nothing", applied, err)
	}

	// The tables are there: the statuses seeded, the rest empty.
	for table, want := range map[string]int{
		"Lists": 0, "Item_Categories": 0, "Storage_Locations": 0, "Items": 0, "Statuses": 3, "Compartments": 0, "List_Contents": 0,
	} {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&n); err != nil || n != want {
			t.Errorf("%s: %d rows, %v; want %d", table, n, err, want)
		}
	}
	// The seeded statuses are ranked from the worst to the best (0004).
	if got, want := statusesByRank(t, pool), []string{"1 Missing", "2 In Storage Location", "3 Packed"}; !reflect.DeepEqual(got, want) {
		t.Errorf("seeded statuses by rank %v, want %v", got, want)
	}
	var nullable string
	err = pool.QueryRow(ctx, `
		SELECT is_nullable FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'list_contents' AND column_name = 'compartment_id'`).Scan(&nullable)
	if err != nil || nullable != "YES" {
		t.Errorf("List_Contents.Compartment_ID should be a nullable column: %q, %v", nullable, err)
	}
}

// statusesByRank lists the statuses as "rank name", from the worst to the best.
func statusesByRank(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT Rank || ' ' || Name FROM Statuses ORDER BY Rank, ID`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// wantPgError checks that err is the PostgreSQL error with the SQLSTATE,
// raised by the named constraint ("" for a NOT NULL, which has no name).
func wantPgError(t *testing.T, what string, err error, code, constraint string) {
	t.Helper()
	var pe *pgconn.PgError
	if !errors.As(err, &pe) || pe.Code != code || pe.ConstraintName != constraint {
		t.Errorf("%s: want SQLSTATE %s from %s, got %v", what, code, constraint, err)
	}
}

const (
	notNullViolation    = "23502"
	foreignKeyViolation = "23503"
	uniqueViolation     = "23505"
	checkViolation      = "23514"
)

// The store checks compartments before it writes (internal/store/contents.go);
// these are the constraints of 0003_compartments.sql that hold whoever writes.
func TestCompartmentConstraints(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()
	exec := func(sql string, args ...any) error {
		_, err := pool.Exec(ctx, sql, args...)
		return err
	}
	must := func(sql string, args ...any) {
		t.Helper()
		if err := exec(sql, args...); err != nil {
			t.Fatalf("%s %v: %v", sql, args, err)
		}
	}
	count := func(sql string, args ...any) (n int) {
		t.Helper()
		if err := pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// Items: 1 bag, 2 kit, 3 crate (containers), 4 socks. Compartments: 1 the
	// bag's lid, 2 the bag's pocket, 3 the kit's mesh, 4 the crate's tray.
	must(`INSERT INTO Item_Categories (Name) VALUES ('Gear')`)
	must(`INSERT INTO Storage_Locations (Name) VALUES ('Garage')`)
	must(`INSERT INTO Items (Item_Category_ID, Name, Storage_Location_ID, Is_Container) VALUES
		(1, 'bag', 1, true), (1, 'kit', 1, true), (1, 'crate', 1, true), (1, 'socks', 1, false)`)
	must(`INSERT INTO Compartments (Item_ID, Name) VALUES (1, 'lid'), (1, 'pocket'), (2, 'mesh'), (3, 'tray')`)
	must(`INSERT INTO Lists (Name, Is_Template) VALUES ('Trip', false)`)
	const insert = `INSERT INTO List_Contents (List_ID, Container_ID, Compartment_ID, Item_ID, Quantity, Status_ID) VALUES (1, $1, $2, $3, 1, 1)`
	must(insert, nil, nil, 1) // the bag, at the top level
	must(insert, 1, nil, 2)   // the kit in the bag, in no compartment

	// A compartment's name is unique within its item, and it needs an item.
	wantPgError(t, "a second lid for the bag", exec(`INSERT INTO Compartments (Item_ID, Name) VALUES (1, 'lid')`), uniqueViolation, "un_compartments")
	wantPgError(t, "rename the pocket to lid", exec(`UPDATE Compartments SET Name = 'lid' WHERE ID = 2`), uniqueViolation, "un_compartments")
	must(`INSERT INTO Compartments (Item_ID, Name) VALUES (2, 'lid')`) // 6 (5 went to the refused insert): the kit can have a lid too
	wantPgError(t, "a compartment of no item", exec(`INSERT INTO Compartments (Item_ID, Name) VALUES (99, 'lid')`), foreignKeyViolation, "fk_compartments_item_id")

	// An entry's compartment must be one of its container's...
	const fk = "fk_list_contents_compartment_id"
	wantPgError(t, "the kit's mesh for something in the bag", exec(insert, 1, 3, 4), foreignKeyViolation, fk)
	wantPgError(t, "a compartment that is not there", exec(insert, 1, 99, 4), foreignKeyViolation, fk)
	// ...which at the top level, in no container, it cannot be.
	const ck = "ck_list_contents_compartment"
	wantPgError(t, "a compartment at the top level", exec(insert, nil, 1, 4), checkViolation, ck)
	wantPgError(t, "a compartment that is not there, at the top level", exec(insert, nil, 99, 4), checkViolation, ck)
	if n := count(`SELECT count(*) FROM List_Contents`); n != 2 {
		t.Fatalf("%d rows after the refused inserts, want 2", n)
	}

	must(insert, 1, 1, 4)                                            // socks in the bag's lid
	socks := count(`SELECT ID FROM List_Contents WHERE Item_ID = 4`) // the row's id
	// The same holds for every later change to the row: container and
	// compartment have to change together.
	wantPgError(t, "into the kit, still in the bag's lid", exec(`UPDATE List_Contents SET Container_ID = 2 WHERE ID = $1`, socks), foreignKeyViolation, fk)
	wantPgError(t, "to the top level, still in the lid", exec(`UPDATE List_Contents SET Container_ID = NULL WHERE ID = $1`, socks), checkViolation, ck)
	wantPgError(t, "into the kit's mesh, still in the bag", exec(`UPDATE List_Contents SET Compartment_ID = 3 WHERE ID = $1`, socks), foreignKeyViolation, fk)
	must(`UPDATE List_Contents SET Compartment_ID = 2 WHERE ID = $1`, socks)
	must(`UPDATE List_Contents SET Container_ID = 2, Compartment_ID = 3 WHERE ID = $1`, socks)
	must(`UPDATE List_Contents SET Container_ID = 1, Compartment_ID = 1 WHERE ID = $1`, socks)
	// Still one row per item and container, whatever the compartment.
	wantPgError(t, "socks in the bag's pocket as well", exec(insert, 1, 2, 4), uniqueViolation, "un_list_contents")
	wantPgError(t, "socks in the bag as well, in no compartment", exec(insert, 1, nil, 4), uniqueViolation, "un_list_contents")

	// A compartment in use cannot be deleted or given to another item behind
	// the entry's back (the store empties it first).
	must(`UPDATE Compartments SET Name = 'top' WHERE ID = 1`) // renaming it is no one's business but its own
	wantPgError(t, "delete the compartment in use", exec(`DELETE FROM Compartments WHERE ID = 1`), foreignKeyViolation, fk)
	wantPgError(t, "give the compartment in use to the kit", exec(`UPDATE Compartments SET Item_ID = 2 WHERE ID = 1`), foreignKeyViolation, fk)
	must(`DELETE FROM Compartments WHERE ID = 2`) // nothing is in the pocket

	// Deleting an item deletes its compartments: the crate is on no list.
	must(`DELETE FROM Items WHERE ID = 3`)
	if n := count(`SELECT count(*) FROM Compartments WHERE Item_ID = 3`); n != 0 {
		t.Fatalf("%d compartments of the deleted crate are left", n)
	}
	// An item on a list stays, and its compartments with it.
	if err := exec(`DELETE FROM Items WHERE ID = 1`); err == nil {
		t.Fatal("deleted the bag while it is on a list")
	}
	if n := count(`SELECT count(*) FROM Compartments`); n != 3 {
		t.Fatalf("%d compartments left, want 3 (the bag's top, the kit's mesh and lid)", n)
	}
}

// The constraints of 0004_status_rank.sql: a rank is a positive number, and
// no two statuses have the same one once a transaction commits (they may
// while it is under way, which is what lets the store reorder them).
func TestStatusRankConstraints(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()
	exec := func(sql string, args ...any) error {
		_, err := pool.Exec(ctx, sql, args...)
		return err
	}
	seeded := []string{"1 Missing", "2 In Storage Location", "3 Packed"}
	unchanged := func(what string) {
		t.Helper()
		if got := statusesByRank(t, pool); !reflect.DeepEqual(got, seeded) {
			t.Fatalf("%s changed the statuses: %v", what, got)
		}
	}
	const insert = `INSERT INTO Statuses (Name, Color, Shortcut, Rank) VALUES ($1, '112233', $2, $3)`

	// Positive, and there: refused at once.
	const ck = "ck_statuses_rank"
	wantPgError(t, "rank 0", exec(`UPDATE Statuses SET Rank = 0 WHERE ID = 1`), checkViolation, ck)
	wantPgError(t, "a negative rank", exec(`UPDATE Statuses SET Rank = -2 WHERE ID = 1`), checkViolation, ck)
	wantPgError(t, "a new status with rank 0", exec(insert, "Bought", "b", 0), checkViolation, ck)
	wantPgError(t, "no rank", exec(`UPDATE Statuses SET Rank = NULL WHERE ID = 1`), notNullViolation, "")
	wantPgError(t, "a new status with no rank", exec(`INSERT INTO Statuses (Name, Color, Shortcut) VALUES ('Bought', '112233', 'b')`), notNullViolation, "")
	unchanged("a refused rank")

	// A rank another status has: a statement on its own is its own
	// transaction, so it is refused there and then.
	const un = "un_statuses_rank"
	wantPgError(t, "a taken rank", exec(`UPDATE Statuses SET Rank = 1 WHERE ID = 1`), uniqueViolation, un)
	wantPgError(t, "a new status with a taken rank", exec(insert, "Bought", "b", 3), uniqueViolation, un)
	unchanged("a taken rank")

	// Inside a transaction the statement goes through, and the commit is refused.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) // should the test stop half way: a no-op after the commit
	if _, err := tx.Exec(ctx, `UPDATE Statuses SET Rank = 1 WHERE ID = 1`); err != nil {
		t.Fatalf("a taken rank inside a transaction should wait for the commit: %v", err)
	}
	var twice int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM Statuses WHERE Rank = 1`).Scan(&twice); err != nil || twice != 2 {
		t.Fatalf("%d statuses at rank 1 inside the transaction, %v; want 2", twice, err)
	}
	wantPgError(t, "commit with a rank taken twice", tx.Commit(ctx), uniqueViolation, un)
	unchanged("the refused commit")

	// So two statuses can swap ranks, one statement after the other...
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	for _, sql := range []string{`UPDATE Statuses SET Rank = 3 WHERE ID = 3`, `UPDATE Statuses SET Rank = 1 WHERE ID = 2`} {
		if _, err := tx.Exec(ctx, sql); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit a swap: %v", err)
	}
	if got, want := statusesByRank(t, pool), []string{"1 Packed", "2 In Storage Location", "3 Missing"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after the swap %v, want %v", got, want)
	}
	// ...or all of them at once.
	if err := exec(`UPDATE Statuses SET Rank = 4 - Rank`); err != nil {
		t.Fatalf("reverse the ranks in one statement: %v", err)
	}
	unchanged("reversing back")

	// The ranks are an order: gaps are fine.
	if err := exec(insert, "Bought", "b", 10); err != nil {
		t.Fatalf("a rank with a gap below it: %v", err)
	}
	if err := exec(`DELETE FROM Statuses WHERE ID = 1`); err != nil {
		t.Fatalf("delete a status: %v", err)
	}
	if got, want := statusesByRank(t, pool), []string{"1 Missing", "3 Packed", "10 Bought"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("with gaps %v, want %v", got, want)
	}
}
