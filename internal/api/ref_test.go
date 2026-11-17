package api_test

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestRefAPIListAll(t *testing.T) {
	e := newEnv(t)
	status, v, raw := e.call("GET", "/api/ref", nil)
	all, _ := v.(jmap)
	if status != http.StatusOK || !reflect.DeepEqual(jsonKeys(all), []string{"item_categories", "items", "statuses", "storage_locations"}) {
		t.Fatalf("GET /api/ref: %d %s", status, raw)
	}
	// Empty tables are empty arrays, not null.
	for _, table := range []string{"item_categories", "storage_locations", "items"} {
		if !reflect.DeepEqual(all[table], []any{}) {
			t.Errorf("%s: %v, want []", table, all[table])
		}
		if status, _, raw := e.call("GET", "/api/ref/"+table, nil); status != http.StatusOK || raw != `[]` {
			t.Errorf("GET /api/ref/%s: %d %s", table, status, raw)
		}
	}
	// The seeded statuses, in the order of their IDs, colours with their "#".
	want := []any{
		jmap{"id": 1.0, "name": "In Storage Location", "color": "#8a94a3", "shortcut": "s", "use_count": 0.0},
		jmap{"id": 2.0, "name": "Packed", "color": "#2e8540", "shortcut": "p", "use_count": 0.0},
		jmap{"id": 3.0, "name": "Missing", "color": "#d62828", "shortcut": "m", "use_count": 0.0},
	}
	if !reflect.DeepEqual(all["statuses"], want) {
		t.Fatalf("statuses:\n got  %v\n want %v", all["statuses"], want)
	}
	if got := e.arr(http.StatusOK, "GET", "/api/ref/statuses", nil); !reflect.DeepEqual(got, want) {
		t.Fatalf("GET /api/ref/statuses: %v", got)
	}
}

