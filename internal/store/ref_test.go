package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"packrat/internal/store"
	"packrat/internal/testdb"
)

func fields(t *testing.T, js string) store.Fields {
	t.Helper()
	var f store.Fields
	if err := json.Unmarshal([]byte(js), &f); err != nil {
		t.Fatalf("bad test JSON %s: %v", js, err)
	}
	return f
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func mustCreate(t *testing.T, st *store.Store, table, js string) store.Row {
	t.Helper()
	row, err := st.CreateRef(context.Background(), store.RefTableByKey(table), fields(t, js))
	if err != nil {
		t.Fatalf("create %s %s: %v", table, js, err)
	}
	return row
}

func mustUpdate(t *testing.T, st *store.Store, table string, id int64, js string) store.Row {
	t.Helper()
	row, err := st.UpdateRef(context.Background(), store.RefTableByKey(table), id, fields(t, js))
	if err != nil {
		t.Fatalf("update %s %d %s: %v", table, id, js, err)
	}
	return row
}

func mustList(t *testing.T, st *store.Store, table string) []store.Row {
	t.Helper()
	rows, err := st.ListRef(context.Background(), store.RefTableByKey(table))
	if err != nil {
		t.Fatalf("list %s: %v", table, err)
	}
	return rows
}

// num reads an integer column whatever its width (smallint, int).
func num(v any) int64 {
	switch v := v.(type) {
	case int16:
		return int64(v)
	case int32:
		return int64(v)
	case int64:
		return v
	}
	panic("not an integer column")
}

func rowID(row store.Row) int64 { return num(row["id"]) }

// rowNamed finds a row by name, or nil.
func rowNamed(rows []store.Row, name string) store.Row {
	for _, r := range rows {
		if r["name"] == name {
			return r
		}
	}
	return nil
}

func rowNames(rows []store.Row) []string {
	out := []string{}
	for _, r := range rows {
		out = append(out, r["name"].(string))
	}
	return out
}

func rowKeys(row store.Row) []string {
	keys := make([]string, 0, len(row))
	for k := range row {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func isValidation(err error) bool { var e *store.ValidationError; return errors.As(err, &e) }
func isConflict(err error) bool   { var e *store.ConflictError; return errors.As(err, &e) }

// wantValidation checks for a *ValidationError whose message mentions `mention`.
func wantValidation(t *testing.T, what string, err error, mention string) {
	t.Helper()
	if !isValidation(err) || !strings.Contains(err.Error(), mention) {
		t.Errorf("%s: want a validation error mentioning %q, got %v", what, mention, err)
	}
}

// wantConflict checks for a *ConflictError whose message mentions `mention`.
func wantConflict(t *testing.T, what string, err error, mention string) {
	t.Helper()
	if !isConflict(err) || !strings.Contains(err.Error(), mention) {
		t.Errorf("%s: want a conflict mentioning %q, got %v", what, mention, err)
	}
}

func wantNotFound(t *testing.T, what string, err error) {
	t.Helper()
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("%s: want not found, got %v", what, err)
	}
}

// IDs no column can hold: past smallint, past int.
const (
	pastSmallint = 40000
	pastInt      = 3000000000
)

// ---------------------------------------------------------------- seeds

func TestSeedsAndListAll(t *testing.T) {
	st := testdb.New(t)
	all, err := st.ListAllRef(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != len(store.RefTables) || len(all) != 4 {
		t.Fatalf("got %d tables, want %d", len(all), len(store.RefTables))
	}
	for _, table := range []string{"item_categories", "storage_locations", "items"} {
		if rows, ok := all[table]; !ok || rows == nil || len(rows) != 0 {
			t.Errorf("%s should start as an empty, non-nil list: %v", table, rows)
		}
	}
	want := []store.Row{
		{"id": int16(1), "name": "In Storage Location", "color": "#8a94a3", "shortcut": "s", "use_count": int32(0)},
		{"id": int16(2), "name": "Packed", "color": "#2e8540", "shortcut": "p", "use_count": int32(0)},
		{"id": int16(3), "name": "Missing", "color": "#d62828", "shortcut": "m", "use_count": int32(0)},
	}
	if !reflect.DeepEqual(all["statuses"], want) {
		t.Fatalf("seeded statuses:\n got  %v\n want %v", all["statuses"], want)
	}
	if store.RefTableByKey("lists") != nil || store.RefTableByKey("Items") != nil {
		t.Fatal("RefTableByKey should only know the four reference tables, by their lower-case keys")
	}
}

// ---------------------------------------------------------------- create, list, update, delete

func TestRefNameTablesCRUD(t *testing.T) {
	for _, table := range []string{"item_categories", "storage_locations"} {
		st := testdb.New(t)
		ctx := context.Background()
		tbl := store.RefTableByKey(table)

		row := mustCreate(t, st, table, `{"name": "  Clothing "}`)
		if row["name"] != "Clothing" || rowID(row) != 1 || num(row["item_count"]) != 0 {
			t.Fatalf("%s: created %v", table, row)
		}
		if got := rowKeys(row); !reflect.DeepEqual(got, []string{"id", "item_count", "name"}) {
			t.Fatalf("%s: keys %v", table, got)
		}
		mustCreate(t, st, table, `{"name": "camp"}`)
		mustCreate(t, st, table, `{"name": "Electronics"}`)
		// Sorted by name without regard to case.
		if got := rowNames(mustList(t, st, table)); !reflect.DeepEqual(got, []string{"camp", "Clothing", "Electronics"}) {
			t.Fatalf("%s: listed %v", table, got)
		}

		id := rowID(row)
		if got := mustUpdate(t, st, table, id, `{"name": " Clothes "}`); got["name"] != "Clothes" || rowID(got) != id {
			t.Fatalf("%s: renamed %v", table, got)
		}
		// Nothing to change is not an error, and a client may send back the
		// whole row it was given: id and the computed columns are ignored.
		if got := mustUpdate(t, st, table, id, `{}`); got["name"] != "Clothes" {
			t.Fatalf("%s: empty update %v", table, got)
		}
		if got := mustUpdate(t, st, table, id, `{"id": 77, "name": "Clothes", "item_count": 12}`); rowID(got) != id || num(got["item_count"]) != 0 {
			t.Fatalf("%s: echoed row %v", table, got)
		}
		// Renaming a row to the name it has is fine.
		mustUpdate(t, st, table, id, `{"name": "Clothes"}`)

		if err := st.DeleteRef(ctx, tbl, id); err != nil {
			t.Fatalf("%s: delete: %v", table, err)
		}
		if got := rowNames(mustList(t, st, table)); !reflect.DeepEqual(got, []string{"camp", "Electronics"}) {
			t.Fatalf("%s: after delete %v", table, got)
		}
		wantNotFound(t, table+": second delete", st.DeleteRef(ctx, tbl, id))
		_, err := st.UpdateRef(ctx, tbl, id, fields(t, `{"name": "Again"}`))
		wantNotFound(t, table+": update of a deleted row", err)
		_, err = st.UpdateRef(ctx, tbl, id, fields(t, `{}`))
		wantNotFound(t, table+": empty update of a deleted row", err)
		// An ID no smallint column can hold is just as absent.
		for _, id := range []int64{pastSmallint, pastInt, -pastSmallint} {
			_, err = st.UpdateRef(ctx, tbl, id, fields(t, `{"name": "Again"}`))
			wantNotFound(t, table+": update out of range", err)
			wantNotFound(t, table+": delete out of range", st.DeleteRef(ctx, tbl, id))
		}
	}
}

func TestStatusCRUD(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	statuses := store.RefTableByKey("statuses")

	row := mustCreate(t, st, "statuses", `{"name": " Bought ", "color": "#2E8540", "shortcut": "B"}`)
	want := store.Row{"id": int16(4), "name": "Bought", "color": "#2e8540", "shortcut": "b", "use_count": int32(0)}
	if !reflect.DeepEqual(row, want) {
		t.Fatalf("created %v, want %v", row, want)
	}
	// Statuses keep the order they were made in, not the order of their names.
	if got := rowNames(mustList(t, st, "statuses")); !reflect.DeepEqual(got, []string{"In Storage Location", "Packed", "Missing", "Bought"}) {
		t.Fatalf("listed %v", got)
	}
	got := mustUpdate(t, st, "statuses", 4, `{"color": "ABCDEF"}`)
	if got["color"] != "#abcdef" || got["name"] != "Bought" || got["shortcut"] != "b" {
		t.Fatalf("recoloured %v", got)
	}
	got = mustUpdate(t, st, "statuses", 4, `{"name": "Borrowed", "shortcut": "W"}`)
	if got["color"] != "#abcdef" || got["name"] != "Borrowed" || got["shortcut"] != "w" {
		t.Fatalf("renamed %v", got)
	}
	// A seeded status is an ordinary row.
	if got := mustUpdate(t, st, "statuses", 1, `{"name": "At Home", "shortcut": "h"}`); got["name"] != "At Home" || got["shortcut"] != "h" {
		t.Fatalf("seeded status %v", got)
	}
	if err := st.DeleteRef(ctx, statuses, 3); err != nil {
		t.Fatalf("delete a seeded status: %v", err)
	}
	if err := st.DeleteRef(ctx, statuses, 4); err != nil {
		t.Fatal(err)
	}
	wantNotFound(t, "second delete", st.DeleteRef(ctx, statuses, 4))
	if got := rowNames(mustList(t, st, "statuses")); !reflect.DeepEqual(got, []string{"At Home", "Packed"}) {
		t.Fatalf("after deletes %v", got)
	}
}

func TestItemCRUD(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	items := store.RefTableByKey("items")
	clothing := rowID(mustCreate(t, st, "item_categories", `{"name": "Clothing"}`))
	camp := rowID(mustCreate(t, st, "item_categories", `{"name": "Camp"}`))
	closet := rowID(mustCreate(t, st, "storage_locations", `{"name": "Closet"}`))
	garage := rowID(mustCreate(t, st, "storage_locations", `{"name": "Garage"}`))

	socks := mustCreate(t, st, "items", mustJSON(t, map[string]any{"name": " Socks ", "item_category_id": clothing, "storage_location_id": closet}))
	want := store.Row{"id": int32(1), "item_category_id": int16(clothing), "name": "Socks", "storage_location_id": int16(closet),
		"is_container": false, "list_count": int32(0)}
	if !reflect.DeepEqual(socks, want) {
		t.Fatalf("created %v, want %v (is_container defaults to false)", socks, want)
	}
	bin := mustCreate(t, st, "items", mustJSON(t, map[string]any{"name": "bin, Large", "item_category_id": camp, "storage_location_id": garage, "is_container": true}))
	if bin["is_container"] != true {
		t.Fatalf("container %v", bin)
	}
	mustCreate(t, st, "items", mustJSON(t, map[string]any{"name": "Lantern, Gas", "item_category_id": camp, "storage_location_id": garage}))
	if got := rowNames(mustList(t, st, "items")); !reflect.DeepEqual(got, []string{"bin, Large", "Lantern, Gas", "Socks"}) {
		t.Fatalf("listed %v", got)
	}

	// Each column on its own; the others stay.
	got := mustUpdate(t, st, "items", rowID(socks), mustJSON(t, map[string]any{"item_category_id": camp}))
	if num(got["item_category_id"]) != camp || num(got["storage_location_id"]) != closet || got["name"] != "Socks" || got["is_container"] != false {
		t.Fatalf("moved category %v", got)
	}
	got = mustUpdate(t, st, "items", rowID(socks), mustJSON(t, map[string]any{"storage_location_id": garage, "name": "Socks, Wool"}))
	if num(got["item_category_id"]) != camp || num(got["storage_location_id"]) != garage || got["name"] != "Socks, Wool" {
		t.Fatalf("moved location %v", got)
	}
	if got = mustUpdate(t, st, "items", rowID(socks), `{"is_container": true}`); got["is_container"] != true || got["name"] != "Socks, Wool" {
		t.Fatalf("made a container %v", got)
	}
	if got = mustUpdate(t, st, "items", rowID(socks), `{"is_container": false, "list_count": 3}`); got["is_container"] != false || num(got["list_count"]) != 0 {
		t.Fatalf("unmade a container %v", got)
	}

	if err := st.DeleteRef(ctx, items, rowID(socks)); err != nil {
		t.Fatal(err)
	}
	wantNotFound(t, "second delete", st.DeleteRef(ctx, items, rowID(socks)))
	_, err := st.UpdateRef(ctx, items, rowID(socks), fields(t, `{"name": "Socks"}`))
	wantNotFound(t, "update of a deleted item", err)
	_, err = st.UpdateRef(ctx, items, rowID(socks), fields(t, `{"is_container": true}`))
	wantNotFound(t, "container flag of a deleted item", err)
	// Items.ID is an int: 40000 could be an item, three thousand million could not.
	for _, id := range []int64{pastSmallint, pastInt, -pastInt} {
		_, err = st.UpdateRef(ctx, items, id, fields(t, `{"is_container": true}`))
		wantNotFound(t, "container flag out of range", err)
		_, err = st.UpdateRef(ctx, items, id, fields(t, `{"name": "Socks"}`))
		wantNotFound(t, "update out of range", err)
		wantNotFound(t, "delete out of range", st.DeleteRef(ctx, items, id))
	}
}

// ---------------------------------------------------------------- validation

func TestRefValidation(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	cat := rowID(mustCreate(t, st, "item_categories", `{"name": "Clothing"}`))
	loc := rowID(mustCreate(t, st, "storage_locations", `{"name": "Closet"}`))
	item := rowID(mustCreate(t, st, "items", `{"name": "Socks", "item_category_id": 1, "storage_location_id": 1}`))
	long := strings.Repeat("n", 128)

	cases := []struct {
		table, body, mention string
	}{
		// Required fields.
		{"item_categories", `{}`, "name is required"},
		{"storage_locations", `{}`, "name is required"},
		{"statuses", `{"color": "#112233", "shortcut": "x"}`, "name is required"},
		{"statuses", `{"name": "Worn", "shortcut": "x"}`, "color is required"},
		{"statuses", `{"name": "Worn", "color": "#112233"}`, "shortcut is required"},
		{"items", `{"item_category_id": 1, "storage_location_id": 1}`, "name is required"},
		{"items", `{"name": "Comb", "storage_location_id": 1}`, "item_category_id is required"},
		{"items", `{"name": "Comb", "item_category_id": 1}`, "storage_location_id is required"},
		// Blank and null count as missing.
		{"item_categories", `{"name": ""}`, "name is required"},
		{"item_categories", `{"name": "   "}`, "name is required"},
		{"item_categories", `{"name": null}`, "name is required"},
		{"statuses", `{"name": "Worn", "color": "", "shortcut": "x"}`, "color is required"},
		{"statuses", `{"name": "Worn", "color": "#112233", "shortcut": null}`, "shortcut is required"},
		{"items", `{"name": "Comb", "item_category_id": null, "storage_location_id": 1}`, "item_category_id is required"},
		{"items", `{"name": "Comb", "item_category_id": 1, "storage_location_id": 1, "is_container": null}`, "is_container is required"},
		// The wrong JSON type.
		{"item_categories", `{"name": 5}`, "name must be text"},
		{"storage_locations", `{"name": ["Closet"]}`, "name must be text"},
		{"statuses", `{"name": "Worn", "color": 112233, "shortcut": "x"}`, "color must be text"},
		{"statuses", `{"name": "Worn", "color": "#112233", "shortcut": 7}`, "shortcut must be text"},
		{"items", `{"name": "Comb", "item_category_id": "1", "storage_location_id": 1}`, "item_category_id must be a whole number"},
		{"items", `{"name": "Comb", "item_category_id": 1.5, "storage_location_id": 1}`, "item_category_id must be a whole number"},
		{"items", `{"name": "Comb", "item_category_id": 1, "storage_location_id": true}`, "storage_location_id must be a whole number"},
		{"items", `{"name": "Comb", "item_category_id": 1, "storage_location_id": 1, "is_container": "yes"}`, "is_container must be true or false"},
		{"items", `{"name": "Comb", "item_category_id": 1, "storage_location_id": 1, "is_container": 1}`, "is_container must be true or false"},
		// Unknown fields.
		{"item_categories", `{"name": "Camp", "colour": "red"}`, `unknown field "colour" for item_categories`},
		{"storage_locations", `{"name": "Attic", "Name": "Attic"}`, `unknown field "Name"`},
		{"statuses", `{"name": "Worn", "color": "#112233", "shortcut": "x", "key": "x"}`, `unknown field "key" for statuses`},
		{"items", `{"name": "Comb", "item_category_id": 1, "storage_location_id": 1, "category": "Clothing"}`, `unknown field "category" for items`},
		// Too long.
		{"item_categories", `{"name": "` + long + `"}`, "name is too long (at most 127 characters)"},
		{"storage_locations", `{"name": "` + long + `"}`, "name is too long"},
		{"statuses", `{"name": "` + long + `", "color": "#112233", "shortcut": "x"}`, "name is too long"},
		{"items", `{"name": "` + long + `", "item_category_id": 1, "storage_location_id": 1}`, "name is too long"},
		// A category or location that is not there, or could not be.
		{"items", `{"name": "Comb", "item_category_id": 99, "storage_location_id": 1}`, "does not exist"},
		{"items", `{"name": "Comb", "item_category_id": 1, "storage_location_id": 99}`, "does not exist"},
		{"items", `{"name": "Comb", "item_category_id": 40000, "storage_location_id": 1}`, "item_category_id is out of range"},
		{"items", `{"name": "Comb", "item_category_id": 1, "storage_location_id": -40000}`, "storage_location_id is out of range"},
		// Text PostgreSQL cannot store.
		{"item_categories", `{"name": "Cam\u0000p"}`, "item category"},
	}
	for _, c := range cases {
		_, err := st.CreateRef(ctx, store.RefTableByKey(c.table), fields(t, c.body))
		wantValidation(t, "create "+c.table+" "+c.body, err, c.mention)
	}

	// The same rules on update, where only what is sent is checked.
	updates := []struct {
		table         string
		id            int64
		body, mention string
	}{
		{"item_categories", cat, `{"name": ""}`, "name is required"},
		{"item_categories", cat, `{"name": null}`, "name is required"},
		{"storage_locations", loc, `{"name": "` + long + `"}`, "name is too long"},
		{"storage_locations", loc, `{"nmae": "Closet"}`, `unknown field "nmae"`},
		{"statuses", 1, `{"color": null}`, "color is required"},
		{"statuses", 1, `{"shortcut": ""}`, "shortcut is required"},
		{"items", item, `{"item_category_id": 99}`, "does not exist"},
		{"items", item, `{"storage_location_id": 40000}`, "storage_location_id is out of range"},
		{"items", item, `{"is_container": "true"}`, "is_container must be true or false"},
		{"items", item, `{"name": 7}`, "name must be text"},
		// A bad field is refused even for a row that is not there.
		{"items", 4242, `{"colour": "red"}`, `unknown field "colour"`},
	}
	for _, c := range updates {
		_, err := st.UpdateRef(ctx, store.RefTableByKey(c.table), c.id, fields(t, c.body))
		wantValidation(t, "update "+c.table+" "+c.body, err, c.mention)
	}

	// Nothing was created or changed along the way.
	all, err := st.ListAllRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for table, names := range map[string][]string{
		"item_categories": {"Clothing"}, "storage_locations": {"Closet"}, "items": {"Socks"},
		"statuses": {"In Storage Location", "Packed", "Missing"},
	} {
		if got := rowNames(all[table]); !reflect.DeepEqual(got, names) {
			t.Errorf("%s after the refused requests: %v, want %v", table, got, names)
		}
	}
	if s := all["statuses"][0]; s["color"] != "#8a94a3" || s["shortcut"] != "s" {
		t.Errorf("first status changed: %v", s)
	}
}

// A name may be 127 characters, counted as characters, not bytes.
func TestRefNameLength(t *testing.T) {
	st := testdb.New(t)
	for _, name := range []string{strings.Repeat("n", 127), strings.Repeat("é", 127), strings.Repeat("袋", 127)} {
		row := mustCreate(t, st, "item_categories", mustJSON(t, map[string]any{"name": name}))
		if row["name"] != name {
			t.Errorf("127 characters did not come back whole: %v", row["name"])
		}
	}
	_, err := st.CreateRef(context.Background(), store.RefTableByKey("item_categories"), fields(t, mustJSON(t, map[string]any{"name": strings.Repeat("é", 128)})))
	wantValidation(t, "128 characters", err, "name is too long")
	// Surrounding blanks do not count.
	mustCreate(t, st, "storage_locations", mustJSON(t, map[string]any{"name": "  " + strings.Repeat("x", 127) + "  "}))
}

// ---------------------------------------------------------------- conflicts

func TestRefDuplicateNames(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	mustCreate(t, st, "item_categories", `{"name": "Clothing"}`)
	mustCreate(t, st, "item_categories", `{"name": "Camp"}`)
	mustCreate(t, st, "storage_locations", `{"name": "Closet"}`)
	mustCreate(t, st, "storage_locations", `{"name": "Garage"}`)
	mustCreate(t, st, "items", `{"name": "Socks", "item_category_id": 1, "storage_location_id": 1}`)
	mustCreate(t, st, "items", `{"name": "Comb", "item_category_id": 1, "storage_location_id": 1}`)

	cases := []struct {
		table, body string
		other       int64 // a row of the table with another name
		mention     string
	}{
		{"item_categories", `{"name": "Clothing"}`, 2, "there is already an item category with that name"},
		{"storage_locations", `{"name": " Closet "}`, 2, "there is already a storage location with that name"},
		{"statuses", `{"name": "Packed", "color": "#112233", "shortcut": "x"}`, 1, "there is already a status with that name"},
		// Item names are unique whatever the category.
		{"items", `{"name": "Socks", "item_category_id": 2, "storage_location_id": 2}`, 2,
			"there is already an item with that name (item names are unique across all categories)"},
	}
	for _, c := range cases {
		tbl := store.RefTableByKey(c.table)
		before := len(mustList(t, st, c.table))
		_, err := st.CreateRef(ctx, tbl, fields(t, c.body))
		wantConflict(t, "create "+c.table, err, c.mention)
		if after := len(mustList(t, st, c.table)); after != before {
			t.Errorf("%s: the refused create left a row", c.table)
		}
		// Renaming another row to the taken name is the same conflict.
		_, err = st.UpdateRef(ctx, tbl, c.other, fields(t, c.body))
		wantConflict(t, "rename "+c.table, err, c.mention)
	}
}

func TestRefDeleteInUse(t *testing.T) {
	fx := newFx(t)
	ctx := context.Background()
	st := fx.st
	categories, locations := store.RefTableByKey("item_categories"), store.RefTableByKey("storage_locations")
	items, statuses := store.RefTableByKey("items"), store.RefTableByKey("statuses")

	// A category or location with items in it.
	wantConflict(t, "category in use", st.DeleteRef(ctx, categories, fx.category),
		"this item category still has items in the library: move them to another category first")
	wantConflict(t, "location in use", st.DeleteRef(ctx, locations, fx.location),
		"this storage location still has items in the library: move them to another location first")
	otherCat := rowID(mustCreate(t, st, "item_categories", `{"name": "Other"}`))
	otherLoc := rowID(mustCreate(t, st, "storage_locations", `{"name": "Other"}`))
	for _, r := range mustList(t, st, "items") {
		mustUpdate(t, st, "items", rowID(r), mustJSON(t, map[string]any{"item_category_id": otherCat}))
	}
	if err := st.DeleteRef(ctx, categories, fx.category); err != nil {
		t.Errorf("category once its items have moved: %v", err)
	}
	wantConflict(t, "location still in use", st.DeleteRef(ctx, locations, fx.location), "still has items")
	for _, r := range mustList(t, st, "items") {
		mustUpdate(t, st, "items", rowID(r), mustJSON(t, map[string]any{"storage_location_id": otherLoc}))
	}
	if err := st.DeleteRef(ctx, locations, fx.location); err != nil {
		t.Errorf("location once its items have moved: %v", err)
	}

	// An item on a list, a container holding things on a list, a status in use.
	trip := fx.list("Trip", "bag", "socks in bag @packed")
	wantConflict(t, "item on a list", st.DeleteRef(ctx, items, fx.socks), "this item is on a list: take it off every list before deleting it")
	wantConflict(t, "container on a list", st.DeleteRef(ctx, items, fx.bag), "take it off every list before deleting it")
	wantConflict(t, "status in use", st.DeleteRef(ctx, statuses, fx.packed), "this status is in use on a list: give those entries another status first")
	if len(mustList(t, st, "items")) != 8 || len(mustList(t, st, "statuses")) != 3 {
		t.Fatal("a refused delete removed something")
	}
	// An item on no list and a status no row has can go.
	if err := st.DeleteRef(ctx, items, fx.comb); err != nil {
		t.Errorf("unused item: %v", err)
	}
	if err := st.DeleteRef(ctx, statuses, fx.missing); err != nil {
		t.Errorf("unused status: %v", err)
	}

	if _, err := st.SetAllStatuses(ctx, trip, fx.stored); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteRef(ctx, statuses, fx.packed); err != nil {
		t.Errorf("status once nothing has it: %v", err)
	}
	fx.del(fx.row(trip, "socks in bag").ID, false)
	if err := st.DeleteRef(ctx, items, fx.socks); err != nil {
		t.Errorf("item once it is off the list: %v", err)
	}
	wantConflict(t, "container still on the list", st.DeleteRef(ctx, items, fx.bag), "this item is on a list")
	fx.del(fx.row(trip, "bag").ID, false)
	if err := st.DeleteRef(ctx, items, fx.bag); err != nil {
		t.Errorf("container once it is off the list: %v", err)
	}
}

// ---------------------------------------------------------------- colours and shortcuts

func TestParseColor(t *testing.T) {
	cases := []struct {
		in, want string
		wantErr  bool
	}{
		{"#2E8540", "2e8540", false},
		{"2e8540", "2e8540", false},
		{"#2e8540", "2e8540", false},
		{" #ABCDEF ", "abcdef", false},
		{"000000", "000000", false},
		{"", "", true},
		{"#", "", true},
		{"green", "", true},
		{"#2e854", "", true},
		{"#2e85401", "", true},
		{"#2e854g", "", true},
		{"##2e8540", "", true},
		{"#2e 540", "", true},
		{"rgb(1,2,3)", "", true},
	}
	for _, c := range cases {
		got, err := store.ParseColor(c.in, "color")
		if (err != nil) != c.wantErr || got != c.want {
			t.Errorf("ParseColor(%q) = %q, %v; want %q, error=%v", c.in, got, err, c.want, c.wantErr)
		}
		if err != nil && (!isValidation(err) || !strings.Contains(err.Error(), "color must be a hex colour")) {
			t.Errorf("ParseColor(%q): want a validation error naming the field, got %v", c.in, err)
		}
	}
}

func TestParseShortcut(t *testing.T) {
	cases := []struct {
		in, want string
		wantErr  bool
	}{
		{"p", "p", false},
		{"P", "p", false},
		{" q ", "q", false},
		{"7", "7", false},
		{"!", "!", false},
		{"É", "é", false},
		{"袋", "袋", false},
		{"", "", true},
		{" ", "", true},
		{"\t", "", true},
		{"pp", "", true},
		{"p q", "", true},
		{"\u0007", "", true}, // a control character is not a key
		{"\u200b", "", true}, // nor is one that cannot be seen (a zero-width space)
	}
	for _, c := range cases {
		got, err := store.ParseShortcut(c.in, "shortcut")
		if (err != nil) != c.wantErr || got != c.want {
			t.Errorf("ParseShortcut(%q) = %q, %v; want %q, error=%v", c.in, got, err, c.want, c.wantErr)
		}
		if err != nil && (!isValidation(err) || !strings.Contains(err.Error(), "shortcut must be a single character")) {
			t.Errorf("ParseShortcut(%q): want a validation error naming the field, got %v", c.in, err)
		}
	}
}

func TestStatusColor(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	statuses := store.RefTableByKey("statuses")

	// With or without the "#", in either case, it comes back as "#rrggbb".
	a := mustCreate(t, st, "statuses", `{"name": "A", "color": "#2E8540", "shortcut": "a"}`)
	b := mustCreate(t, st, "statuses", `{"name": "B", "color": "2e8540", "shortcut": "b"}`)
	if a["color"] != "#2e8540" || b["color"] != "#2e8540" {
		t.Fatalf("colours came back as %v and %v, want #2e8540", a["color"], b["color"])
	}
	if got := mustUpdate(t, st, "statuses", rowID(a), `{"color": " #FFaa00 "}`); got["color"] != "#ffaa00" {
		t.Fatalf("updated colour %v", got["color"])
	}
	if got := rowNamed(mustList(t, st, "statuses"), "A"); got["color"] != "#ffaa00" {
		t.Fatalf("listed colour %v", got["color"])
	}
	// The database holds the six lower-case digits.
	var stored string
	if err := st.Pool.QueryRow(ctx, `SELECT Color FROM Statuses WHERE ID = $1`, rowID(a)).Scan(&stored); err != nil || stored != "ffaa00" {
		t.Fatalf("stored colour %q, %v", stored, err)
	}

	for _, bad := range []string{"blue", "#12345", "#1234567", "#12345g", "#", "12 345"} {
		_, err := st.CreateRef(ctx, statuses, fields(t, mustJSON(t, map[string]any{"name": "C", "color": bad, "shortcut": "c"})))
		wantValidation(t, "create with colour "+bad, err, "color must be a hex colour")
		_, err = st.UpdateRef(ctx, statuses, rowID(a), fields(t, mustJSON(t, map[string]any{"color": bad})))
		wantValidation(t, "update to colour "+bad, err, "color must be a hex colour")
	}
	if got := rowNamed(mustList(t, st, "statuses"), "A"); got["color"] != "#ffaa00" || len(mustList(t, st, "statuses")) != 5 {
		t.Fatalf("a refused colour changed something: %v", got)
	}
}

func TestStatusShortcut(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	statuses := store.RefTableByKey("statuses")
	create := func(name, shortcut string) (store.Row, error) {
		return st.CreateRef(ctx, statuses, fields(t, mustJSON(t, map[string]any{"name": name, "color": "#112233", "shortcut": shortcut})))
	}

	// Stored in lower case, whatever was typed.
	worn, err := create("Worn", " W ")
	if err != nil || worn["shortcut"] != "w" {
		t.Fatalf("created %v, %v", worn, err)
	}
	if digit, err := create("One", "1"); err != nil || digit["shortcut"] != "1" {
		t.Fatalf("digit: %v, %v", digit, err)
	}
	if sym, err := create("Query", "?"); err != nil || sym["shortcut"] != "?" {
		t.Fatalf("symbol: %v, %v", sym, err)
	}

	// One visible character.
	for _, bad := range []string{"ab", "w w", "\u0007"} {
		_, err := create("Bad", bad)
		wantValidation(t, "shortcut "+bad, err, "shortcut must be a single character")
	}
	for _, blank := range []string{"", " ", "\t"} {
		_, err := create("Bad", blank)
		wantValidation(t, "blank shortcut", err, "shortcut is required")
	}

	// Unique, and "P" is "p": the seeded Packed has it.
	const taken = "another status already uses that shortcut key"
	_, err = create("Posted", "p")
	wantConflict(t, "taken shortcut", err, taken)
	_, err = create("Posted", "P")
	wantConflict(t, "taken shortcut in upper case", err, taken)
	_, err = create("Washed", "W")
	wantConflict(t, "taken by a status made here", err, taken)
	_, err = st.UpdateRef(ctx, statuses, rowID(worn), fields(t, `{"shortcut": "M"}`))
	wantConflict(t, "update to a taken shortcut", err, taken)
	_, err = st.UpdateRef(ctx, statuses, rowID(worn), fields(t, `{"shortcut": "mm"}`))
	wantValidation(t, "update to two characters", err, "shortcut must be a single character")

	// A status can keep its own key, and a freed key can be reused.
	if got := mustUpdate(t, st, "statuses", rowID(worn), `{"shortcut": "W", "name": "Worn Out"}`); got["shortcut"] != "w" {
		t.Fatalf("kept its own key: %v", got)
	}
	mustUpdate(t, st, "statuses", 2, `{"shortcut": "k"}`)
	if got, err := create("Posted", "P"); err != nil || got["shortcut"] != "p" {
		t.Fatalf("reusing a freed key: %v, %v", got, err)
	}
	if n := len(mustList(t, st, "statuses")); n != 7 {
		t.Fatalf("%d statuses, want 7: a refused create left a row", n)
	}
}

// ---------------------------------------------------------------- computed columns

func TestRefComputedColumns(t *testing.T) {
	fx := newFx(t)
	st := fx.st
	count := func(table, name, key string) int64 {
		t.Helper()
		row := rowNamed(mustList(t, st, table), name)
		if row == nil {
			t.Fatalf("no %s named %q", table, name)
		}
		return num(row[key])
	}

	// item_count: the items in a category, at a location.
	mustCreate(t, st, "item_categories", `{"name": "Empty"}`)
	mustCreate(t, st, "storage_locations", `{"name": "Attic"}`)
	if a, b := count("item_categories", "Gear", "item_count"), count("item_categories", "Empty", "item_count"); a != 8 || b != 0 {
		t.Fatalf("category item_count %d and %d, want 8 and 0", a, b)
	}
	if a, b := count("storage_locations", "Garage", "item_count"), count("storage_locations", "Attic", "item_count"); a != 8 || b != 0 {
		t.Fatalf("location item_count %d and %d, want 8 and 0", a, b)
	}
	mustUpdate(t, st, "items", fx.socks, `{"item_category_id": 2, "storage_location_id": 2}`)
	mustCreate(t, st, "items", `{"name": "Hat", "item_category_id": 2, "storage_location_id": 1}`)
	if a, b := count("item_categories", "Gear", "item_count"), count("item_categories", "Empty", "item_count"); a != 7 || b != 2 {
		t.Fatalf("category item_count after a move %d and %d, want 7 and 2", a, b)
	}
	if a, b := count("storage_locations", "Garage", "item_count"), count("storage_locations", "Attic", "item_count"); a != 8 || b != 1 {
		t.Fatalf("location item_count after a move %d and %d, want 8 and 1", a, b)
	}

	// list_count: the lists an item is on, however often it is on each;
	// use_count: the rows with a status, over all lists.
	trip := fx.list("Trip", "bag", "kit in bag", "socks in bag @packed", "socks in kit @packed", "socks @missing")
	fx.list("Day Out", "bag @packed", "socks in bag")
	for name, want := range map[string]int64{"socks": 2, "bag": 2, "kit": 1, "comb": 0} {
		if got := count("items", name, "list_count"); got != want {
			t.Errorf("list_count of %s = %d, want %d", name, got, want)
		}
	}
	for name, want := range map[string]int64{"In Storage Location": 3, "Packed": 3, "Missing": 1} {
		if got := count("statuses", name, "use_count"); got != want {
			t.Errorf("use_count of %s = %d, want %d", name, got, want)
		}
	}
	// They come back from an update too, and follow the lists.
	if got := mustUpdate(t, st, "statuses", fx.packed, `{"name": "Packed!"}`); num(got["use_count"]) != 3 {
		t.Errorf("use_count in the updated row: %v", got)
	}
	if got := mustUpdate(t, st, "items", fx.socks, `{}`); num(got["list_count"]) != 2 {
		t.Errorf("list_count in the updated row: %v", got)
	}
	if err := st.DeleteList(context.Background(), trip); err != nil {
		t.Fatal(err)
	}
	if a, b, c := count("items", "socks", "list_count"), count("items", "kit", "list_count"), count("statuses", "Packed!", "use_count"); a != 1 || b != 0 || c != 1 {
		t.Errorf("after deleting a list: socks on %d lists, kit on %d, %d rows packed; want 1, 0, 1", a, b, c)
	}
}

// ---------------------------------------------------------------- the Is_Container guard

func TestContainerFlagGuard(t *testing.T) {
	fx := newFx(t)
	ctx := context.Background()
	items := store.RefTableByKey("items")
	flag := func(item int64, v bool) error {
		_, err := fx.st.UpdateRef(ctx, items, item, fields(t, mustJSON(t, map[string]any{"is_container": v})))
		return err
	}

	zoo := fx.list("Zoo Trip", "car", "bag in car", "socks in bag", "socks in car", "comb")
	beach := fx.list("Beach", "bag", "towel in bag", "socks", "lamp", "kit")

	// A container that holds something on a list cannot stop being one. The
	// message names the list (the first by name when there are several).
	wantConflict(t, "unset while holding things", flag(fx.bag, false),
		`this item holds other items on the list "Beach": empty it there before it stops being a container`)
	wantConflict(t, "unset while holding things", flag(fx.car, false), `on the list "Zoo Trip"`)
	// Sending the value it has is not a change, and the other columns can
	// change all the same.
	if err := flag(fx.bag, true); err != nil {
		t.Errorf("setting the flag a full container already has: %v", err)
	}
	if got := mustUpdate(t, fx.st, "items", fx.bag, `{"name": "Blue Backpack", "is_container": true}`); got["name"] != "Blue Backpack" {
		t.Errorf("rename of a full container: %v", got)
	}
	// A refused change changes nothing, the other columns included.
	_, err := fx.st.UpdateRef(ctx, items, fx.car, fields(t, `{"name": "Van", "is_container": false}`))
	wantConflict(t, "unset with a rename", err, "holds other items")
	if row := rowNamed(mustList(t, fx.st, "items"), "car"); row == nil || row["is_container"] != true {
		t.Errorf("the refused update changed the car: %v", row)
	}
	// An empty container can: the kit is on a list, with nothing in it.
	if err := flag(fx.kit, false); err != nil {
		t.Errorf("unset an empty container: %v", err)
	}
	// So can the bag once it is empty everywhere.
	fx.del(fx.row(beach, "towel in bag").ID, false)
	wantConflict(t, "still full on the other list", flag(fx.bag, false), `on the list "Zoo Trip"`)

	// Something on a list more than once cannot become a container.
	wantConflict(t, "set while on a list twice", flag(fx.socks, true),
		`this item is on the list "Zoo Trip" more than once, and a container can only be in one place on a list`)
	// Once on each of several lists is fine, and so is on none.
	if err := flag(fx.comb, true); err != nil {
		t.Errorf("set for an item on one list: %v", err)
	}
	if err := flag(fx.towel, true); err != nil {
		t.Errorf("set for an item on no list: %v", err)
	}
	fx.del(fx.row(zoo, "socks in car").ID, false)
	if err := flag(fx.socks, true); err != nil {
		t.Errorf("set for an item on two lists, once each: %v", err)
	}
	// And then the rules of a container hold for it: it cannot be added again.
	_, err = fx.addErr(beach, "socks in bag")
	wantConflict(t, "adding the new container a second time", err, "is a container and is already on this list")

	wantNotFound(t, "flag of a missing item", flag(4242, true))
}
