package api_test

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

// listAPI is the API over a database with a small library, made through
// the API itself: three containers and three things that are not. The bag
// has two compartments (a lid and a pocket) and the car one (a boot).
type listAPI struct {
	*env
	car, bag, kit      float64 // containers
	socks, comb, towel float64
	lid, pocket, boot  float64 // compartments
	itemName           map[float64]string
	compartmentName    map[float64]string
}

// The seeded statuses.
const (
	stored  = 1.0
	packed  = 2.0
	missing = 3.0
)

func newListAPI(t *testing.T) *listAPI {
	t.Helper()
	a := &listAPI{env: newEnv(t), itemName: map[float64]string{}, compartmentName: map[float64]string{}}
	cat := a.obj(http.StatusCreated, "POST", "/api/ref/item_categories", jmap{"name": "Gear"})["id"]
	loc := a.obj(http.StatusCreated, "POST", "/api/ref/storage_locations", jmap{"name": "Garage"})["id"]
	item := func(name string, container bool) float64 {
		t.Helper()
		id := a.obj(http.StatusCreated, "POST", "/api/ref/items", jmap{
			"name": name, "item_category_id": cat, "storage_location_id": loc, "is_container": container,
		})["id"].(float64)
		a.itemName[id] = name
		return id
	}
	a.car, a.bag, a.kit = item("car", true), item("bag", true), item("kit", true)
	a.socks, a.comb, a.towel = item("socks", false), item("comb", false), item("towel", false)
	compartment := func(container float64, name string) float64 {
		t.Helper()
		id := a.obj(http.StatusCreated, "POST", "/api/ref/compartments", jmap{"item_id": container, "name": name})["id"].(float64)
		a.compartmentName[id] = name
		return id
	}
	a.lid, a.pocket, a.boot = compartment(a.bag, "lid"), compartment(a.bag, "pocket"), compartment(a.car, "boot")
	return a
}

// list makes a list through the API and returns its id.
func (a *listAPI) list(name string) float64 {
	a.t.Helper()
	return a.obj(http.StatusCreated, "POST", "/api/lists", jmap{"name": name})["id"].(float64)
}

// add puts an item on a list (container 0 = the top level), with any other
// fields, and returns the new row's id.
func (a *listAPI) add(list, item, container float64, extra jmap) float64 {
	a.t.Helper()
	body := jmap{"item_id": item}
	if container != 0 {
		body["container_id"] = container
	}
	for k, v := range extra {
		body[k] = v
	}
	return a.obj(http.StatusCreated, "POST", fmt.Sprintf("/api/lists/%v/contents", list), body)["id"].(float64)
}

// places lists a list's rows in order as "socks in bag", or "socks in
// bag/lid" when in a compartment ("bag" at the top level).
func (a *listAPI) places(list float64) []string {
	a.t.Helper()
	out := []string{}
	for _, r := range a.obj(http.StatusOK, "GET", fmt.Sprintf("/api/lists/%v", list), nil)["contents"].([]any) {
		row := r.(jmap)
		s := a.itemName[row["item_id"].(float64)]
		if c, ok := row["container_id"].(float64); ok {
			s += " in " + a.itemName[c]
		}
		if c, ok := row["compartment_id"].(float64); ok {
			s += "/" + a.compartmentName[c]
		}
		out = append(out, s)
	}
	return out
}

func (a *listAPI) wantPlaces(list float64, want ...string) {
	a.t.Helper()
	if want == nil {
		want = []string{}
	}
	if got := a.places(list); !reflect.DeepEqual(got, want) {
		a.t.Fatalf("list %v:\n got  %q\n want %q", list, got, want)
	}
}

// ---------------------------------------------------------------- lists

