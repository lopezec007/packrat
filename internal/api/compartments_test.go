package api_test

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

// Compartments over HTTP: the fifth reference table, and the compartment_id
// of a list entry. The fixture (lists_test.go) has the bag's lid and pocket
// and the car's boot; the kit has none.

// compartments lists GET /api/ref/compartments as "item/name" strings.
func (a *listAPI) compartments() []string {
	a.t.Helper()
	out := []string{}
	for _, r := range a.arr(http.StatusOK, "GET", "/api/ref/compartments", nil) {
		row := r.(jmap)
		out = append(out, a.itemName[row["item_id"].(float64)]+"/"+row["name"].(string))
	}
	return out
}

// useCounts maps each compartment's name to its use_count.
func (a *listAPI) useCounts() map[string]float64 {
	a.t.Helper()
	out := map[string]float64{}
	for _, r := range a.arr(http.StatusOK, "GET", "/api/ref/compartments", nil) {
		out[r.(jmap)["name"].(string)] = r.(jmap)["use_count"].(float64)
	}
	return out
}

func TestCompartmentsAPI(t *testing.T) {
	a := newListAPI(t)

	// Listed container by container, in the order they were made; the same
	// rows under "compartments" in the combined listing.
	want := []any{
		jmap{"id": a.boot, "item_id": a.car, "name": "boot", "use_count": 0.0},
		jmap{"id": a.lid, "item_id": a.bag, "name": "lid", "use_count": 0.0},
		jmap{"id": a.pocket, "item_id": a.bag, "name": "pocket", "use_count": 0.0},
	}
	if got := a.arr(http.StatusOK, "GET", "/api/ref/compartments", nil); !reflect.DeepEqual(got, want) {
		t.Fatalf("GET /api/ref/compartments:\n got  %v\n want %v", got, want)
	}
	all := a.obj(http.StatusOK, "GET", "/api/ref", nil)
	if !reflect.DeepEqual(all["compartments"], any(want)) || len(all) != 5 {
		t.Fatalf("GET /api/ref: compartments %v among %v", all["compartments"], jsonKeys(all))
	}

	row := a.obj(http.StatusCreated, "POST", "/api/ref/compartments", jmap{"item_id": a.kit, "name": "  Mesh "})
	mesh := jmap{"id": 4.0, "item_id": a.kit, "name": "Mesh", "use_count": 0.0}
	if !reflect.DeepEqual(row, mesh) {
		t.Fatalf("POST: %v, want %v", row, mesh)
	}
	// PATCH is partial; the item may be sent back but not changed.
	mesh["name"] = "Net"
	for _, body := range []jmap{{"name": "Net"}, {}, {"item_id": a.kit}, {"id": 4, "item_id": a.kit, "name": " Net ", "use_count": 7}} {
		if got := a.obj(http.StatusOK, "PATCH", "/api/ref/compartments/4", body); !reflect.DeepEqual(got, mesh) {
			t.Fatalf("PATCH %v: %v, want %v", body, got, mesh)
		}
	}
	// The same name on another container is fine.
	a.obj(http.StatusCreated, "POST", "/api/ref/compartments", jmap{"item_id": a.kit, "name": "lid"})
	a.obj(http.StatusCreated, "POST", "/api/ref/compartments", jmap{"item_id": a.car, "name": "lid"})
	before := []string{"car/boot", "car/lid", "bag/lid", "bag/pocket", "kit/Net", "kit/lid"}
	if got := a.compartments(); !reflect.DeepEqual(got, before) {
		t.Fatalf("compartments %v, want %v", got, before)
	}

	lid, pocket := fmt.Sprintf("/api/ref/compartments/%v", a.lid), fmt.Sprintf("/api/ref/compartments/%v", a.pocket)
	cases := []struct {
		status       int
		method, path string
		body         jmap
		mention      string
	}{
		{400, "POST", "/api/ref/compartments", jmap{}, "item_id is required"},
		{400, "POST", "/api/ref/compartments", jmap{"name": "side"}, "item_id is required"},
		{400, "POST", "/api/ref/compartments", jmap{"item_id": a.bag}, "name is required"},
		{400, "POST", "/api/ref/compartments", jmap{"item_id": a.bag, "name": " "}, "name is required"},
		{400, "POST", "/api/ref/compartments", jmap{"item_id": a.bag, "name": strings.Repeat("n", 128)}, "name is too long (at most 127 characters)"},
		{400, "POST", "/api/ref/compartments", jmap{"item_id": "2", "name": "side"}, "item_id must be a whole number"},
		{400, "POST", "/api/ref/compartments", jmap{"item_id": 0, "name": "side"}, "item_id is out of range"},
		{400, "POST", "/api/ref/compartments", jmap{"item_id": a.bag, "name": "side", "list_id": 1}, `unknown field "list_id" for compartments`},
		// Only an item that is there, and is a container, has compartments.
		{400, "POST", "/api/ref/compartments", jmap{"item_id": 999, "name": "side"}, "item_id: there is no item 999"},
		{400, "POST", "/api/ref/compartments", jmap{"item_id": a.socks, "name": "side"}, `"socks" is not a container: only a container has compartments`},
		{400, "PATCH", lid, jmap{"name": ""}, "name is required"},
		{400, "PATCH", lid, jmap{"item_id": a.car}, "item_id cannot be changed once the compartment exists"},
		{400, "PATCH", lid, jmap{"item_id": a.car, "name": "moved"}, "item_id cannot be changed once the compartment exists"},
		// A container's compartments have different names.
		{409, "POST", "/api/ref/compartments", jmap{"item_id": a.bag, "name": "lid"}, "that container already has a compartment with that name"},
		{409, "POST", "/api/ref/compartments", jmap{"item_id": a.bag, "name": " pocket "}, "that container already has a compartment with that name"},
		{409, "PATCH", pocket, jmap{"name": "lid"}, "that container already has a compartment with that name"},
		{404, "PATCH", "/api/ref/compartments/999", jmap{"name": "side"}, "not found"},
		{404, "PATCH", "/api/ref/compartments/999", jmap{"item_id": a.bag}, "not found"},
		{404, "DELETE", "/api/ref/compartments/999", nil, "not found"},
	}
	for _, c := range cases {
		var body any
		if c.body != nil {
			body = c.body
		}
		a.fails(c.status, c.method, c.path, body, c.mention)
	}
	if got := a.compartments(); !reflect.DeepEqual(got, before) {
		t.Fatalf("compartments after the refused requests %v, want %v", got, before)
	}

	a.gone("DELETE", "/api/ref/compartments/4")
	a.fails(http.StatusNotFound, "DELETE", "/api/ref/compartments/4", nil, "not found")
	a.fails(http.StatusNotFound, "PATCH", "/api/ref/compartments/4", jmap{"name": "Net"}, "not found")
	if got, want := a.compartments(), []string{"car/boot", "car/lid", "bag/lid", "bag/pocket", "kit/lid"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("compartments after the delete %v, want %v", got, want)
	}
}

