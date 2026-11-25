package api_test

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"
)

// The statuses' rank over HTTP: a read-only "rank" in the rows of
// /api/ref/statuses (1 = the worst), set by POST /api/statuses/order.

// statusRanks maps each status's name to its rank.
func (e *env) statusRanks() map[string]float64 {
	e.t.Helper()
	out := map[string]float64{}
	for _, r := range e.arr(http.StatusOK, "GET", "/api/ref/statuses", nil) {
		out[r.(jmap)["name"].(string)] = r.(jmap)["rank"].(float64)
	}
	return out
}

func TestStatusRankInRefAPI(t *testing.T) {
	e := newEnv(t)
	seeded := map[string]float64{"Missing": 1, "In Storage Location": 2, "Packed": 3}
	if got := e.statusRanks(); !reflect.DeepEqual(got, seeded) {
		t.Fatalf("seeded ranks %v, want %v", got, seeded)
	}

	// A new status is ranked after the others; a rank sent with it is ignored.
	bought := e.obj(http.StatusCreated, "POST", "/api/ref/statuses", jmap{"name": "Bought", "color": "#112233", "shortcut": "b"})
	want := jmap{"id": 4.0, "name": "Bought", "color": "#112233", "shortcut": "b", "rank": 4.0, "use_count": 0.0}
	if !reflect.DeepEqual(bought, want) {
		t.Fatalf("POST: %v, want %v", bought, want)
	}
	clean := e.obj(http.StatusCreated, "POST", "/api/ref/statuses", jmap{"name": "Clean", "color": "#112233", "shortcut": "c", "rank": 1})
	if clean["rank"] != 5.0 {
		t.Fatalf("POST with a rank: %v, want rank 5", clean)
	}
	// PATCH ignores it as well: alone, in a row sent back whole, or with a real change.
	for _, body := range []jmap{
		{"rank": 1},
		{"rank": nil},
		{"rank": "worst"},
		{"id": 4, "name": "Bought", "color": "#112233", "shortcut": "b", "rank": 2, "use_count": 0},
	} {
		if got := e.obj(http.StatusOK, "PATCH", "/api/ref/statuses/4", body); !reflect.DeepEqual(got, want) {
			t.Fatalf("PATCH %v: %v, want %v", body, got, want)
		}
	}
	want["name"] = "Borrowed"
	if got := e.obj(http.StatusOK, "PATCH", "/api/ref/statuses/4", jmap{"name": "Borrowed", "rank": 1}); !reflect.DeepEqual(got, want) {
		t.Fatalf("PATCH a name with a rank: %v, want %v", got, want)
	}
	all := map[string]float64{"Missing": 1, "In Storage Location": 2, "Packed": 3, "Borrowed": 4, "Clean": 5}
	if got := e.statusRanks(); !reflect.DeepEqual(got, all) {
		t.Fatalf("ranks %v, want %v", got, all)
	}

	// Deleting a status is what it was: the others keep their ranks.
	e.gone("DELETE", "/api/ref/statuses/4")
	delete(all, "Borrowed")
	if got := e.statusRanks(); !reflect.DeepEqual(got, all) {
		t.Fatalf("ranks after a delete %v, want %v", got, all)
	}
}