// Every route's response has the documented shape: every key, nothing more,
// with nulls and empty arrays where there is nothing.
func TestListsAPIShapes(t *testing.T) {
	a := newListAPI(t)
	if status, _, raw := a.call("GET", "/api/lists", nil); status != http.StatusOK || raw != `[]` {
		t.Fatalf("no lists: %d %s", status, raw)
	}

	status, _, raw := a.call("POST", "/api/lists", jmap{"name": "  Trip "})
	if status != http.StatusCreated || raw != `{"id":1,"name":"Trip","is_template":false,"rows":0,"status_counts":{},"contents":[]}` {
		t.Fatalf("POST /api/lists: %d %s", status, raw)
	}
	tpl := a.obj(http.StatusCreated, "POST", "/api/lists", jmap{"name": "Basics", "is_template": true, "sources": []int{}})
	if tpl["id"] != 2.0 || tpl["is_template"] != true {
		t.Fatalf("POST a template: %v", tpl)
	}

	// A row with nothing optional, and one with everything.
	bare := a.obj(http.StatusCreated, "POST", "/api/lists/1/contents", jmap{"item_id": a.bag})
	wantBare := jmap{"id": 1.0, "list_id": 1.0, "container_id": nil, "compartment_id": nil, "item_id": a.bag, "quantity": 1.0, "unit": nil, "status_id": stored, "notes": nil}
	if !reflect.DeepEqual(bare, wantBare) {
		t.Fatalf("POST a bare row:\n got  %v\n want %v", bare, wantBare)
	}
	full := a.obj(http.StatusCreated, "POST", "/api/lists/1/contents",
		jmap{"item_id": a.socks, "container_id": a.bag, "compartment_id": a.lid, "quantity": 3, "unit": " pairs ", "status_id": packed, "notes": "wool"})
	wantFull := jmap{"id": 2.0, "list_id": 1.0, "container_id": a.bag, "compartment_id": a.lid, "item_id": a.socks, "quantity": 3.0, "unit": "pairs", "status_id": packed, "notes": "wool"}
	if !reflect.DeepEqual(full, wantFull) {
		t.Fatalf("POST a full row:\n got  %v\n want %v", full, wantFull)
	}

	// The list with its contents; status counts are keyed by status id.
	wantList := jmap{"id": 1.0, "name": "Trip", "is_template": false, "rows": 2.0,
		"status_counts": jmap{"1": 1.0, "2": 1.0}, "contents": []any{wantBare, wantFull}}
	if got := a.obj(http.StatusOK, "GET", "/api/lists/1", nil); !reflect.DeepEqual(got, wantList) {
		t.Fatalf("GET /api/lists/1:\n got  %v\n want %v", got, wantList)
	}
	// The listing has the summaries, by name, without the contents.
	wantAll := []any{
		jmap{"id": 2.0, "name": "Basics", "is_template": true, "rows": 0.0, "status_counts": jmap{}},
		jmap{"id": 1.0, "name": "Trip", "is_template": false, "rows": 2.0, "status_counts": jmap{"1": 1.0, "2": 1.0}},
	}
	if got := a.arr(http.StatusOK, "GET", "/api/lists", nil); !reflect.DeepEqual(got, wantAll) {
		t.Fatalf("GET /api/lists:\n got  %v\n want %v", got, wantAll)
	}

	// PATCH of a list and POST of its statuses return the list as GET does.
	wantList["name"] = "Road Trip"
	if got := a.obj(http.StatusOK, "PATCH", "/api/lists/1", jmap{"name": "Road Trip"}); !reflect.DeepEqual(got, wantList) {
		t.Fatalf("PATCH /api/lists/1:\n got  %v\n want %v", got, wantList)
	}
	wantBare["status_id"], wantFull["status_id"], wantList["status_counts"] = missing, missing, jmap{"3": 2.0}
	if got := a.obj(http.StatusOK, "POST", "/api/lists/1/statuses", jmap{"status_id": missing}); !reflect.DeepEqual(got, wantList) {
		t.Fatalf("POST /api/lists/1/statuses:\n got  %v\n want %v", got, wantList)
	}
	// PATCH of a row returns the row.
	wantFull["quantity"] = 4.0
	if got := a.obj(http.StatusOK, "PATCH", "/api/contents/2", jmap{"quantity": 4}); !reflect.DeepEqual(got, wantFull) {
		t.Fatalf("PATCH /api/contents/2:\n got  %v\n want %v", got, wantFull)
	}
	// The preview is one count.
	if status, _, raw := a.call("POST", "/api/lists/preview", jmap{"sources": []int{1, 2}}); status != http.StatusOK || raw != `{"rows":2}` {
		t.Fatalf("POST /api/lists/preview: %d %s", status, raw)
	}
	// Deletes answer 204 with no body.
	a.gone("DELETE", "/api/contents/2")
	a.gone("DELETE", "/api/lists/2")
	a.gone("DELETE", "/api/lists/1")
	if status, _, raw := a.call("GET", "/api/lists", nil); status != http.StatusOK || raw != `[]` {
		t.Fatalf("no lists again: %d %s", status, raw)
	}
}

