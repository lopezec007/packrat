package store_test

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"sync"
	"testing"

	"packrat/internal/store"
	"packrat/internal/testdb"
)

// The statuses' rank: 1 is the worst, the highest the best. A fresh database
// has Missing (id 3) at rank 1, In Storage Location (id 1) at 2 and Packed
// (id 2) at 3.

// ranked lists the statuses' names from the worst to the best.
func ranked(t *testing.T, st *store.Store) []string {
	t.Helper()
	rows := mustList(t, st, "statuses")
	sort.SliceStable(rows, func(i, j int) bool { return num(rows[i]["rank"]) < num(rows[j]["rank"]) })
	return rowNames(rows)
}

// ranks maps each status's id to its rank.
func ranks(t *testing.T, st *store.Store) map[int64]int64 {
	t.Helper()
	out := map[int64]int64{}
	for _, r := range mustList(t, st, "statuses") {
		out[rowID(r)] = num(r["rank"])
	}
	return out
}

func newStatus(t *testing.T, st *store.Store, name, shortcut string) store.Row {
	t.Helper()
	return mustCreate(t, st, "statuses", mustJSON(t, map[string]any{"name": name, "color": "#112233", "shortcut": shortcut}))
}

func TestStatusSeededRanks(t *testing.T) {
	st := testdb.New(t)
	if got, want := ranked(t, st), []string{"Missing", "In Storage Location", "Packed"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("from the worst to the best: %v, want %v", got, want)
	}
	if got, want := ranks(t, st), (map[int64]int64{1: 2, 2: 3, 3: 1}); !reflect.DeepEqual(got, want) {
		t.Fatalf("ranks by id %v, want %v", got, want)
	}
	// They are still listed in the order they were made in.
	if got, want := rowNames(mustList(t, st, "statuses")), []string{"In Storage Location", "Packed", "Missing"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("listed %v, want %v", got, want)
	}
}

// A new status goes after the others: one more than the highest rank there is.
func TestStatusRankOfNewStatuses(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	statuses := store.RefTableByKey("statuses")
	// create makes a status and checks its rank.
	create := func(what, name, shortcut string, wantRank int64) int64 {
		t.Helper()
		row := newStatus(t, st, name, shortcut)
		if num(row["rank"]) != wantRank {
			t.Fatalf("%s: %v, want rank %d", what, row, wantRank)
		}
		return rowID(row)
	}

	bought := create("the first new status", "Bought", "b", 4)
	first := create("the second, whatever its name", "A First", "a", 5)
	// A refused create uses up no rank.
	if _, err := st.CreateRef(ctx, statuses, fields(t, `{"name": "Bought", "color": "#112233", "shortcut": "z"}`)); !isConflict(err) {
		t.Fatalf("duplicate name: %v", err)
	}
	clean := create("after a refused create", "Clean", "c", 6)

	// Deleting one from the middle leaves a gap, which is not filled...
	if err := st.DeleteRef(ctx, statuses, bought); err != nil {
		t.Fatal(err)
	}
	if got, want := ranks(t, st), (map[int64]int64{1: 2, 2: 3, 3: 1, first: 5, clean: 6}); !reflect.DeepEqual(got, want) {
		t.Fatalf("after deleting rank 4: %v, want %v", got, want)
	}
	dry := create("with a gap below", "Dry", "d", 7)
	// ...while the rank of the best, once that is deleted, is the next one's.
	if err := st.DeleteRef(ctx, statuses, dry); err != nil {
		t.Fatal(err)
	}
	emptied := create("after deleting the best", "Emptied", "e", 7)
	if got, want := ranked(t, st), []string{"Missing", "In Storage Location", "Packed", "A First", "Clean", "Emptied"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("from the worst to the best: %v, want %v", got, want)
	}

	// After a reordering it is one more than the number of statuses.
	if _, err := st.OrderStatuses(ctx, []int64{2, 1, 3, first, clean, emptied}); err != nil {
		t.Fatal(err)
	}
	create("after a reordering", "Folded", "f", 7)
	// The first status of an empty table is rank 1.
	for _, r := range mustList(t, st, "statuses") {
		if err := st.DeleteRef(ctx, statuses, rowID(r)); err != nil {
			t.Fatal(err)
		}
	}
	create("the only status", "Only", "o", 1)
}