func TestStatusesOrderAPI(t *testing.T) {
	e := newEnv(t)
	e.obj(http.StatusCreated, "POST", "/api/ref/statuses", jmap{"name": "Bought", "color": "#112233", "shortcut": "b"}) // 4
	e.obj(http.StatusCreated, "POST", "/api/ref/statuses", jmap{"name": "Clean", "color": "#112233", "shortcut": "c"})  // 5

	// A full reversal. The answer is the statuses as GET lists them: by id,
	// each row whole, with its new rank.
	rows := e.arr(http.StatusOK, "POST", "/api/statuses/order", jmap{"ids": []int{5, 4, 2, 1, 3}})
	want := []any{
		jmap{"id": 1.0, "name": "In Storage Location", "color": "#8a94a3", "shortcut": "s", "rank": 4.0, "use_count": 0.0},
		jmap{"id": 2.0, "name": "Packed", "color": "#2e8540", "shortcut": "p", "rank": 3.0, "use_count": 0.0},
		jmap{"id": 3.0, "name": "Missing", "color": "#d62828", "shortcut": "m", "rank": 5.0, "use_count": 0.0},
		jmap{"id": 4.0, "name": "Bought", "color": "#112233", "shortcut": "b", "rank": 2.0, "use_count": 0.0},
		jmap{"id": 5.0, "name": "Clean", "color": "#112233", "shortcut": "c", "rank": 1.0, "use_count": 0.0},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("POST /api/statuses/order:\n got  %v\n want %v", rows, want)
	}
	if got := e.arr(http.StatusOK, "GET", "/api/ref/statuses", nil); !reflect.DeepEqual(got, want) {
		t.Fatalf("GET /api/ref/statuses afterwards: %v", got)
	}
	if got := e.obj(http.StatusOK, "GET", "/api/ref", nil)["statuses"]; !reflect.DeepEqual(got, any(want)) {
		t.Fatalf("GET /api/ref afterwards: %v", got)
	}
	// Two neighbours swapped, then the order they are in again.
	for _, c := range []struct {
		ids  []int
		want map[string]float64
	}{
		{[]int{5, 4, 1, 2, 3}, map[string]float64{"Clean": 1, "Bought": 2, "In Storage Location": 3, "Packed": 4, "Missing": 5}},
		{[]int{5, 4, 1, 2, 3}, map[string]float64{"Clean": 1, "Bought": 2, "In Storage Location": 3, "Packed": 4, "Missing": 5}},
		{[]int{3, 1, 2, 4, 5}, map[string]float64{"Missing": 1, "In Storage Location": 2, "Packed": 3, "Bought": 4, "Clean": 5}},
	} {
		if rows := e.arr(http.StatusOK, "POST", "/api/statuses/order", jmap{"ids": c.ids}); len(rows) != 5 {
			t.Fatalf("order %v: %v", c.ids, rows)
		}
		if got := e.statusRanks(); !reflect.DeepEqual(got, c.want) {
			t.Fatalf("order %v: ranks %v, want %v", c.ids, got, c.want)
		}
	}

	// Every status once, or it is a 400 and nothing changes.
	before := e.statusRanks()
	const every = "ids must list every status once, from the worst to the best"
	for _, c := range []struct {
		body    any
		mention string
	}{
		{jmap{"ids": []int{5, 4, 3, 2}}, every + " (4 of 5 given)"},
		{jmap{"ids": []int{}}, every + " (0 of 5 given)"},
		{jmap{"ids": nil}, every + " (0 of 5 given)"},
		{jmap{}, every + " (0 of 5 given)"},
		{jmap{"ids": []int{5, 4, 3, 2, 1, 6}}, "ids: there is no status 6"},
		{jmap{"ids": []int{5, 4, 3, 2, 0}}, "ids: there is no status 0"},
		{jmap{"ids": []int{5, 4, 3, 2, 40000}}, "ids: there is no status 40000"},
		{jmap{"ids": []int{5, 4, 3, 2, 1, 1}}, "ids: status 1 is listed twice"},
		{jmap{"ids": []int{5, 5, 3, 2, 1}}, "ids: status 5 is listed twice"},
		// Not a list of whole numbers, or not the field the route takes.
		{`{"ids": "5,4,3,2,1"}`, "invalid JSON body"},
		{`{"ids": ["5", "4", "3", "2", "1"]}`, "invalid JSON body"},
		{`{"ids": [5, 4, 3, 2, 1.5]}`, "invalid JSON body"},
		{`{"ids": 5}`, "invalid JSON body"},
		{`{"order": [5, 4, 3, 2, 1]}`, `unknown field "order"`},
		{`{"ids": [5, 4, 3, 2, 1], "ranks": [1, 2, 3, 4, 5]}`, `unknown field "ranks"`},
		{`{"ids": [5, 4, 3, 2, 1]} [1]`, "unexpected data after the object"},
		{`[5, 4, 3, 2, 1]`, "invalid JSON body"},
	} {
		e.fails(http.StatusBadRequest, "POST", "/api/statuses/order", c.body, c.mention)
		if got := e.statusRanks(); !reflect.DeepEqual(got, before) {
			t.Fatalf("order %v changed the ranks: %v, want %v", c.body, got, before)
		}
	}

	// A deleted status leaves a gap and is no longer one to order; ordering
	// the rest ranks them 1 to 4.
	e.gone("DELETE", "/api/ref/statuses/4")
	if got, want := e.statusRanks(), (map[string]float64{"Missing": 1, "In Storage Location": 2, "Packed": 3, "Clean": 5}); !reflect.DeepEqual(got, want) {
		t.Fatalf("ranks after a delete %v, want %v", got, want)
	}
	e.fails(http.StatusBadRequest, "POST", "/api/statuses/order", jmap{"ids": []int{3, 1, 2, 4, 5}}, "ids: there is no status 4")
	e.arr(http.StatusOK, "POST", "/api/statuses/order", jmap{"ids": []int{3, 1, 5, 2}})
	if got, want := e.statusRanks(), (map[string]float64{"Missing": 1, "In Storage Location": 2, "Clean": 3, "Packed": 4}); !reflect.DeepEqual(got, want) {
		t.Fatalf("ranks after ordering the rest %v, want %v", got, want)
	}
	// The next new status goes on top of those.
	if row := e.obj(http.StatusCreated, "POST", "/api/ref/statuses", jmap{"name": "Dry", "color": "#112233", "shortcut": "d"}); row["rank"] != 5.0 {
		t.Fatalf("a new status after the ordering: %v, want rank 5", row)
	}
}