func TestListsAPICreateFromSources(t *testing.T) {
	a := newListAPI(t)
	camp := a.obj(http.StatusCreated, "POST", "/api/lists", jmap{"name": "Camping", "is_template": true})["id"].(float64)
	a.add(camp, a.car, 0, jmap{"status_id": packed})
	a.add(camp, a.bag, a.car, nil)
	a.add(camp, a.socks, a.bag, jmap{"quantity": 2, "status_id": missing})
	a.add(camp, a.towel, a.car, nil)
	city := a.list("City Break")
	a.add(city, a.bag, 0, jmap{"status_id": packed})
	a.add(city, a.socks, a.bag, jmap{"quantity": 5, "status_id": packed})
	a.add(city, a.comb, a.bag, nil)

	// The preview counts the rows the new list would have.
	preview := func(sources ...float64) float64 {
		t.Helper()
		p := a.obj(http.StatusOK, "POST", "/api/lists/preview", jmap{"sources": sources})
		if !reflect.DeepEqual(jsonKeys(p), []string{"rows"}) {
			t.Fatalf("preview %v: %v", sources, p)
		}
		return p["rows"].(float64)
	}
	for _, c := range []struct {
		sources []float64
		rows    float64
	}{{[]float64{camp, city}, 5}, {[]float64{city, camp}, 5}, {[]float64{camp}, 4}, {[]float64{city, city}, 3}, {[]float64{}, 0}, {nil, 0}} {
		if got := preview(c.sources...); got != c.rows {
			t.Fatalf("preview of %v: %v rows, want %v", c.sources, got, c.rows)
		}
	}
	if got := a.obj(http.StatusOK, "POST", "/api/lists/preview", jmap{}); !reflect.DeepEqual(got, jmap{"rows": 0.0}) {
		t.Fatalf("preview without sources: %v", got)
	}

	// Creating makes just that: everything on either list.
	both := a.obj(http.StatusCreated, "POST", "/api/lists", jmap{"name": "Both", "sources": []float64{camp, city}})
	id := both["id"].(float64)
	a.wantPlaces(id, "car", "bag in car", "socks in bag", "towel in car", "comb in bag")
	if both["rows"] != 5.0 || both["is_template"] != false || len(both["contents"].([]any)) != 5 ||
		!reflect.DeepEqual(both["status_counts"], jmap{"1": 3.0, "2": 1.0, "3": 1.0}) {
		t.Fatalf("both: %v", both)
	}
	// The template's statuses are kept, the larger quantity wins, and the
	// ordinary list's comb starts at the first status.
	rows := both["contents"].([]any)
	if car, socks, comb := rows[0].(jmap), rows[2].(jmap), rows[4].(jmap); car["status_id"] != packed ||
		socks["status_id"] != missing || socks["quantity"] != 5.0 || comb["status_id"] != stored || car["list_id"] != id {
		t.Fatalf("rows of both: %v", rows)
	}
	// The sources count in the order given: the other way round the bag
	// stays at the top level, where the first list has it.
	reversed := a.obj(http.StatusCreated, "POST", "/api/lists", jmap{"name": "Reversed", "is_template": true, "sources": []float64{city, camp}})
	a.wantPlaces(reversed["id"].(float64), "bag", "socks in bag", "comb in bag", "car", "towel in car")
	if reversed["rows"] != 5.0 || reversed["is_template"] != true {
		t.Fatalf("reversed: %v", reversed)
	}

	// There is one way to combine lists: "mode" is not a field.
	a.fails(http.StatusBadRequest, "POST", "/api/lists", jmap{"name": "New", "sources": []float64{camp, city}, "mode": "union"}, `unknown field "mode"`)
	a.fails(http.StatusBadRequest, "POST", "/api/lists", jmap{"name": "New", "sources": []float64{camp, 999}}, "sources: there is no list 999")
	a.fails(http.StatusBadRequest, "POST", "/api/lists", jmap{"name": "New", "sources": []float64{40000}}, "sources: there is no list 40000")
	a.fails(http.StatusBadRequest, "POST", "/api/lists", jmap{"name": " ", "sources": []float64{camp}}, "name is required")
	a.fails(http.StatusBadRequest, "POST", "/api/lists", jmap{"sources": []float64{camp}}, "name is required")
	a.fails(http.StatusBadRequest, "POST", "/api/lists", jmap{"name": strings.Repeat("n", 128)}, "name is too long")
	a.fails(http.StatusConflict, "POST", "/api/lists", jmap{"name": "Camping"}, "there is already a list with that name")
	a.fails(http.StatusConflict, "POST", "/api/lists", jmap{"name": "Both", "sources": []float64{camp}}, "there is already a list with that name")
	a.fails(http.StatusBadRequest, "POST", "/api/lists/preview", jmap{"sources": []float64{camp, 999}}, "sources: there is no list 999")
	if got := names(a.arr(http.StatusOK, "GET", "/api/lists", nil)); !reflect.DeepEqual(got, []string{"Both", "Camping", "City Break", "Reversed"}) {
		t.Fatalf("lists: %v", got)
	}
}