// rank is read-only in the generic routes: a client may send a row back with
// it, and nothing comes of it.
func TestStatusRankIsNotWritable(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	seeded := map[int64]int64{1: 2, 2: 3, 3: 1}

	row := mustCreate(t, st, "statuses", `{"name": "Bought", "color": "#112233", "shortcut": "b", "rank": 1}`)
	if num(row["rank"]) != 4 {
		t.Fatalf("created with a rank of its own choosing: %v", row)
	}
	want := map[int64]int64{1: 2, 2: 3, 3: 1, 4: 4}
	for _, body := range []string{
		`{"rank": 1}`,
		`{"rank": 99}`,
		`{"rank": 0}`,
		`{"rank": null}`,
		`{"rank": "first"}`,
		`{"id": 4, "name": "Bought", "color": "#112233", "shortcut": "b", "rank": 2, "use_count": 0}`,
		`{"name": "Borrowed", "rank": 3}`,
	} {
		got := mustUpdate(t, st, "statuses", 4, body)
		if num(got["rank"]) != 4 {
			t.Errorf("update %s: rank %v, want 4 still", body, got["rank"])
		}
		if all := ranks(t, st); !reflect.DeepEqual(all, want) {
			t.Fatalf("update %s: ranks %v, want %v", body, all, want)
		}
	}
	if got := rowNamed(mustList(t, st, "statuses"), "Borrowed"); got == nil {
		t.Fatal("the name sent along with a rank should still have changed")
	}
	// A seeded status is no different.
	mustUpdate(t, st, "statuses", 3, `{"rank": 3, "name": "Lost"}`)
	if err := st.DeleteRef(ctx, store.RefTableByKey("statuses"), 4); err != nil {
		t.Fatal(err)
	}
	if all := ranks(t, st); !reflect.DeepEqual(all, seeded) {
		t.Fatalf("ranks %v, want %v", all, seeded)
	}
}

