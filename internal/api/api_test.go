package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"

	"packrat/internal/api"
	"packrat/internal/store"
	"packrat/internal/testdb"
)

// jmap is a JSON object, as sent or as decoded.
type jmap = map[string]any

// env is the API over a fresh test database.
type env struct {
	t  *testing.T
	st *store.Store
	h  http.Handler
}

func newEnv(t *testing.T) *env {
	t.Helper()
	st := testdb.New(t)
	srv := &api.Server{St: st, Info: api.Info{Version: "1.2.3", Date: "2026-11-16", Build: "v1.2.3-4-gabcdef"}}
	return &env{t: t, st: st, h: srv.Handler()}
}

// do makes one request. body may be nil, a string (sent as is) or anything
// json.Marshal takes.
func (e *env) do(method, path string, body any) *httptest.ResponseRecorder {
	e.t.Helper()
	var rd *strings.Reader
	switch b := body.(type) {
	case nil:
		rd = strings.NewReader("")
	case string:
		rd = strings.NewReader(b)
	default:
		js, err := json.Marshal(b)
		if err != nil {
			e.t.Fatal(err)
		}
		rd = strings.NewReader(string(js))
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, httptest.NewRequest(method, path, rd))
	return rec
}

// call makes one request and decodes the response, which is JSON whenever
// there is one (v is nil for an empty body).
func (e *env) call(method, path string, body any) (status int, v any, raw string) {
	e.t.Helper()
	rec := e.do(method, path, body)
	raw = strings.TrimSpace(rec.Body.String())
	if raw != "" {
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			e.t.Fatalf("%s %s: Content-Type %q, want application/json", method, path, ct)
		}
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			e.t.Fatalf("%s %s: response is not JSON: %s", method, path, raw)
		}
	}
	return rec.Code, v, raw
}

// must is call for a request that has to give the status.
func (e *env) must(want int, method, path string, body any) any {
	e.t.Helper()
	status, v, raw := e.call(method, path, body)
	if status != want {
		e.t.Fatalf("%s %s: status %d, want %d: %s", method, path, status, want, raw)
	}
	return v
}

// obj is must for a response that is a JSON object; arr for an array.
func (e *env) obj(want int, method, path string, body any) jmap {
	e.t.Helper()
	v, ok := e.must(want, method, path, body).(jmap)
	if !ok {
		e.t.Fatalf("%s %s: the response is not a JSON object", method, path)
	}
	return v
}

func (e *env) arr(want int, method, path string, body any) []any {
	e.t.Helper()
	v, ok := e.must(want, method, path, body).([]any)
	if !ok {
		e.t.Fatalf("%s %s: the response is not a JSON array", method, path)
	}
	return v
}

// fails is call for a request that has to be refused with the status and an
// {"error": "..."} body mentioning `mention`.
func (e *env) fails(want int, method, path string, body any, mention string) {
	e.t.Helper()
	status, v, raw := e.call(method, path, body)
	obj, _ := v.(jmap)
	msg, _ := obj["error"].(string)
	if status != want || len(obj) != 1 || msg == "" || !strings.Contains(msg, mention) {
		e.t.Errorf("%s %s %v: got %d %s; want %d and an error mentioning %q", method, path, body, status, raw, want, mention)
	}
}

// gone is call for a delete: 204 and no body.
func (e *env) gone(method, path string) {
	e.t.Helper()
	if status, _, raw := e.call(method, path, nil); status != http.StatusNoContent || raw != "" {
		e.t.Fatalf("%s %s: got %d %q; want 204 and no body", method, path, status, raw)
	}
}

func jsonKeys(m jmap) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// names lists the "name" of each object in a JSON array.
func names(rows []any) []string {
	out := []string{}
	for _, r := range rows {
		out = append(out, r.(jmap)["name"].(string))
	}
	return out
}

// ---------------------------------------------------------------- version, unknown routes