func TestListsAPIPatchAndDelete(t *testing.T) {
	a := newListAPI(t)
	trip := a.list("Trip")
	a.list("Other")
	a.add(trip, a.bag, 0, nil)
	a.add(trip, a.socks, a.bag, nil)
	path := fmt.Sprintf("/api/lists/%v", trip)

	// Partial: a key that is absent is left alone.
	d := a.obj(http.StatusOK, "PATCH", path, jmap{"is_template": true})
	if d["name"] != "Trip" || d["is_template"] != true || d["rows"] != 2.0 {
		t.Fatalf("PATCH the flag: %v", d)
	}
	d = a.obj(http.StatusOK, "PATCH", path, jmap{"name": " Road Trip "})
	if d["name"] != "Road Trip" || d["is_template"] != true {
		t.Fatalf("PATCH the name: %v", d)
	}
	d = a.obj(http.StatusOK, "PATCH", path, jmap{})
	if d["name"] != "Road Trip" || d["is_template"] != true || len(d["contents"].([]any)) != 2 {
		t.Fatalf("PATCH nothing: %v", d)
	}
	d = a.obj(http.StatusOK, "PATCH", path, jmap{"name": "Trip", "is_template": false})
	if d["name"] != "Trip" || d["is_template"] != false {
		t.Fatalf("PATCH both: %v", d)
	}

	a.fails(http.StatusBadRequest, "PATCH", path, jmap{"name": ""}, "name is required")
	a.fails(http.StatusBadRequest, "PATCH", path, jmap{"name": nil}, "name is required")
	a.fails(http.StatusBadRequest, "PATCH", path, jmap{"is_template": nil}, "is_template must be true or false")
	a.fails(http.StatusBadRequest, "PATCH", path, jmap{"name": strings.Repeat("n", 128)}, "name is too long")
	a.fails(http.StatusBadRequest, "PATCH", path, jmap{"sources": []int{1}}, `unknown field "sources" for a list`)
	a.fails(http.StatusConflict, "PATCH", path, jmap{"name": "Other"}, "there is already a list with that name")
	if d = a.obj(http.StatusOK, "GET", path, nil); d["name"] != "Trip" || d["is_template"] != false {
		t.Fatalf("after the refused PATCHes: %v", d)
	}

	// Deleting a list takes its contents; the library stays.
	a.gone("DELETE", path)
	a.fails(http.StatusNotFound, "GET", path, nil, "not found")
	a.fails(http.StatusNotFound, "DELETE", path, nil, "not found")
	a.fails(http.StatusNotFound, "PATCH", path, jmap{"name": "Back"}, "not found")
	a.fails(http.StatusNotFound, "PATCH", "/api/contents/1", jmap{"quantity": 2}, "not found")
	if got := names(a.arr(http.StatusOK, "GET", "/api/lists", nil)); !reflect.DeepEqual(got, []string{"Other"}) {
		t.Fatalf("lists after the delete: %v", got)
	}
	items := a.arr(http.StatusOK, "GET", "/api/ref/items", nil)
	if len(items) != 6 {
		t.Fatalf("%d items in the library after the delete, want 6", len(items))
	}
	for _, it := range items {
		if it.(jmap)["list_count"] != 0.0 {
			t.Fatalf("an item is still on a list: %v", it)
		}
	}
}