// A compartment in use can be deleted: what was in it stays in the
// container. An item's compartments go with the item, and stay when it
// stops being a container.
func TestCompartmentsAPIInUse(t *testing.T) {
	a := newListAPI(t)
	trip := a.list("Trip")
	a.add(trip, a.car, 0, nil)
	a.add(trip, a.bag, a.car, jmap{"compartment_id": a.boot})
	a.add(trip, a.socks, a.bag, jmap{"compartment_id": a.lid, "quantity": 2})
	a.add(trip, a.comb, a.bag, jmap{"compartment_id": a.lid})
	a.add(trip, a.kit, a.bag, jmap{"compartment_id": a.pocket})
	a.add(trip, a.towel, a.bag, nil)
	a.wantPlaces(trip, "car", "bag in car/boot", "socks in bag/lid", "comb in bag/lid", "kit in bag/pocket", "towel in bag")
	if got, want := a.useCounts(), (map[string]float64{"boot": 1, "lid": 2, "pocket": 1}); !reflect.DeepEqual(got, want) {
		t.Fatalf("use counts %v, want %v", got, want)
	}

	a.gone("DELETE", fmt.Sprintf("/api/ref/compartments/%v", a.lid))
	a.wantPlaces(trip, "car", "bag in car/boot", "socks in bag", "comb in bag", "kit in bag/pocket", "towel in bag")
	socks := a.obj(http.StatusOK, "GET", fmt.Sprintf("/api/lists/%v", trip), nil)["contents"].([]any)[2].(jmap)
	if socks["compartment_id"] != nil || socks["container_id"] != a.bag || socks["quantity"] != 2.0 {
		t.Fatalf("the socks after their compartment was deleted: %v", socks)
	}
	if got, want := a.useCounts(), (map[string]float64{"boot": 1, "pocket": 1}); !reflect.DeepEqual(got, want) {
		t.Fatalf("use counts after the delete %v, want %v", got, want)
	}
	a.fails(http.StatusBadRequest, "PATCH", fmt.Sprintf("/api/contents/%v", socks["id"]), jmap{"compartment_id": a.lid},
		fmt.Sprintf("compartment_id: there is no compartment %v", a.lid))

	// The bag is on a list (and holds things): neither it nor its flag can go.
	bag, car := fmt.Sprintf("/api/ref/items/%v", a.bag), fmt.Sprintf("/api/ref/items/%v", a.car)
	a.fails(http.StatusConflict, "DELETE", bag, nil, "take it off every list before deleting it")
	a.fails(http.StatusConflict, "PATCH", bag, jmap{"is_container": false}, `this item holds other items on the list "Trip"`)
	if got, want := a.compartments(), []string{"car/boot", "bag/pocket"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("compartments %v, want %v", got, want)
	}

	// Off every list, the car can stop being a container; its boot stays,
	// but it gets no new compartments.
	a.gone("DELETE", fmt.Sprintf("/api/lists/%v", trip))
	if got := a.obj(http.StatusOK, "PATCH", car, jmap{"is_container": false}); got["is_container"] != false {
		t.Fatalf("PATCH the car: %v", got)
	}
	if got, want := a.compartments(), []string{"car/boot", "bag/pocket"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("compartments once the car is no container %v, want %v", got, want)
	}
	a.fails(http.StatusBadRequest, "POST", "/api/ref/compartments", jmap{"item_id": a.car, "name": "glovebox"}, `"car" is not a container`)
	// Deleting an item deletes its compartments.
	a.gone("DELETE", bag)
	a.gone("DELETE", car)
	if status, _, raw := a.call("GET", "/api/ref/compartments", nil); status != http.StatusOK || raw != `[]` {
		t.Fatalf("compartments once their items are gone: %d %s", status, raw)
	}
}