func TestVersion(t *testing.T) {
	e := newEnv(t)
	rec := e.do("GET", "/api/version", nil)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/json" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status %d, headers %v", rec.Code, rec.Header())
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"version":"1.2.3","date":"2026-11-16","build":"v1.2.3-4-gabcdef"}` {
		t.Fatalf("body %s", got)
	}
}

func TestUnknownEndpoints(t *testing.T) {
	e := newEnv(t)
	list := e.obj(http.StatusCreated, "POST", "/api/lists", jmap{"name": "Trip"})["id"]

	// Nothing answers these paths, or not that method on them.
	for _, r := range [][2]string{
		{"GET", "/api/"},
		{"GET", "/api/nope"},
		{"GET", "/api/list"},
		{"GET", "/api/lists/"},
		{"GET", "/api/lists/1/"},
		{"GET", "/api/lists/1/contents"},
		{"GET", "/api/lists/1/statuses"},
		{"GET", "/api/lists/1/contents/1"},
		{"GET", "/api/contents"},
		{"GET", "/api/contents/1"},
		{"POST", "/api/contents"},
		{"POST", "/api/contents/1"},
		{"PUT", "/api/contents/1"},
		{"PUT", "/api/lists"},
		{"PUT", "/api/lists/1"},
		{"DELETE", "/api/lists"},
		{"PATCH", "/api/lists"},
		{"GET", "/api/lists/preview/1"},
		{"POST", "/api/version"},
		{"POST", "/api/ref"},
		{"DELETE", "/api/ref"},
		{"DELETE", "/api/ref/items"},
		{"PUT", "/api/ref/items/1"},
		{"GET", "/api/ref/items/1"},
		{"GET", "/api/ref/items/1/lists"},
		{"GET", "/api/v1/lists"},
		{"GET", "/api/statuses"},
		{"GET", "/api/statuses/order"},
		{"PUT", "/api/statuses/order"},
		{"POST", "/api/statuses/order/1"},
		{"POST", "/api/ref/statuses/order"},
	} {
		e.fails(http.StatusNotFound, r[0], r[1], nil, "no such API endpoint: "+r[0]+" "+r[1])
	}
	// The error is JSON like any other.
	rec := e.do("GET", "/api/nope", nil)
	if rec.Header().Get("Content-Type") != "application/json" || strings.TrimSpace(rec.Body.String()) != `{"error":"no such API endpoint: GET /api/nope"}` {
		t.Fatalf("unknown endpoint: %v %s", rec.Header(), rec.Body.String())
	}
	// The list is still there: none of that reached it.
	e.obj(http.StatusOK, "GET", fmt.Sprintf("/api/lists/%v", list), nil)
}

func TestBadIDs(t *testing.T) {
	e := newEnv(t)
	e.obj(http.StatusCreated, "POST", "/api/lists", jmap{"name": "Trip"})
	e.obj(http.StatusCreated, "POST", "/api/ref/item_categories", jmap{"name": "Gear"})

	// IDs that are not positive whole numbers, that no row has, and that no
	// row could have (past smallint for lists and settings, past int for
	// items and list entries; past anything).
	bad := []string{"abc", "0", "-1", "1.5", "1e3", "0x1", " 1", "999", "40000", "3000000000", "99999999999999999999"}
	routes := []struct {
		method, path string // %s is the id
		body         any
	}{
		{"GET", "/api/lists/%s", nil},
		{"PATCH", "/api/lists/%s", jmap{"name": "Renamed"}},
		{"DELETE", "/api/lists/%s", nil},
		{"POST", "/api/lists/%s/statuses", jmap{"status_id": 1}},
		{"POST", "/api/lists/%s/contents", jmap{"item_id": 1}},
		{"PATCH", "/api/contents/%s", jmap{"quantity": 2}},
		{"DELETE", "/api/contents/%s", nil},
		{"DELETE", "/api/contents/%s?contents=keep", nil},
		{"PATCH", "/api/ref/item_categories/%s", jmap{"name": "Renamed"}},
		{"DELETE", "/api/ref/item_categories/%s", nil},
		{"PATCH", "/api/ref/storage_locations/%s", jmap{"name": "Renamed"}},
		{"DELETE", "/api/ref/storage_locations/%s", nil},
		{"PATCH", "/api/ref/statuses/%s", jmap{"name": "Renamed"}},
		{"DELETE", "/api/ref/statuses/%s", nil},
		{"PATCH", "/api/ref/items/%s", jmap{"name": "Renamed"}},
		{"PATCH", "/api/ref/items/%s", jmap{"is_container": true}},
		{"DELETE", "/api/ref/items/%s", nil},
		{"PATCH", "/api/ref/compartments/%s", jmap{"name": "Renamed"}},
		{"PATCH", "/api/ref/compartments/%s", jmap{"item_id": 1}},
		{"DELETE", "/api/ref/compartments/%s", nil},
	}
	for _, r := range routes {
		for _, id := range bad {
			path := fmt.Sprintf(r.path, strings.ReplaceAll(id, " ", "%20"))
			status, _, raw := e.call(r.method, path, r.body)
			if status != http.StatusNotFound || raw != `{"error":"not found"}` {
				t.Errorf("%s %s: got %d %s; want 404 not found", r.method, path, status, raw)
			}
		}
	}

	// A table that is not one of the four.
	for _, r := range [][2]string{
		{"GET", "/api/ref/nope"}, {"POST", "/api/ref/nope"}, {"PATCH", "/api/ref/nope/1"}, {"DELETE", "/api/ref/nope/1"},
		{"GET", "/api/ref/lists"}, {"GET", "/api/ref/list_contents"}, {"GET", "/api/ref/Items"}, {"POST", "/api/ref/lists"},
	} {
		if status, _, raw := e.call(r[0], r[1], `{"name": "X"}`); status != http.StatusNotFound || raw != `{"error":"not found"}` {
			t.Errorf("%s %s: got %d %s; want 404 not found", r[0], r[1], status, raw)
		}
	}

	if got := names(e.arr(http.StatusOK, "GET", "/api/lists", nil)); !reflect.DeepEqual(got, []string{"Trip"}) {
		t.Fatalf("lists after all that: %v", got)
	}
	if got := names(e.arr(http.StatusOK, "GET", "/api/ref/item_categories", nil)); !reflect.DeepEqual(got, []string{"Gear"}) {
		t.Fatalf("categories after all that: %v", got)
	}
}

// ---------------------------------------------------------------- request bodies

func TestMalformedBodies(t *testing.T) {
	e := newEnv(t)
	e.obj(http.StatusCreated, "POST", "/api/lists", jmap{"name": "Trip"})
	e.obj(http.StatusCreated, "POST", "/api/ref/item_categories", jmap{"name": "Gear"})
	e.obj(http.StatusCreated, "POST", "/api/ref/storage_locations", jmap{"name": "Garage"})
	e.obj(http.StatusCreated, "POST", "/api/ref/items", jmap{"name": "socks", "item_category_id": 1, "storage_location_id": 1})
	e.obj(http.StatusCreated, "POST", "/api/lists/1/contents", jmap{"item_id": 1})

	routes := [][2]string{
		{"POST", "/api/lists"},
		{"POST", "/api/lists/preview"},
		{"PATCH", "/api/lists/1"},
		{"POST", "/api/lists/1/statuses"},
		{"POST", "/api/statuses/order"},
		{"POST", "/api/lists/1/contents"},
		{"PATCH", "/api/contents/1"},
		{"POST", "/api/ref/item_categories"},
		{"PATCH", "/api/ref/item_categories/1"},
		{"POST", "/api/ref/items"},
		{"PATCH", "/api/ref/statuses/1"},
	}
	// Not JSON, or JSON that is not an object.
	for _, r := range routes {
		for _, body := range []string{``, `{`, `{"name": }`, `{"name": "x",}`, `{name: "x"}`, `[1]`, `"text"`, `7`, `name=x`} {
			e.fails(http.StatusBadRequest, r[0], r[1], body, "invalid JSON body")
		}
	}
	// null is not an object either, where the body is a set of fields.
	for _, r := range [][2]string{
		{"PATCH", "/api/lists/1"}, {"POST", "/api/lists/1/contents"}, {"PATCH", "/api/contents/1"},
		{"POST", "/api/ref/item_categories"}, {"PATCH", "/api/ref/item_categories/1"},
	} {
		e.fails(http.StatusBadRequest, r[0], r[1], `null`, "invalid JSON body: expected an object")
	}

	// A field the route does not know.
	for _, c := range []struct {
		method, path, body, mention string
	}{
		{"POST", "/api/lists", `{"name": "New", "template": true}`, `unknown field "template"`},
		{"POST", "/api/lists", `{"name": "New", "contents": []}`, `unknown field "contents"`},
		{"POST", "/api/lists/preview", `{"sources": [1], "name": "New"}`, `unknown field "name"`},
		{"PATCH", "/api/lists/1", `{"name": "New", "rows": 3}`, `unknown field "rows"`},
		{"POST", "/api/lists/1/statuses", `{"status_id": 2, "only": [1]}`, `unknown field "only"`},
		{"POST", "/api/lists/1/contents", `{"item_id": 1, "count": 2}`, `unknown field "count"`},
		{"PATCH", "/api/contents/1", `{"item_id": 1}`, `unknown field "item_id"`},
		{"POST", "/api/ref/item_categories", `{"name": "New", "colour": "red"}`, `unknown field "colour"`},
		{"PATCH", "/api/ref/items/1", `{"category": "Gear"}`, `unknown field "category"`},
	} {
		e.fails(http.StatusBadRequest, c.method, c.path, c.body, c.mention)
	}

	// The wrong JSON type for a field, and more than one object.
	for _, c := range []struct {
		method, path, body, mention string
	}{
		{"POST", "/api/lists", `{"name": 5}`, "invalid JSON body"},
		{"POST", "/api/lists", `{"name": "New", "is_template": "yes"}`, "invalid JSON body"},
		{"POST", "/api/lists", `{"name": "New", "sources": "1"}`, "invalid JSON body"},
		{"POST", "/api/lists", `{"name": "New", "sources": [1.5]}`, "invalid JSON body"},
		{"POST", "/api/lists", `{"name": "New", "sources": ["1"]}`, "invalid JSON body"},
		{"POST", "/api/lists", `{"name": "New"} {"name": "Newer"}`, "unexpected data after the object"},
		{"POST", "/api/lists", `{"name": "New"} x`, "unexpected data after the object"},
		// The routes that read a partial object are as strict about what follows it.
		{"POST", "/api/ref/item_categories", `{"name": "Gear"} x`, "unexpected data after the object"},
		{"PATCH", "/api/lists/1", `{"name": "Trip"} {"name": "Other"}`, "unexpected data after the object"},
		{"POST", "/api/lists/1/contents", `{"item_id": 1} []`, "unexpected data after the object"},
		{"POST", "/api/lists/preview", `{"sources": 1}`, "invalid JSON body"},
		{"POST", "/api/lists/preview", `{"sources": [1]} []`, "unexpected data after the object"},
		{"POST", "/api/lists/1/statuses", `{"status_id": "2"}`, "invalid JSON body"},
		{"POST", "/api/lists/1/statuses", `{"status_id": 2.5}`, "invalid JSON body"},
		{"PATCH", "/api/lists/1", `{"name": 5}`, "name must be text"},
		{"PATCH", "/api/lists/1", `{"is_template": "yes"}`, "is_template must be true or false"},
		{"POST", "/api/lists/1/contents", `{"item_id": "1"}`, "item_id must be a whole number"},
		{"PATCH", "/api/contents/1", `{"quantity": "2"}`, "quantity must be a whole number"},
		{"POST", "/api/ref/items", `{"name": "comb", "item_category_id": "1", "storage_location_id": 1}`, "item_category_id must be a whole number"},
	} {
		e.fails(http.StatusBadRequest, c.method, c.path, c.body, c.mention)
	}

	// A body past the limit is refused as such.
	huge := `{"name": "` + strings.Repeat("n", 1<<20) + `"}`
	e.fails(http.StatusRequestEntityTooLarge, "POST", "/api/lists", huge, "request is too large")
	e.fails(http.StatusRequestEntityTooLarge, "PATCH", "/api/lists/1", huge, "request is too large")
	e.fails(http.StatusRequestEntityTooLarge, "POST", "/api/ref/item_categories", huge, "request is too large")

	// None of it changed anything.
	want := jmap{"id": 1.0, "name": "Trip", "is_template": false, "rows": 1.0, "status_counts": jmap{"1": 1.0},
		"contents": []any{jmap{"id": 1.0, "list_id": 1.0, "container_id": nil, "compartment_id": nil, "item_id": 1.0, "quantity": 1.0, "unit": nil, "status_id": 1.0, "notes": nil}}}
	if got := e.obj(http.StatusOK, "GET", "/api/lists/1", nil); !reflect.DeepEqual(got, want) {
		t.Fatalf("the list after the refused requests:\n got  %v\n want %v", got, want)
	}
	if n := len(e.arr(http.StatusOK, "GET", "/api/lists", nil)); n != 1 {
		t.Fatalf("%d lists after the refused requests, want 1", n)
	}
	ref := e.obj(http.StatusOK, "GET", "/api/ref", nil)
	if got := names(ref["item_categories"].([]any)); !reflect.DeepEqual(got, []string{"Gear"}) {
		t.Fatalf("categories after the refused requests: %v", got)
	}
	if got := names(ref["statuses"].([]any)); !reflect.DeepEqual(got, []string{"In Storage Location", "Packed", "Missing"}) {
		t.Fatalf("statuses after the refused requests: %v", got)
	}
}