func TestListsAPISetAllStatuses(t *testing.T) {
	a := newListAPI(t)
	trip, other := a.list("Trip"), a.list("Other")
	a.add(trip, a.bag, 0, nil)
	a.add(trip, a.socks, a.bag, jmap{"status_id": packed})
	a.add(trip, a.socks, 0, jmap{"status_id": missing})
	a.add(other, a.comb, 0, nil)
	path := fmt.Sprintf("/api/lists/%v/statuses", trip)

	d := a.obj(http.StatusOK, "POST", path, jmap{"status_id": packed})
	if d["id"] != trip || d["rows"] != 3.0 || !reflect.DeepEqual(d["status_counts"], jmap{"2": 3.0}) {
		t.Fatalf("POST statuses: %v", d)
	}
	for _, r := range d["contents"].([]any) {
		if r.(jmap)["status_id"] != packed {
			t.Fatalf("a row kept its status: %v", r)
		}
	}
	// The other list is not touched.
	if d := a.obj(http.StatusOK, "GET", fmt.Sprintf("/api/lists/%v", other), nil); !reflect.DeepEqual(d["status_counts"], jmap{"1": 1.0}) {
		t.Fatalf("the other list: %v", d)
	}

	a.fails(http.StatusBadRequest, "POST", path, jmap{"status_id": 99}, "status_id: there is no status 99")
	a.fails(http.StatusBadRequest, "POST", path, jmap{"status_id": 40000}, "status_id: there is no status 40000")
	a.fails(http.StatusBadRequest, "POST", path, jmap{}, "status_id is required")
	a.fails(http.StatusBadRequest, "POST", path, jmap{"status_id": nil}, "status_id is required")
	a.fails(http.StatusNotFound, "POST", "/api/lists/999/statuses", jmap{"status_id": packed}, "not found")
	if d := a.obj(http.StatusOK, "GET", fmt.Sprintf("/api/lists/%v", trip), nil); !reflect.DeepEqual(d["status_counts"], jmap{"2": 3.0}) {
		t.Fatalf("after the refused requests: %v", d)
	}
}

// ---------------------------------------------------------------- contents

func TestContentsAPIAdd(t *testing.T) {
	a := newListAPI(t)
	trip, other := a.list("Trip"), a.list("Other")
	a.add(trip, a.car, 0, nil)
	a.add(trip, a.bag, a.car, nil)
	a.add(trip, a.socks, a.bag, nil)
	a.add(other, a.kit, 0, nil)
	path := fmt.Sprintf("/api/lists/%v/contents", trip)

	cases := []struct {
		status  int
		body    jmap
		mention string
	}{
		{400, jmap{}, "item_id is required"},
		{400, jmap{"item_id": nil}, "item_id is required"},
		{400, jmap{"item_id": 999}, "item_id: there is no item 999"},
		{400, jmap{"item_id": 3000000000}, "item_id: there is no item 3000000000"},
		{400, jmap{"item_id": a.comb, "quantity": 0}, "quantity must be a whole number from 1 up"},
		{400, jmap{"item_id": a.comb, "quantity": 2.5}, "quantity must be a whole number"},
		{400, jmap{"item_id": a.comb, "status_id": 99}, "status_id: there is no status 99"},
		{400, jmap{"item_id": a.comb, "unit": strings.Repeat("u", 128)}, "unit is too long (at most 127 characters)"},
		{400, jmap{"item_id": a.comb, "notes": strings.Repeat("n", 256)}, "notes is too long (at most 255 characters)"},
		// Rule 1: into a container, on this list.
		{400, jmap{"item_id": a.comb, "container_id": a.socks}, `"socks" is not a container`},
		{400, jmap{"item_id": a.comb, "container_id": a.kit}, `"kit" is not on this list`},
		{400, jmap{"item_id": a.comb, "container_id": 999}, "container_id: there is no item 999"},
		{400, jmap{"item_id": a.kit, "container_id": a.kit}, `"kit" cannot be put inside itself`},
		// Rule 2: a container once.
		{409, jmap{"item_id": a.bag}, `"bag" is a container and is already on this list`},
		{409, jmap{"item_id": a.car, "container_id": a.bag}, `"car" is a container and is already on this list`},
		// The same placement twice.
		{409, jmap{"item_id": a.socks, "container_id": a.bag, "quantity": 9}, `"socks" is already in "bag" on this list`},
	}
	for _, c := range cases {
		a.fails(c.status, "POST", path, c.body, c.mention)
	}
	a.wantPlaces(trip, "car", "bag in car", "socks in bag")

	// Elsewhere the socks are welcome; at the top level only once.
	a.add(trip, a.socks, a.car, nil)
	a.add(trip, a.socks, 0, nil)
	a.fails(http.StatusConflict, "POST", path, jmap{"item_id": a.socks}, `"socks" is already at the top level of this list`)
	a.fails(http.StatusConflict, "POST", path, jmap{"item_id": a.socks, "container_id": nil}, `"socks" is already at the top level of this list`)
	a.wantPlaces(trip, "car", "bag in car", "socks in bag", "socks in car", "socks")
	a.wantPlaces(other, "kit")
}