func TestRefAPICreateUpdateDelete(t *testing.T) {
	e := newEnv(t)

	// Categories and locations: a name and a count.
	for _, table := range []string{"item_categories", "storage_locations"} {
		path := "/api/ref/" + table
		row := e.obj(http.StatusCreated, "POST", path, jmap{"name": "  Garage "})
		if want := (jmap{"id": 1.0, "name": "Garage", "item_count": 0.0}); !reflect.DeepEqual(row, want) {
			t.Fatalf("POST %s: %v, want %v", path, row, want)
		}
		e.obj(http.StatusCreated, "POST", path, jmap{"name": "attic"})
		if got := names(e.arr(http.StatusOK, "GET", path, nil)); !reflect.DeepEqual(got, []string{"attic", "Garage"}) {
			t.Fatalf("GET %s: %v", path, got)
		}
		row = e.obj(http.StatusOK, "PATCH", path+"/1", jmap{"name": "Shed"})
		if want := (jmap{"id": 1.0, "name": "Shed", "item_count": 0.0}); !reflect.DeepEqual(row, want) {
			t.Fatalf("PATCH %s/1: %v, want %v", path, row, want)
		}
		// An empty patch changes nothing and returns the row.
		if row = e.obj(http.StatusOK, "PATCH", path+"/1", jmap{}); row["name"] != "Shed" {
			t.Fatalf("empty PATCH %s/1: %v", path, row)
		}
		e.gone("DELETE", path+"/2")
		e.fails(http.StatusNotFound, "DELETE", path+"/2", nil, "not found")
		e.fails(http.StatusNotFound, "PATCH", path+"/2", jmap{"name": "Attic"}, "not found")
		if got := names(e.arr(http.StatusOK, "GET", path, nil)); !reflect.DeepEqual(got, []string{"Shed"}) {
			t.Fatalf("GET %s after the delete: %v", path, got)
		}
	}

	// Statuses: the colour comes back as "#rrggbb", the shortcut in lower case.
	for i, color := range []string{"#2E8540", "2e8540"} {
		row := e.obj(http.StatusCreated, "POST", "/api/ref/statuses", jmap{"name": fmt.Sprint("Status ", i), "color": color, "shortcut": string(rune('X' + i))})
		want := jmap{"id": float64(4 + i), "name": fmt.Sprint("Status ", i), "color": "#2e8540", "shortcut": string(rune('x' + i)), "use_count": 0.0}
		if !reflect.DeepEqual(row, want) {
			t.Fatalf("POST status with colour %s: %v, want %v", color, row, want)
		}
	}
	// PATCH is partial: only the colour changes.
	row := e.obj(http.StatusOK, "PATCH", "/api/ref/statuses/4", jmap{"color": "AABBCC"})
	if want := (jmap{"id": 4.0, "name": "Status 0", "color": "#aabbcc", "shortcut": "x", "use_count": 0.0}); !reflect.DeepEqual(row, want) {
		t.Fatalf("PATCH status colour: %v, want %v", row, want)
	}
	row = e.obj(http.StatusOK, "PATCH", "/api/ref/statuses/4", jmap{"shortcut": "Q", "name": "Queued"})
	if want := (jmap{"id": 4.0, "name": "Queued", "color": "#aabbcc", "shortcut": "q", "use_count": 0.0}); !reflect.DeepEqual(row, want) {
		t.Fatalf("PATCH status shortcut and name: %v, want %v", row, want)
	}
	e.gone("DELETE", "/api/ref/statuses/5")
	if got := names(e.arr(http.StatusOK, "GET", "/api/ref/statuses", nil)); !reflect.DeepEqual(got, []string{"In Storage Location", "Packed", "Missing", "Queued"}) {
		t.Fatalf("statuses: %v", got)
	}

	// Items.
	item := e.obj(http.StatusCreated, "POST", "/api/ref/items", jmap{"name": "Socks", "item_category_id": 1, "storage_location_id": 1})
	want := jmap{"id": 1.0, "item_category_id": 1.0, "name": "Socks", "storage_location_id": 1.0, "is_container": false, "list_count": 0.0}
	if !reflect.DeepEqual(item, want) {
		t.Fatalf("POST item: %v, want %v", item, want)
	}
	bag := e.obj(http.StatusCreated, "POST", "/api/ref/items", jmap{"name": "Bag", "item_category_id": 1, "storage_location_id": 1, "is_container": true})
	if bag["is_container"] != true || bag["id"] != 2.0 {
		t.Fatalf("POST container: %v", bag)
	}
	// PATCH is partial: the flag alone, then the name alone.
	want["is_container"] = true
	if got := e.obj(http.StatusOK, "PATCH", "/api/ref/items/1", jmap{"is_container": true}); !reflect.DeepEqual(got, want) {
		t.Fatalf("PATCH item flag: %v, want %v", got, want)
	}
	want["name"] = "Socks, Wool"
	if got := e.obj(http.StatusOK, "PATCH", "/api/ref/items/1", jmap{"name": "Socks, Wool"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("PATCH item name: %v, want %v", got, want)
	}
	if got := e.arr(http.StatusOK, "GET", "/api/ref/items", nil); len(got) != 2 || !reflect.DeepEqual(got[1], any(want)) {
		t.Fatalf("GET items: %v", got)
	}
	// Everything shows up in the combined listing.
	all := e.obj(http.StatusOK, "GET", "/api/ref", nil)
	if got := names(all["items"].([]any)); !reflect.DeepEqual(got, []string{"Bag", "Socks, Wool"}) {
		t.Fatalf("items in GET /api/ref: %v", got)
	}
	if cat := all["item_categories"].([]any)[0].(jmap); cat["item_count"] != 2.0 {
		t.Fatalf("category in GET /api/ref: %v", cat)
	}
	e.gone("DELETE", "/api/ref/items/1")
	e.fails(http.StatusNotFound, "DELETE", "/api/ref/items/1", nil, "not found")
}

func TestRefAPIErrors(t *testing.T) {
	e := newEnv(t)
	e.obj(http.StatusCreated, "POST", "/api/ref/item_categories", jmap{"name": "Gear"})
	e.obj(http.StatusCreated, "POST", "/api/ref/item_categories", jmap{"name": "Clothing"})
	e.obj(http.StatusCreated, "POST", "/api/ref/storage_locations", jmap{"name": "Garage"})
	item := func(name string, container bool) {
		t.Helper()
		e.obj(http.StatusCreated, "POST", "/api/ref/items", jmap{"name": name, "item_category_id": 1, "storage_location_id": 1, "is_container": container})
	}
	item("bag", true) // 1
	item("socks", false)
	item("comb", false)
	e.obj(http.StatusCreated, "POST", "/api/lists", jmap{"name": "Trip"})
	for _, body := range []jmap{{"item_id": 1}, {"item_id": 2, "container_id": 1, "status_id": 2}, {"item_id": 2}} {
		e.obj(http.StatusCreated, "POST", "/api/lists/1/contents", body)
	}

	long := strings.Repeat("n", 128)
	cases := []struct {
		status             int
		method, path, body string
		mention            string
	}{
		// 400: the request is wrong.
		{400, "POST", "/api/ref/item_categories", `{}`, "name is required"},
		{400, "POST", "/api/ref/item_categories", `{"name": " "}`, "name is required"},
		{400, "POST", "/api/ref/storage_locations", `{"name": "` + long + `"}`, "name is too long (at most 127 characters)"},
		{400, "PATCH", "/api/ref/storage_locations/1", `{"name": null}`, "name is required"},
		{400, "POST", "/api/ref/statuses", `{"name": "Worn", "color": "green", "shortcut": "w"}`, "color must be a hex colour"},
		{400, "POST", "/api/ref/statuses", `{"name": "Worn", "color": "#112233", "shortcut": "ww"}`, "shortcut must be a single character"},
		{400, "POST", "/api/ref/statuses", `{"name": "Worn", "color": "#112233"}`, "shortcut is required"},
		{400, "PATCH", "/api/ref/statuses/1", `{"color": "#12345"}`, "color must be a hex colour"},
		{400, "POST", "/api/ref/items", `{"name": "lamp", "storage_location_id": 1}`, "item_category_id is required"},
		{400, "POST", "/api/ref/items", `{"name": "lamp", "item_category_id": 99, "storage_location_id": 1}`, "does not exist"},
		{400, "POST", "/api/ref/items", `{"name": "lamp", "item_category_id": 1, "storage_location_id": 1, "is_container": "no"}`, "is_container must be true or false"},
		{400, "PATCH", "/api/ref/items/3", `{"storage_location_id": 99}`, "does not exist"},
		// 409: the data forbids it. Duplicates...
		{409, "POST", "/api/ref/item_categories", `{"name": "Gear"}`, "there is already an item category with that name"},
		{409, "PATCH", "/api/ref/item_categories/2", `{"name": "Gear"}`, "there is already an item category with that name"},
		{409, "POST", "/api/ref/storage_locations", `{"name": "Garage"}`, "there is already a storage location with that name"},
		{409, "POST", "/api/ref/statuses", `{"name": "Packed", "color": "#112233", "shortcut": "w"}`, "there is already a status with that name"},
		{409, "POST", "/api/ref/statuses", `{"name": "Posted", "color": "#112233", "shortcut": "P"}`, "another status already uses that shortcut key"},
		{409, "PATCH", "/api/ref/statuses/1", `{"shortcut": "m"}`, "another status already uses that shortcut key"},
		{409, "POST", "/api/ref/items", `{"name": "socks", "item_category_id": 2, "storage_location_id": 1}`, "item names are unique across all categories"},
		{409, "PATCH", "/api/ref/items/3", `{"name": "socks"}`, "there is already an item with that name"},
		// ...rows in use...
		{409, "DELETE", "/api/ref/item_categories/1", ``, "this item category still has items in the library"},
		{409, "DELETE", "/api/ref/storage_locations/1", ``, "this storage location still has items in the library"},
		{409, "DELETE", "/api/ref/items/2", ``, "this item is on a list: take it off every list before deleting it"},
		{409, "DELETE", "/api/ref/items/1", ``, "take it off every list before deleting it"},
		{409, "DELETE", "/api/ref/statuses/1", ``, "this status is in use on a list"},
		{409, "DELETE", "/api/ref/statuses/2", ``, "this status is in use on a list"},
		// ...and the container flag of an item the lists depend on.
		{409, "PATCH", "/api/ref/items/1", `{"is_container": false}`, `this item holds other items on the list "Trip"`},
		{409, "PATCH", "/api/ref/items/2", `{"is_container": true}`, `this item is on the list "Trip" more than once`},
	}
	for _, c := range cases {
		var body any
		if c.body != "" {
			body = c.body
		}
		e.fails(c.status, c.method, c.path, body, c.mention)
	}

	// Nothing changed; what is not in use can go.
	all := e.obj(http.StatusOK, "GET", "/api/ref", nil)
	for table, want := range map[string][]string{
		"item_categories": {"Clothing", "Gear"}, "storage_locations": {"Garage"},
		"statuses": {"In Storage Location", "Packed", "Missing"}, "items": {"bag", "comb", "socks"},
	} {
		if got := names(all[table].([]any)); !reflect.DeepEqual(got, want) {
			t.Errorf("%s after the refused requests: %v, want %v", table, got, want)
		}
	}
	items := all["items"].([]any)
	if bag, socks, comb := items[0].(jmap), items[2].(jmap), items[1].(jmap); bag["is_container"] != true || socks["is_container"] != false ||
		bag["list_count"] != 1.0 || socks["list_count"] != 1.0 || comb["list_count"] != 0.0 {
		t.Errorf("items after the refused requests: %v", items)
	}
	if st := all["statuses"].([]any); st[0].(jmap)["use_count"] != 2.0 || st[1].(jmap)["use_count"] != 1.0 || st[2].(jmap)["use_count"] != 0.0 {
		t.Errorf("status use counts: %v", st)
	}
	e.gone("DELETE", "/api/ref/statuses/3")
	e.gone("DELETE", "/api/ref/items/3")
	e.gone("DELETE", "/api/ref/item_categories/2")
}