func TestContentsAPICompartments(t *testing.T) {
	a := newListAPI(t)
	trip := a.list("Trip")
	a.add(trip, a.car, 0, nil)
	bag := a.add(trip, a.bag, a.car, jmap{"compartment_id": a.boot})
	contents := fmt.Sprintf("/api/lists/%v/contents", trip)

	// compartment_id sits between container_id and item_id in the row.
	rec := a.do("POST", contents, jmap{"item_id": a.socks, "container_id": a.bag, "compartment_id": a.lid})
	wantRaw := fmt.Sprintf(`{"id":3,"list_id":%v,"container_id":%v,"compartment_id":%v,"item_id":%v,"quantity":1,"unit":null,"status_id":1,"notes":null}`,
		trip, a.bag, a.lid, a.socks)
	if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusCreated || got != wantRaw {
		t.Fatalf("POST a row in a compartment: %d\n got  %s\n want %s", rec.Code, got, wantRaw)
	}
	socks := 3.0
	path := fmt.Sprintf("/api/contents/%v", socks)

	const atTop = "an entry at the top level of a list is in no container, so it cannot be in a compartment"
	for _, c := range []struct {
		status  int
		body    jmap
		mention string
	}{
		{400, jmap{"item_id": a.comb, "compartment_id": a.lid}, atTop},
		{400, jmap{"item_id": a.comb, "container_id": nil, "compartment_id": a.lid}, atTop},
		{400, jmap{"item_id": a.comb, "container_id": a.bag, "compartment_id": 999}, "compartment_id: there is no compartment 999"},
		{400, jmap{"item_id": a.comb, "container_id": a.bag, "compartment_id": 3000000000}, "compartment_id: there is no compartment 3000000000"},
		{400, jmap{"item_id": a.comb, "container_id": a.bag, "compartment_id": a.boot}, `that compartment is not one of "bag"'s`},
		{400, jmap{"item_id": a.comb, "container_id": a.car, "compartment_id": a.lid}, `that compartment is not one of "car"'s`},
		{400, jmap{"item_id": a.comb, "container_id": a.bag, "compartment_id": "lid"}, "compartment_id must be a whole number"},
		{400, jmap{"item_id": a.comb, "container_id": a.bag, "compartment": a.lid}, `unknown field "compartment"`},
		// Once in a container, whichever compartment.
		{409, jmap{"item_id": a.socks, "container_id": a.bag, "compartment_id": a.pocket}, `"socks" is already in "bag" on this list`},
		{409, jmap{"item_id": a.socks, "container_id": a.bag}, `"socks" is already in "bag" on this list`},
	} {
		a.fails(c.status, "POST", contents, c.body, c.mention)
	}
	a.wantPlaces(trip, "car", "bag in car/boot", "socks in bag/lid")

	want := jmap{"id": socks, "list_id": trip, "container_id": a.bag, "compartment_id": a.lid, "item_id": a.socks,
		"quantity": 1.0, "unit": nil, "status_id": stored, "notes": nil}
	patch := func(what string, body jmap, container, compartment any) {
		t.Helper()
		want["container_id"], want["compartment_id"] = container, compartment
		if got := a.obj(http.StatusOK, "PATCH", path, body); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s:\n got  %v\n want %v", what, got, want)
		}
	}
	// An absent key is left alone; a compartment alone changes within the
	// container; null clears it.
	want["quantity"] = 2.0
	patch("quantity only", jmap{"quantity": 2}, a.bag, a.lid)
	patch("to the pocket", jmap{"compartment_id": a.pocket}, a.bag, a.pocket)
	patch("to none", jmap{"compartment_id": nil}, a.bag, nil)
	patch("to the lid", jmap{"compartment_id": a.lid}, a.bag, a.lid)
	patch("the container it is in", jmap{"container_id": a.bag}, a.bag, a.lid)
	// A move clears it, unless a compartment of the new container comes along.
	patch("into the car", jmap{"container_id": a.car}, a.car, nil)
	patch("into the bag's pocket", jmap{"container_id": a.bag, "compartment_id": a.pocket}, a.bag, a.pocket)
	patch("into the car's boot", jmap{"container_id": a.car, "compartment_id": a.boot}, a.car, a.boot)
	patch("to the top level", jmap{"container_id": nil}, nil, nil)
	a.fails(http.StatusBadRequest, "PATCH", path, jmap{"compartment_id": a.lid}, atTop)
	a.fails(http.StatusBadRequest, "PATCH", path, jmap{"container_id": nil, "compartment_id": a.lid}, atTop)
	patch("back into the bag's lid", jmap{"container_id": a.bag, "compartment_id": a.lid}, a.bag, a.lid)

	for _, c := range []struct {
		body    jmap
		mention string
	}{
		{jmap{"compartment_id": a.boot}, `that compartment is not one of "bag"'s`},
		{jmap{"compartment_id": 999}, "compartment_id: there is no compartment 999"},
		{jmap{"compartment_id": "lid"}, "compartment_id must be a whole number"},
		{jmap{"container_id": a.car, "compartment_id": a.lid}, `that compartment is not one of "car"'s`},
		{jmap{"container_id": a.car, "compartment_id": a.boot, "quantity": 0}, "quantity must be"},
	} {
		a.fails(http.StatusBadRequest, "PATCH", path, c.body, c.mention)
	}
	patch("after the refused requests", jmap{}, a.bag, a.lid)

	// The same item cannot be moved into a second compartment of the bag.
	loose := a.add(trip, a.socks, a.car, jmap{"compartment_id": a.boot})
	a.fails(http.StatusConflict, "PATCH", fmt.Sprintf("/api/contents/%v", loose), jmap{"container_id": a.bag, "compartment_id": a.pocket},
		`"socks" is already in "bag" on this list`)
	a.gone("DELETE", fmt.Sprintf("/api/contents/%v", loose))

	// contents=keep moves what was in the bag up into the car, out of the
	// bag's compartments; what is deeper keeps its place.
	a.add(trip, a.kit, a.bag, jmap{"compartment_id": a.pocket})
	a.add(trip, a.comb, a.kit, nil)
	a.wantPlaces(trip, "car", "bag in car/boot", "socks in bag/lid", "kit in bag/pocket", "comb in kit")
	a.gone("DELETE", fmt.Sprintf("/api/contents/%v?contents=keep", bag))
	a.wantPlaces(trip, "car", "socks in car", "kit in car", "comb in kit")
	patch("after the bag went", jmap{}, a.car, nil)
}