func TestContentsAPIPatch(t *testing.T) {
	a := newListAPI(t)
	trip := a.list("Trip")
	a.add(trip, a.car, 0, nil)
	bag := a.add(trip, a.bag, a.car, nil)
	socks := a.add(trip, a.socks, a.bag, jmap{"quantity": 2, "unit": "pairs", "status_id": packed, "notes": "wool"})
	kit := a.add(trip, a.kit, a.bag, nil)
	a.add(trip, a.socks, a.car, nil)
	path := fmt.Sprintf("/api/contents/%v", socks)
	patch := func(what string, body jmap, want jmap) {
		t.Helper()
		if got := a.obj(http.StatusOK, "PATCH", path, body); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s:\n got  %v\n want %v", what, got, want)
		}
	}

	// Partial: a key that is absent is left alone.
	want := jmap{"id": socks, "list_id": trip, "container_id": a.bag, "compartment_id": nil, "item_id": a.socks, "quantity": 2.0, "unit": "pairs", "status_id": packed, "notes": "wool"}
	patch("nothing", jmap{}, want)
	want["quantity"] = 7.0
	patch("quantity", jmap{"quantity": 7}, want)
	want["status_id"] = missing
	patch("status", jmap{"status_id": missing}, want)
	want["notes"] = "thick"
	patch("notes", jmap{"notes": " thick "}, want)
	want["unit"] = "kg"
	patch("unit", jmap{"unit": "kg"}, want)

	// An explicit null clears unit and notes (as does an empty string)...
	want["unit"] = nil
	patch("unit to null", jmap{"unit": nil}, want)
	want["notes"] = nil
	patch("notes to null", jmap{"notes": nil}, want)
	want["unit"], want["notes"] = "g", "n"
	patch("set again", jmap{"unit": "g", "notes": "n"}, want)
	want["unit"], want["notes"] = nil, nil
	patch("to empty strings", jmap{"unit": "", "notes": ""}, want)

	// ...and for container_id means the top level; a number moves the row there.
	want["container_id"] = nil
	patch("to the top level", jmap{"container_id": nil}, want)
	a.wantPlaces(trip, "car", "bag in car", "socks", "kit in bag", "socks in car")
	want["container_id"] = a.kit
	patch("into the kit", jmap{"container_id": a.kit}, want)
	a.wantPlaces(trip, "car", "bag in car", "socks in kit", "kit in bag", "socks in car")
	// A container takes its contents along.
	moved := a.obj(http.StatusOK, "PATCH", fmt.Sprintf("/api/contents/%v", kit), jmap{"container_id": nil})
	if moved["container_id"] != nil || moved["id"] != kit {
		t.Fatalf("kit to the top level: %v", moved)
	}
	a.wantPlaces(trip, "car", "bag in car", "socks in kit", "kit", "socks in car")

	cases := []struct {
		status  int
		path    string
		body    jmap
		mention string
	}{
		{400, path, jmap{"quantity": 0}, "quantity must be a whole number from 1 up"},
		{400, path, jmap{"quantity": nil}, "quantity must be a whole number from 1 up"},
		{400, path, jmap{"status_id": nil}, "status_id is required"},
		{400, path, jmap{"status_id": 99}, "status_id: there is no status 99"},
		{400, path, jmap{"unit": strings.Repeat("u", 128)}, "unit is too long"},
		{400, path, jmap{"notes": strings.Repeat("n", 256)}, "notes is too long"},
		{400, path, jmap{"item_id": a.comb}, `unknown field "item_id" for a list entry`},
		// Rule 1.
		{400, path, jmap{"container_id": a.towel}, `"towel" is not a container`},
		{400, path, jmap{"container_id": 999}, "container_id: there is no item 999"},
		// Rule 3: not into itself or what is inside it.
		{400, fmt.Sprintf("/api/contents/%v", bag), jmap{"container_id": a.bag}, `"bag" cannot be put inside itself`},
		{400, "/api/contents/1", jmap{"container_id": a.bag}, `"car" cannot be moved into "bag", which is inside it`},
		// A placement that is taken.
		{409, path, jmap{"container_id": a.car}, `"socks" is already in "car" on this list`},
		// One bad field refuses the whole change.
		{400, path, jmap{"container_id": a.bag, "quantity": 3, "status_id": 99}, "there is no status 99"},
	}
	for _, c := range cases {
		a.fails(c.status, "PATCH", c.path, c.body, c.mention)
	}
	a.wantPlaces(trip, "car", "bag in car", "socks in kit", "kit", "socks in car")
	patch("after the refused requests", jmap{}, want)
}