func TestOrderStatuses(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	newStatus(t, st, "Bought", "b") // 4
	newStatus(t, st, "Clean", "c")  // 5
	order := func(what string, ids []int64, want ...string) {
		t.Helper()
		rows, err := st.OrderStatuses(ctx, ids)
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		// What comes back is the listing: by id, each with its new rank.
		if listed := mustList(t, st, "statuses"); !reflect.DeepEqual(rows, listed) {
			t.Fatalf("%s: returned %v, listed %v", what, rows, listed)
		}
		for i, id := range ids {
			if got := num(rows[id-1]["rank"]); rowID(rows[id-1]) != id || got != int64(i+1) {
				t.Fatalf("%s: status %d has rank %d, want %d (%v)", what, id, got, i+1, rows)
			}
		}
		if got := ranked(t, st); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: from the worst to the best %v, want %v", what, got, want)
		}
	}

	order("as they are", []int64{3, 1, 2, 4, 5}, "Missing", "In Storage Location", "Packed", "Bought", "Clean")
	order("as they are, again", []int64{3, 1, 2, 4, 5}, "Missing", "In Storage Location", "Packed", "Bought", "Clean")
	// Every status takes a rank another has just now: only possible because
	// the ranks need not be distinct until the change is complete.
	order("reversed", []int64{5, 4, 2, 1, 3}, "Clean", "Bought", "Packed", "In Storage Location", "Missing")
	order("two swapped", []int64{5, 4, 1, 2, 3}, "Clean", "Bought", "In Storage Location", "Packed", "Missing")
	order("the ends swapped", []int64{3, 4, 1, 2, 5}, "Missing", "Bought", "In Storage Location", "Packed", "Clean")
	order("rotated", []int64{4, 1, 2, 5, 3}, "Bought", "In Storage Location", "Packed", "Clean", "Missing")
	order("by id", []int64{1, 2, 3, 4, 5}, "In Storage Location", "Packed", "Missing", "Bought", "Clean")
	shape := store.Row{"id": int16(5), "name": "Clean", "color": "#112233", "shortcut": "c", "rank": int32(5), "use_count": int32(0)}
	if got := mustList(t, st, "statuses")[4]; !reflect.DeepEqual(got, shape) {
		t.Fatalf("a row after ordering %v, want %v", got, shape)
	}

	// Every status exactly once, or nothing changes.
	before := ranks(t, st)
	const every = "ids must list every status once, from the worst to the best"
	refused := []struct {
		ids     []int64
		mention string
	}{
		{nil, every + " (0 of 5 given)"},
		{[]int64{}, every + " (0 of 5 given)"},
		{[]int64{5, 4, 3, 2}, every + " (4 of 5 given)"},
		{[]int64{3}, every + " (1 of 5 given)"},
		{[]int64{5, 4, 3, 2, 1, 6}, "ids: there is no status 6"},
		{[]int64{5, 4, 3, 2, 99}, "ids: there is no status 99"},
		{[]int64{0, 5, 4, 3, 2, 1}, "ids: there is no status 0"},
		{[]int64{5, 4, 3, 2, -1}, "ids: there is no status -1"},
		{[]int64{5, 4, 3, 2, pastSmallint}, "ids: there is no status 40000"},
		{[]int64{pastInt, 4, 3, 2, 1}, "ids: there is no status 3000000000"},
		{[]int64{5, 4, 3, 2, 1, 1}, "ids: status 1 is listed twice"},
		{[]int64{5, 5, 3, 2, 1}, "ids: status 5 is listed twice"},
		{[]int64{5, 4, 3, 2, 2}, "ids: status 2 is listed twice"},
	}
	for _, c := range refused {
		rows, err := st.OrderStatuses(ctx, c.ids)
		wantValidation(t, fmt.Sprintf("order %v", c.ids), err, c.mention)
		if rows != nil {
			t.Errorf("order %v: rows came back with the error", c.ids)
		}
		if got := ranks(t, st); !reflect.DeepEqual(got, before) {
			t.Fatalf("order %v changed the ranks: %v, want %v", c.ids, got, before)
		}
	}
}

// A deleted status leaves a gap in the ranks; ordering the rest closes it.
// Deleting a status is what it was before there were ranks.
func TestOrderStatusesAfterADelete(t *testing.T) {
	fx := newFx(t)
	ctx := context.Background()
	st := fx.st
	statuses := store.RefTableByKey("statuses")
	newStatus(t, st, "Bought", "b") // 4
	newStatus(t, st, "Clean", "c")  // 5
	trip := fx.list("Trip", "socks @packed", "comb")

	// A status in use stays, rank and all.
	wantConflict(t, "delete a status in use", st.DeleteRef(ctx, statuses, fx.packed), "this status is in use on a list")
	if got, want := ranks(t, st), (map[int64]int64{1: 2, 2: 3, 3: 1, 4: 4, 5: 5}); !reflect.DeepEqual(got, want) {
		t.Fatalf("after the refused delete %v, want %v", got, want)
	}
	// The worst and one from the middle go: the others keep their ranks.
	for _, id := range []int64{fx.missing, 4} {
		if err := st.DeleteRef(ctx, statuses, id); err != nil {
			t.Fatalf("delete status %d: %v", id, err)
		}
	}
	if got, want := ranks(t, st), (map[int64]int64{1: 2, 2: 3, 5: 5}); !reflect.DeepEqual(got, want) {
		t.Fatalf("after the deletes %v, want %v", got, want)
	}
	if got, want := ranked(t, st), []string{"In Storage Location", "Packed", "Clean"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("from the worst to the best %v, want %v", got, want)
	}

	// The deleted ones are no longer statuses to order...
	_, err := st.OrderStatuses(ctx, []int64{3, 1, 2, 5})
	wantValidation(t, "order with a deleted status", err, "ids: there is no status 3")
	_, err = st.OrderStatuses(ctx, []int64{1, 2})
	wantValidation(t, "order without the newest", err, "(2 of 3 given)")
	// ...and the rest are ranked 1, 2, 3.
	rows, err := st.OrderStatuses(ctx, []int64{5, 2, 1})
	if err != nil || len(rows) != 3 {
		t.Fatalf("order the three that are left: %v, %v", rows, err)
	}
	if got, want := ranks(t, st), (map[int64]int64{1: 3, 2: 2, 5: 1}); !reflect.DeepEqual(got, want) {
		t.Fatalf("ranks %v, want %v", got, want)
	}
	// The counts come back with the rows; the list is as it was.
	if num(rows[0]["use_count"]) != 1 || num(rows[1]["use_count"]) != 1 || num(rows[2]["use_count"]) != 0 {
		t.Fatalf("use counts in the returned rows: %v", rows)
	}
	fx.want(trip, "socks @packed", "comb")

	// The status a new entry starts with is the first by id, not by rank:
	// In Storage Location, which is now ranked the best.
	if c := fx.add(trip, "towel"); c.StatusID != fx.stored {
		t.Fatalf("a new entry has status %d, want %d", c.StatusID, fx.stored)
	}
}