func TestListsAPIFromSourcesWithCompartments(t *testing.T) {
	a := newListAPI(t)
	camp := a.list("Camping")
	a.add(camp, a.car, 0, nil)
	a.add(camp, a.bag, a.car, jmap{"compartment_id": a.boot})
	a.add(camp, a.socks, a.bag, jmap{"compartment_id": a.lid})
	a.add(camp, a.towel, a.bag, nil)
	city := a.list("City Break")
	a.add(city, a.bag, 0, nil)
	a.add(city, a.socks, a.bag, jmap{"compartment_id": a.pocket, "quantity": 4})
	a.add(city, a.towel, a.bag, jmap{"compartment_id": a.lid})
	a.add(city, a.comb, a.bag, jmap{"compartment_id": a.pocket})

	for _, c := range []struct {
		name    string
		sources []float64
		want    []string
	}{
		{"Copy", []float64{camp}, []string{"car", "bag in car/boot", "socks in bag/lid", "towel in bag"}},
		// A placement on both keeps the first list's compartment (or its
		// lack of one); the bag stays in the boot and the comb follows it.
		{"Both", []float64{camp, city}, []string{"car", "bag in car/boot", "socks in bag/lid", "towel in bag", "comb in bag/pocket"}},
		{"Reversed", []float64{city, camp}, []string{"bag", "socks in bag/pocket", "towel in bag/lid", "comb in bag/pocket", "car"}},
	} {
		// The preview counts rows as before.
		if p := a.obj(http.StatusOK, "POST", "/api/lists/preview", jmap{"sources": c.sources}); !reflect.DeepEqual(p, jmap{"rows": float64(len(c.want))}) {
			t.Fatalf("%s: preview %v, want %d rows", c.name, p, len(c.want))
		}
		d := a.obj(http.StatusCreated, "POST", "/api/lists", jmap{"name": c.name, "sources": c.sources})
		a.wantPlaces(d["id"].(float64), c.want...)
		for _, r := range d["contents"].([]any) {
			if row := r.(jmap); row["item_id"] == a.socks && len(c.sources) == 2 && row["quantity"] != 4.0 {
				t.Fatalf("%s: the socks should be the larger pile: %v", c.name, row)
			}
		}
	}
	if got, want := a.useCounts(), (map[string]float64{"boot": 3, "lid": 5, "pocket": 5}); !reflect.DeepEqual(got, want) {
		t.Fatalf("use counts %v, want %v", got, want)
	}
}