func TestContentsAPIDelete(t *testing.T) {
	a := newListAPI(t)
	build := func(name string) (list float64, bag float64) {
		t.Helper()
		list = a.list(name)
		a.add(list, a.car, 0, nil)
		bag = a.add(list, a.bag, a.car, nil)
		a.add(list, a.kit, a.bag, nil)
		a.add(list, a.comb, a.kit, nil)
		a.add(list, a.socks, a.bag, nil)
		a.add(list, a.towel, a.car, nil)
		return list, bag
	}
	all := []string{"car", "bag in car", "kit in bag", "comb in kit", "socks in bag", "towel in car"}

	// Without the parameter, and with contents=remove, what is inside goes too.
	for _, query := range []string{"", "?contents=remove", "?contents="} {
		list, bag := build("Remove " + query)
		a.gone("DELETE", fmt.Sprintf("/api/contents/%v%s", bag, query))
		a.wantPlaces(list, "car", "towel in car")
		a.fails(http.StatusNotFound, "DELETE", fmt.Sprintf("/api/contents/%v%s", bag, query), nil, "not found")
	}

	// contents=keep moves what was directly inside to where the container was.
	list, bag := build("Keep")
	a.gone("DELETE", fmt.Sprintf("/api/contents/%v?contents=keep", bag))
	a.wantPlaces(list, "car", "kit in car", "comb in kit", "socks in car", "towel in car")
	a.fails(http.StatusNotFound, "DELETE", fmt.Sprintf("/api/contents/%v?contents=keep", bag), nil, "not found")

	// Any other value is refused, and nothing is deleted.
	list, bag = build("Invalid")
	for _, query := range []string{"?contents=bogus", "?contents=KEEP", "?contents=true", "?contents=1", "?contents=move", "?contents=keep%20"} {
		a.fails(http.StatusBadRequest, "DELETE", fmt.Sprintf("/api/contents/%v%s", bag, query), nil, `contents must be "remove" or "keep"`)
	}
	a.wantPlaces(list, all...)
	// Other parameters are ignored.
	a.gone("DELETE", fmt.Sprintf("/api/contents/%v?keep=1&other=x", bag))
	a.wantPlaces(list, "car", "towel in car")

	// Keeping is refused when something inside is already at the destination.
	list, bag = build("Clash")
	a.add(list, a.socks, a.car, nil)
	a.fails(http.StatusConflict, "DELETE", fmt.Sprintf("/api/contents/%v?contents=keep", bag), nil,
		`the contents cannot be kept: "socks" is already in the place they would move to`)
	a.wantPlaces(list, append(all[:len(all):len(all)], "socks in car")...)
	a.gone("DELETE", fmt.Sprintf("/api/contents/%v", bag))
	a.wantPlaces(list, "car", "towel in car", "socks in car")
}