// Ranks are about the statuses only: entries keep theirs, and a new entry
// still starts with the first status by id, whatever its rank.
func TestStatusesOrderAPILeavesListsAlone(t *testing.T) {
	a := newListAPI(t)
	trip := a.list("Trip")
	a.add(trip, a.bag, 0, jmap{"status_id": packed})
	a.add(trip, a.socks, a.bag, jmap{"status_id": missing})
	list := fmt.Sprintf("/api/lists/%v", trip)
	before := a.obj(http.StatusOK, "GET", list, nil)

	rows := a.arr(http.StatusOK, "POST", "/api/statuses/order", jmap{"ids": []float64{packed, missing, stored}})
	// The use counts come back with the rows.
	if got := rows[1].(jmap); got["name"] != "Packed" || got["rank"] != 1.0 || got["use_count"] != 1.0 {
		t.Fatalf("the Packed row: %v", got)
	}
	if got := a.obj(http.StatusOK, "GET", list, nil); !reflect.DeepEqual(got, before) {
		t.Fatalf("the list changed:\n got  %v\n want %v", got, before)
	}
	// In Storage Location is now ranked the best, and is still what a new entry gets.
	row := a.obj(http.StatusCreated, "POST", list+"/contents", jmap{"item_id": a.comb})
	if row["status_id"] != stored {
		t.Fatalf("a new entry: %v, want status %v", row, stored)
	}
	// A status in use still cannot be deleted; one that is not, can.
	a.fails(http.StatusConflict, "DELETE", fmt.Sprintf("/api/ref/statuses/%v", packed), nil, "this status is in use on a list")
	a.gone("DELETE", fmt.Sprintf("/api/contents/%v", 2.0))
	a.gone("DELETE", fmt.Sprintf("/api/ref/statuses/%v", missing))
	if got, want := a.statusRanks(), (map[string]float64{"Packed": 1, "In Storage Location": 3}); !reflect.DeepEqual(got, want) {
		t.Fatalf("ranks %v, want %v", got, want)
	}
}

func TestStatusesOrderAPIWithNoStatuses(t *testing.T) {
	e := newEnv(t)
	for id := 1; id <= 3; id++ {
		e.gone("DELETE", fmt.Sprintf("/api/ref/statuses/%d", id))
	}
	// Nothing to order is an order: an empty array, not null.
	for _, body := range []any{jmap{"ids": []int{}}, jmap{}, jmap{"ids": nil}} {
		if status, _, raw := e.call("POST", "/api/statuses/order", body); status != http.StatusOK || raw != `[]` {
			t.Fatalf("order %v of no statuses: %d %s", body, status, raw)
		}
	}
	e.fails(http.StatusBadRequest, "POST", "/api/statuses/order", jmap{"ids": []int{1}}, "ids: there is no status 1")
	// The first status made after that is rank 1.
	if row := e.obj(http.StatusCreated, "POST", "/api/ref/statuses", jmap{"name": "Only", "color": "#112233", "shortcut": "o"}); row["rank"] != 1.0 {
		t.Fatalf("the only status: %v, want rank 1", row)
	}
}