func TestOrderStatusesWithNoStatuses(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	for _, id := range []int64{1, 2, 3} {
		if err := st.DeleteRef(ctx, store.RefTableByKey("statuses"), id); err != nil {
			t.Fatal(err)
		}
	}
	for _, ids := range [][]int64{nil, {}} {
		rows, err := st.OrderStatuses(ctx, ids)
		if err != nil || rows == nil || len(rows) != 0 {
			t.Fatalf("order %v of no statuses: %v, %v; want an empty, non-nil list", ids, rows, err)
		}
	}
	_, err := st.OrderStatuses(ctx, []int64{1})
	wantValidation(t, "order a status that is gone", err, "ids: there is no status 1")
	// One status is an order too.
	only := rowID(newStatus(t, st, "Only", "o"))
	rows, err := st.OrderStatuses(ctx, []int64{only})
	if err != nil || len(rows) != 1 || num(rows[0]["rank"]) != 1 {
		t.Fatalf("order the only status: %v, %v", rows, err)
	}
}

// Statuses created at the same time get different ranks, each the next one
// up, and a reordering going on meanwhile neither loses nor repeats any.
func TestConcurrentStatusCreatesGetDistinctRanks(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	const n = 8

	errs := make([]error, n)
	var orderErr error
	var wg sync.WaitGroup
	for i := range n {
		body := fields(t, mustJSON(t, map[string]any{"name": fmt.Sprint("Status ", i), "color": "#112233", "shortcut": string(rune('0' + i))}))
		wg.Go(func() {
			_, errs[i] = st.CreateRef(ctx, store.RefTableByKey("statuses"), body)
		})
	}
	wg.Go(func() {
		// Reverses the seeded three if it gets in first; refused (they are no
		// longer all the statuses) if any create does.
		_, orderErr = st.OrderStatuses(ctx, []int64{2, 1, 3})
	})
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("create %d: %v", i, err)
		}
	}
	if orderErr != nil && !isValidation(orderErr) {
		t.Errorf("the reordering: want success or a validation error, got %v", orderErr)
	}

	var got []int64
	for _, rank := range ranks(t, st) {
		got = append(got, rank)
	}
	sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
	want := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ranks after %d creates at once: %v, want %v", n, got, want)
	}
	// The eight new ones are the best eight, whatever became of the three.
	for id, rank := range ranks(t, st) {
		if (id <= 3) != (rank <= 3) {
			t.Errorf("status %d has rank %d", id, rank)
		}
	}
}
