package store_test

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"packrat/internal/store"
	"packrat/internal/testdb"
)

// fx is a store with a small library to pack: four containers and four
// things that are not, all in one category and one storage location. Three
// of the containers have compartments: the bag a lid and a pocket, the car a
// boot, the kit a mesh.
type fx struct {
	t  *testing.T
	st *store.Store

	category, location       int64
	car, bag, kit, crate     int64 // containers
	socks, comb, lamp, towel int64 // not containers
	lid, pocket, boot, mesh  int64 // compartments: of the bag, the bag, the car, the kit
	stored, packed, missing  int64 // the seeded statuses; stored is the one new rows start with

	itemID          map[string]int64
	itemName        map[int64]string
	compartmentID   map[string]int64
	compartmentName map[int64]string
	statusID        map[string]int64
	statusName      map[int64]string
}

func newFx(t *testing.T) *fx {
	t.Helper()
	st := testdb.New(t)
	f := &fx{t: t, st: st, itemID: map[string]int64{}, itemName: map[int64]string{},
		compartmentID: map[string]int64{}, compartmentName: map[int64]string{}}
	f.category = rowID(mustCreate(t, st, "item_categories", `{"name": "Gear"}`))
	f.location = rowID(mustCreate(t, st, "storage_locations", `{"name": "Garage"}`))
	item := func(name string, container bool) int64 {
		id := rowID(mustCreate(t, st, "items", mustJSON(t, map[string]any{
			"name": name, "item_category_id": f.category, "storage_location_id": f.location, "is_container": container,
		})))
		f.itemID[name], f.itemName[id] = id, name
		return id
	}
	f.car, f.bag, f.kit, f.crate = item("car", true), item("bag", true), item("kit", true), item("crate", true)
	f.socks, f.comb, f.lamp, f.towel = item("socks", false), item("comb", false), item("lamp", false), item("towel", false)
	compartment := func(container int64, name string) int64 {
		id := rowID(mustCreate(t, st, "compartments", mustJSON(t, map[string]any{"item_id": container, "name": name})))
		f.compartmentID[name], f.compartmentName[id] = id, name
		return id
	}
	f.lid, f.pocket, f.boot, f.mesh = compartment(f.bag, "lid"), compartment(f.bag, "pocket"), compartment(f.car, "boot"), compartment(f.kit, "mesh")
	f.stored, f.packed, f.missing = 1, 2, 3
	f.statusID = map[string]int64{"stored": f.stored, "packed": f.packed, "missing": f.missing}
	f.statusName = map[int64]string{f.stored: "stored", f.packed: "packed", f.missing: "missing"}
	return f
}

// Rows are written as one line each, as in merge_test.go:
//
//	item [in container[/compartment]] [xQUANTITY] [unit] [@status] [#notes]
//
// e.g. "socks in bag/lid x2 pairs @packed #wool". entry turns a line into the
// body AddContent takes, sending only what the line says; show writes a row
// back the same way, leaving out a quantity of 1 and the status "stored".
func (f *fx) entry(line string) map[string]any {
	f.t.Helper()
	id := func(name string) int64 {
		id, ok := f.itemID[name]
		if !ok {
			f.t.Fatalf("row %q: no item %q", line, name)
		}
		return id
	}
	tok := strings.Fields(line)
	body := map[string]any{"item_id": id(tok[0])}
	tok = tok[1:]
	if len(tok) >= 2 && tok[0] == "in" {
		container, compartment, has := strings.Cut(tok[1], "/")
		body["container_id"] = id(container)
		if has {
			c, ok := f.compartmentID[compartment]
			if !ok {
				f.t.Fatalf("row %q: no compartment %q", line, compartment)
			}
			body["compartment_id"] = c
		}
		tok = tok[2:]
	}
	for _, w := range tok {
		switch {
		case w[0] == 'x' && len(w) > 1 && strings.Trim(w[1:], "0123456789") == "":
			body["quantity"], _ = strconv.ParseInt(w[1:], 10, 64)
		case w[0] == '@':
			s, ok := f.statusID[w[1:]]
			if !ok {
				f.t.Fatalf("row %q: no status %q", line, w[1:])
			}
			body["status_id"] = s
		case w[0] == '#':
			body["notes"] = w[1:]
		default:
			body["unit"] = w
		}
	}
	return body
}

func (f *fx) line(c store.Content) string {
	s := f.itemName[c.ItemID]
	if c.ContainerID != nil {
		s += " in " + f.itemName[*c.ContainerID]
	}
	if c.CompartmentID != nil {
		name, ok := f.compartmentName[*c.CompartmentID]
		if !ok {
			name = strconv.FormatInt(*c.CompartmentID, 10)
		}
		s += "/" + name
	}
	if c.Quantity != 1 {
		s += fmt.Sprintf(" x%d", c.Quantity)
	}
	if c.Unit != nil {
		s += " " + *c.Unit
	}
	if c.StatusID != f.stored {
		name, ok := f.statusName[c.StatusID]
		if !ok {
			name = strconv.FormatInt(c.StatusID, 10)
		}
		s += " @" + name
	}
	if c.Notes != nil {
		s += " #" + *c.Notes
	}
	return s
}

func (f *fx) lines(rows []store.Content) []string {
	out := []string{}
	for _, c := range rows {
		out = append(out, f.line(c))
	}
	return out
}

// addErr puts a row (a line of the notation) on a list.
func (f *fx) addErr(list int64, line string) (*store.Content, error) {
	f.t.Helper()
	return f.st.AddContent(context.Background(), list, fields(f.t, mustJSON(f.t, f.entry(line))))
}

// add is addErr for a row that has to go on.
func (f *fx) add(list int64, line string) *store.Content {
	f.t.Helper()
	c, err := f.addErr(list, line)
	if err != nil {
		f.t.Fatalf("add %q: %v", line, err)
	}
	return c
}

// list makes a list holding the given rows, added one by one.
func (f *fx) list(name string, lines ...string) int64 {
	f.t.Helper()
	return f.newList(store.NewList{Name: name}, lines...)
}

// template is list for a list flagged as a template.
func (f *fx) template(name string, lines ...string) int64 {
	f.t.Helper()
	return f.newList(store.NewList{Name: name, IsTemplate: true}, lines...)
}

func (f *fx) newList(n store.NewList, lines ...string) int64 {
	f.t.Helper()
	d, err := f.st.CreateList(context.Background(), n)
	if err != nil {
		f.t.Fatalf("create list %q: %v", n.Name, err)
	}
	for _, l := range lines {
		f.add(d.ID, l)
	}
	return d.ID
}

func (f *fx) get(list int64) *store.ListDetail {
	f.t.Helper()
	d, err := f.st.GetList(context.Background(), list)
	if err != nil {
		f.t.Fatalf("get list %d: %v", list, err)
	}
	return d
}

// want checks a list's rows, in the order they were made, against lines of
// the notation.
func (f *fx) want(list int64, lines ...string) {
	f.t.Helper()
	if lines == nil {
		lines = []string{}
	}
	d := f.get(list)
	if got := f.lines(d.Contents); !reflect.DeepEqual(got, lines) {
		f.t.Errorf("list %q:\n got  %q\n want %q", d.Name, got, lines)
	}
}

// row finds a row of a list by its place: "socks in bag" (whatever the
// compartment), or "bag" for the top level.
func (f *fx) row(list int64, place string) store.Content {
	f.t.Helper()
	for _, c := range f.get(list).Contents {
		at := f.itemName[c.ItemID]
		if c.ContainerID != nil {
			at += " in " + f.itemName[*c.ContainerID]
		}
		if at == place {
			return c
		}
	}
	f.t.Fatalf("list %d has no row %q", list, place)
	return store.Content{}
}

func (f *fx) patchErr(id int64, js string) (*store.Content, error) {
	f.t.Helper()
	return f.st.UpdateContent(context.Background(), id, fields(f.t, js))
}

// patch is patchErr for a change that has to work.
func (f *fx) patch(id int64, js string) *store.Content {
	f.t.Helper()
	c, err := f.patchErr(id, js)
	if err != nil {
		f.t.Fatalf("patch %d %s: %v", id, js, err)
	}
	return c
}

// move puts a row into a container (0 = the top level).
func (f *fx) move(id, container int64) (*store.Content, error) {
	f.t.Helper()
	if container == 0 {
		return f.patchErr(id, `{"container_id": null}`)
	}
	return f.patchErr(id, mustJSON(f.t, map[string]any{"container_id": container}))
}

func (f *fx) del(id int64, keepContents bool) {
	f.t.Helper()
	if err := f.st.DeleteContent(context.Background(), id, keepContents); err != nil {
		f.t.Fatalf("delete row %d: %v", id, err)
	}
}

// count runs a scalar count query.
func (f *fx) count(sql string, args ...any) int {
	f.t.Helper()
	var n int
	if err := f.st.Pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

func listNames(lists []store.List) []string {
	out := []string{}
	for _, l := range lists {
		out = append(out, l.Name)
	}
	return out
}

// ---------------------------------------------------------------- create

func TestCreateListFromScratch(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()

	d, err := st.CreateList(ctx, store.NewList{Name: "  Park Day "})
	if err != nil {
		t.Fatal(err)
	}
	if d.ID != 1 || d.Name != "Park Day" || d.IsTemplate || d.Rows != 0 {
		t.Fatalf("created %+v", d)
	}
	if d.Contents == nil || len(d.Contents) != 0 || d.StatusCounts == nil || len(d.StatusCounts) != 0 {
		t.Fatalf("an empty list should have empty, non-nil contents and status counts: %+v", d)
	}
	// An empty set of sources is from scratch too.
	tpl, err := st.CreateList(ctx, store.NewList{Name: "Weekend", IsTemplate: true, Sources: []int64{}})
	if err != nil || !tpl.IsTemplate || tpl.ID != 2 || tpl.Rows != 0 {
		t.Fatalf("template: %+v, %v", tpl, err)
	}
	if _, err := st.CreateList(ctx, store.NewList{Name: strings.Repeat("é", 127)}); err != nil {
		t.Fatalf("a name of 127 characters: %v", err)
	}

	bad := []struct {
		n       store.NewList
		mention string
	}{
		{store.NewList{}, "name is required"},
		{store.NewList{Name: "   "}, "name is required"},
		{store.NewList{Name: strings.Repeat("n", 128)}, "name is too long (at most 127 characters)"},
	}
	for _, c := range bad {
		_, err := st.CreateList(ctx, c.n)
		wantValidation(t, fmt.Sprintf("create %+v", c.n), err, c.mention)
	}
	for _, name := range []string{"Park Day", " Park Day  ", "Weekend"} {
		_, err := st.CreateList(ctx, store.NewList{Name: name, IsTemplate: true})
		wantConflict(t, "duplicate "+name, err, "there is already a list with that name")
	}
	lists, err := st.ListLists(ctx)
	if err != nil || len(lists) != 3 {
		t.Fatalf("%d lists after the refused creates, want 3 (%v)", len(lists), err)
	}
}

func TestCreateListFromSources(t *testing.T) {
	fx := newFx(t)
	ctx := context.Background()
	campLines := []string{"car @packed", "bag in car @packed", "socks in bag x2 pairs @missing #wool", "lamp in car", "towel in bag"}
	cityLines := []string{"bag @packed", "socks in bag x4 pairs @packed", "comb in bag @packed", "kit in bag", "towel in kit x2"}
	camp := fx.template("Camping", campLines...)
	city := fx.list("City Break", cityLines...)

	create := func(name string, sources ...int64) *store.ListDetail {
		t.Helper()
		d, err := fx.st.CreateList(ctx, store.NewList{Name: name, Sources: sources})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return d
	}

	cases := []struct {
		name    string
		sources []int64
		want    []string
	}{
		// Everything on either. The bag stays in the car, where the first
		// list has it, and what the second packs into it follows; the socks
		// are one row with the larger quantity. The template's rows keep
		// their statuses; the ordinary list's start at the default.
		{"Both", []int64{camp, city},
			[]string{"car @packed", "bag in car @packed", "socks in bag x4 pairs @missing #wool", "lamp in car", "towel in bag",
				"comb in bag", "kit in bag", "towel in kit x2"}},
		{"Both, city first", []int64{city, camp},
			[]string{"bag", "socks in bag x4 pairs", "comb in bag", "kit in bag", "towel in kit x2",
				"car @packed", "lamp in car", "towel in bag"}},
		// One source is a copy (an ordinary list's statuses reset).
		{"Copy of a template", []int64{camp}, campLines},
		{"Copy of a list", []int64{city},
			[]string{"bag", "socks in bag x4 pairs", "comb in bag", "kit in bag", "towel in kit x2"}},
		// A list named twice counts once.
		{"Repeated", []int64{city, city, city},
			[]string{"bag", "socks in bag x4 pairs", "comb in bag", "kit in bag", "towel in kit x2"}},
		{"Both, repeated", []int64{camp, city, camp, city},
			[]string{"car @packed", "bag in car @packed", "socks in bag x4 pairs @missing #wool", "lamp in car", "towel in bag",
				"comb in bag", "kit in bag", "towel in kit x2"}},
	}
	for _, c := range cases {
		d := create(c.name, c.sources...)
		if got := fx.lines(d.Contents); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n got  %q\n want %q", c.name, got, c.want)
		}
		// What CreateList returns is what GetList then finds: rows of the new
		// list, with its counts.
		if again := fx.get(d.ID); !reflect.DeepEqual(again, d) {
			t.Errorf("%s: GetList differs from what CreateList returned:\n %+v\n %+v", c.name, again, d)
		}
		counts := map[int64]int{}
		for _, row := range d.Contents {
			if row.ListID != d.ID {
				t.Errorf("%s: row %d belongs to list %d", c.name, row.ID, row.ListID)
			}
			counts[row.StatusID]++
		}
		if d.Rows != len(c.want) || !reflect.DeepEqual(d.StatusCounts, counts) || d.IsTemplate {
			t.Errorf("%s: rows %d, status counts %v, template %v", c.name, d.Rows, d.StatusCounts, d.IsTemplate)
		}
	}

	// The sources are as they were, and the new list is its own: changing
	// it leaves them alone.
	both := fx.get(3)
	fx.patch(both.Contents[0].ID, `{"quantity": 9}`)
	fx.del(fx.row(both.ID, "bag in car").ID, false)
	fx.want(both.ID, "car x9 @packed", "lamp in car")
	fx.want(camp, campLines...)
	fx.want(city, cityLines...)

	// The new list's template flag is its own, not its sources'.
	d, err := fx.st.CreateList(ctx, store.NewList{Name: "Template from a list", IsTemplate: true, Sources: []int64{city}})
	if err != nil || !d.IsTemplate || d.Rows != 5 {
		t.Fatalf("template from a list: %+v, %v", d, err)
	}

	// Empty sources make an empty list, and add nothing to another.
	empty := fx.list("Empty")
	if d := create("Of nothing", empty); d.Rows != 0 || len(d.Contents) != 0 {
		t.Errorf("copy of an empty list: %v", fx.lines(d.Contents))
	}
	if d := create("Of nothing and something", empty, city); d.Rows != 5 {
		t.Errorf("an empty list and another: %v", fx.lines(d.Contents))
	}
	if d := create("Of something and nothing", city, empty); d.Rows != 5 {
		t.Errorf("a list and an empty one: %v", fx.lines(d.Contents))
	}
}

func TestCreateListRefused(t *testing.T) {
	fx := newFx(t)
	ctx := context.Background()
	camp := fx.list("Camping", "car", "bag in car", "socks in bag")
	lists := fx.count(`SELECT count(*) FROM Lists`)
	rows := fx.count(`SELECT count(*) FROM List_Contents`)

	bad := []struct {
		n       store.NewList
		mention string
	}{
		{store.NewList{Name: "New", Sources: []int64{999}}, "sources: there is no list 999"},
		{store.NewList{Name: "New", Sources: []int64{camp, 999}}, "sources: there is no list 999"},
		{store.NewList{Name: "New", Sources: []int64{0}}, "sources: there is no list 0"},
		{store.NewList{Name: "New", Sources: []int64{-1}}, "sources: there is no list -1"},
		{store.NewList{Name: "New", Sources: []int64{pastSmallint}}, "sources: there is no list 40000"},
		{store.NewList{Name: "New", Sources: []int64{camp, pastInt}}, "sources: there is no list 3000000000"},
		{store.NewList{Name: "", Sources: []int64{camp}}, "name is required"},
	}
	for _, c := range bad {
		_, err := fx.st.CreateList(ctx, c.n)
		wantValidation(t, fmt.Sprintf("create %+v", c.n), err, c.mention)
	}
	// A taken name is found out after the rows are merged; nothing stays behind.
	_, err := fx.st.CreateList(ctx, store.NewList{Name: "Camping", Sources: []int64{camp}})
	wantConflict(t, "duplicate name with sources", err, "there is already a list with that name")

	if a, b := fx.count(`SELECT count(*) FROM Lists`), fx.count(`SELECT count(*) FROM List_Contents`); a != lists || b != rows {
		t.Fatalf("refused creates left %d lists and %d rows, want %d and %d", a, b, lists, rows)
	}
}

func TestPreviewMerge(t *testing.T) {
	fx := newFx(t)
	ctx := context.Background()
	camp := fx.template("Camping", "car", "bag in car", "kit in bag", "socks in bag x2", "socks in kit", "comb in kit", "lamp in car", "towel in car")
	city := fx.list("City Break", "bag", "socks in bag x4", "comb in bag", "towel")
	beach := fx.list("Beach", "crate", "towel in crate", "socks in crate", "bag in crate", "lamp in bag")
	empty := fx.list("Empty")

	cases := []struct {
		sources []int64
		rows    int
	}{
		{nil, 0},
		{[]int64{}, 0},
		{[]int64{empty}, 0},
		{[]int64{camp}, 8},
		{[]int64{city}, 4},
		{[]int64{camp, camp}, 8},
		// The city list adds the comb in the bag and the loose towel; its bag
		// and the socks in it are there already.
		{[]int64{camp, city}, 10},
		{[]int64{city, camp}, 10},
		{[]int64{camp, empty}, 8},
		{[]int64{empty, camp, empty}, 8},
		// The beach list adds the crate and what is in it, bar the bag, and
		// the lamp in the bag.
		{[]int64{camp, city, beach}, 14},
		{[]int64{beach, city, camp}, 14},
		{[]int64{beach, camp}, 12},
	}
	lists := fx.count(`SELECT count(*) FROM Lists`)
	for i, c := range cases {
		p, err := fx.st.PreviewMerge(ctx, c.sources)
		if err != nil {
			t.Fatalf("preview %v: %v", c.sources, err)
		}
		if p.Rows != c.rows {
			t.Errorf("preview %v: %d rows, want %d", c.sources, p.Rows, c.rows)
		}
		// The preview made nothing...
		if n := fx.count(`SELECT count(*) FROM Lists`); n != lists {
			t.Fatalf("preview %v left %d lists, want %d", c.sources, n, lists)
		}
		// ...and CreateList makes exactly as many rows as it promised.
		d, err := fx.st.CreateList(ctx, store.NewList{Name: fmt.Sprintf("Case %d", i), Sources: c.sources})
		if err != nil {
			t.Fatalf("create from %v: %v", c.sources, err)
		}
		if d.Rows != p.Rows || len(d.Contents) != p.Rows {
			t.Errorf("from %v: created %d rows, previewed %d", c.sources, d.Rows, p.Rows)
		}
		if err := fx.st.DeleteList(ctx, d.ID); err != nil {
			t.Fatal(err)
		}
	}

	for _, ids := range [][]int64{{999}, {camp, 999}, {pastSmallint}, {0}} {
		_, err := fx.st.PreviewMerge(ctx, ids)
		wantValidation(t, fmt.Sprintf("preview %v", ids), err, "sources: there is no list")
	}
}

// ---------------------------------------------------------------- read

func TestListListsAndGetList(t *testing.T) {
	fx := newFx(t)
	ctx := context.Background()

	lists, err := fx.st.ListLists(ctx)
	if err != nil || lists == nil || len(lists) != 0 {
		t.Fatalf("no lists should be an empty, non-nil slice: %v, %v", lists, err)
	}

	paris := fx.list("paris 2027", "bag @packed", "socks in bag @packed", "comb in bag", "towel @missing")
	park := fx.template("Park Day")
	alps := fx.list("Alps", "car", "lamp in car")
	zoo := fx.list("Zoo", "socks @missing")

	lists, err = fx.st.ListLists(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// By name, without regard to case; statuses nothing has are left out.
	want := []store.List{
		{ID: alps, Name: "Alps", Rows: 2, StatusCounts: map[int64]int{fx.stored: 2}},
		{ID: paris, Name: "paris 2027", Rows: 4, StatusCounts: map[int64]int{fx.stored: 1, fx.packed: 2, fx.missing: 1}},
		{ID: park, Name: "Park Day", IsTemplate: true, Rows: 0, StatusCounts: map[int64]int{}},
		{ID: zoo, Name: "Zoo", Rows: 1, StatusCounts: map[int64]int{fx.missing: 1}},
	}
	if !reflect.DeepEqual(lists, want) {
		t.Fatalf("lists:\n got  %+v\n want %+v", lists, want)
	}

	// GetList has the same summary, and the rows in the order they were added.
	d := fx.get(paris)
	if !reflect.DeepEqual(d.List, want[1]) {
		t.Fatalf("detail summary %+v, want %+v", d.List, want[1])
	}
	wantRows := []store.Content{
		{ID: 1, ListID: paris, ItemID: fx.bag, Quantity: 1, StatusID: fx.packed},
		{ID: 2, ListID: paris, ContainerID: &fx.bag, ItemID: fx.socks, Quantity: 1, StatusID: fx.packed},
		{ID: 3, ListID: paris, ContainerID: &fx.bag, ItemID: fx.comb, Quantity: 1, StatusID: fx.stored},
		{ID: 4, ListID: paris, ItemID: fx.towel, Quantity: 1, StatusID: fx.missing},
	}
	if !reflect.DeepEqual(d.Contents, wantRows) {
		t.Fatalf("contents:\n got  %+v\n want %+v", d.Contents, wantRows)
	}
	if d := fx.get(park); d.Contents == nil || len(d.Contents) != 0 || !d.IsTemplate {
		t.Fatalf("empty template: %+v", d)
	}

	for _, id := range []int64{999, 0, -1, pastSmallint, pastInt} {
		_, err := fx.st.GetList(ctx, id)
		wantNotFound(t, fmt.Sprintf("get list %d", id), err)
	}
}

// ---------------------------------------------------------------- update

func TestUpdateList(t *testing.T) {
	fx := newFx(t)
	ctx := context.Background()
	trip := fx.list("Trip", "bag", "socks in bag x2 @packed")
	other := fx.template("Other")
	update := func(id int64, js string) (*store.ListDetail, error) {
		return fx.st.UpdateList(ctx, id, fields(t, js))
	}

	// Each field on its own leaves the other; the list comes back whole.
	d, err := update(trip, `{"name": "  Road Trip "}`)
	if err != nil || d.Name != "Road Trip" || d.IsTemplate || d.ID != trip {
		t.Fatalf("rename: %+v, %v", d, err)
	}
	if got := fx.lines(d.Contents); !reflect.DeepEqual(got, []string{"bag", "socks in bag x2 @packed"}) || d.Rows != 2 ||
		!reflect.DeepEqual(d.StatusCounts, map[int64]int{fx.stored: 1, fx.packed: 1}) {
		t.Fatalf("rename should return the list with its contents: %+v", d)
	}
	if d, err = update(trip, `{"is_template": true}`); err != nil || d.Name != "Road Trip" || !d.IsTemplate {
		t.Fatalf("flag: %+v, %v", d, err)
	}
	if d, err = update(trip, `{"name": "Trip", "is_template": false}`); err != nil || d.Name != "Trip" || d.IsTemplate {
		t.Fatalf("both: %+v, %v", d, err)
	}
	if d, err = update(trip, `{}`); err != nil || d.Name != "Trip" || d.IsTemplate || d.Rows != 2 {
		t.Fatalf("nothing: %+v, %v", d, err)
	}
	// Its own name is not a duplicate.
	if _, err = update(trip, `{"name": "Trip"}`); err != nil {
		t.Fatalf("rename to its own name: %v", err)
	}
	if _, err = update(trip, mustJSON(t, map[string]any{"name": strings.Repeat("é", 127)})); err != nil {
		t.Fatalf("a name of 127 characters: %v", err)
	}
	if _, err = update(trip, `{"name": "Trip"}`); err != nil {
		t.Fatal(err)
	}

	bad := []struct{ body, mention string }{
		{`{"name": ""}`, "name is required"},
		{`{"name": "  "}`, "name is required"},
		{`{"name": null}`, "name is required"},
		{`{"name": 7}`, "name must be text"},
		{`{"name": "` + strings.Repeat("n", 128) + `"}`, "name is too long"},
		{`{"is_template": "yes"}`, "is_template must be true or false"},
		{`{"is_template": null}`, "is_template must be true or false"},
		{`{"is_template": 1}`, "is_template must be true or false"},
		{`{"name": "Trip", "contents": []}`, `unknown field "contents" for a list`},
		{`{"id": 9}`, `unknown field "id"`},
		// One bad field refuses the whole change.
		{`{"name": "Renamed", "is_template": "no"}`, "is_template must be true or false"},
	}
	for _, c := range bad {
		_, err := update(trip, c.body)
		wantValidation(t, "update "+c.body, err, c.mention)
	}
	_, err = update(trip, `{"name": " Other "}`)
	wantConflict(t, "rename to a taken name", err, "there is already a list with that name")
	_, err = update(trip, `{"is_template": true, "name": "Other"}`)
	wantConflict(t, "rename to a taken name with a flag", err, "there is already a list with that name")

	if d := fx.get(trip); d.Name != "Trip" || d.IsTemplate {
		t.Fatalf("refused updates changed the list: %+v", d.List)
	}
	if d := fx.get(other); d.Name != "Other" || !d.IsTemplate {
		t.Fatalf("the other list changed: %+v", d.List)
	}

	for _, id := range []int64{999, pastSmallint, pastInt} {
		for _, body := range []string{`{"name": "Nowhere"}`, `{"is_template": true}`, `{}`} {
			_, err := update(id, body)
			wantNotFound(t, fmt.Sprintf("update list %d %s", id, body), err)
		}
	}
}

// ---------------------------------------------------------------- delete

func TestDeleteList(t *testing.T) {
	fx := newFx(t)
	ctx := context.Background()
	trip := fx.list("Trip", "car", "bag in car", "kit in bag", "socks in kit", "socks in bag", "comb")
	other := fx.list("Other", "bag", "socks in bag")
	items := len(mustList(t, fx.st, "items"))

	if err := fx.st.DeleteList(ctx, trip); err != nil {
		t.Fatal(err)
	}
	_, err := fx.st.GetList(ctx, trip)
	wantNotFound(t, "get the deleted list", err)
	if n := fx.count(`SELECT count(*) FROM List_Contents WHERE List_ID = $1`, trip); n != 0 {
		t.Fatalf("%d rows of the deleted list are left", n)
	}
	// The other list and the library are untouched.
	fx.want(other, "bag", "socks in bag")
	if lists, _ := fx.st.ListLists(ctx); !reflect.DeepEqual(listNames(lists), []string{"Other"}) {
		t.Fatalf("lists left: %v", listNames(lists))
	}
	if n := len(mustList(t, fx.st, "items")); n != items {
		t.Fatalf("%d items in the library, want %d", n, items)
	}
	if row := rowNamed(mustList(t, fx.st, "items"), "kit"); row == nil || num(row["list_count"]) != 0 {
		t.Fatalf("the kit should be in the library, on no list: %v", row)
	}
	// What was only on that list can now leave the library.
	if err := fx.st.DeleteRef(ctx, store.RefTableByKey("items"), fx.kit); err != nil {
		t.Fatalf("delete an item that was on the deleted list: %v", err)
	}

	wantNotFound(t, "second delete", fx.st.DeleteList(ctx, trip))
	for _, id := range []int64{999, 0, pastSmallint, pastInt} {
		wantNotFound(t, fmt.Sprintf("delete list %d", id), fx.st.DeleteList(ctx, id))
	}
	// The name is free again.
	if again := fx.list("Trip"); again == trip {
		t.Fatalf("the new list reuses id %d", again)
	}
	// An empty list goes just the same.
	if err := fx.st.DeleteList(ctx, fx.list("Empty")); err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------- statuses

func TestSetAllStatuses(t *testing.T) {
	fx := newFx(t)
	ctx := context.Background()
	trip := fx.list("Trip", "car @packed", "bag in car x2 @missing #blue", "socks in bag x3 pairs", "socks in car @packed")
	other := fx.list("Other", "bag @packed", "socks in bag")

	d, err := fx.st.SetAllStatuses(ctx, trip, fx.missing)
	if err != nil {
		t.Fatal(err)
	}
	// Every row, and nothing but the status.
	want := []string{"car @missing", "bag in car x2 @missing #blue", "socks in bag x3 pairs @missing", "socks in car @missing"}
	if got := fx.lines(d.Contents); !reflect.DeepEqual(got, want) {
		t.Fatalf("returned rows %q, want %q", got, want)
	}
	if d.ID != trip || d.Name != "Trip" || d.Rows != 4 || !reflect.DeepEqual(d.StatusCounts, map[int64]int{fx.missing: 4}) {
		t.Fatalf("returned list %+v", d.List)
	}
	fx.want(trip, want...)
	fx.want(other, "bag @packed", "socks in bag")

	// Back to the start, and a status the rows already have.
	for range 2 {
		if d, err = fx.st.SetAllStatuses(ctx, trip, fx.stored); err != nil || !reflect.DeepEqual(d.StatusCounts, map[int64]int{fx.stored: 4}) {
			t.Fatalf("reset: %+v, %v", d, err)
		}
	}
	fx.want(trip, "car", "bag in car x2 #blue", "socks in bag x3 pairs", "socks in car")

	// An empty list has nothing to change.
	empty := fx.list("Empty")
	if d, err = fx.st.SetAllStatuses(ctx, empty, fx.packed); err != nil || d.Rows != 0 || len(d.StatusCounts) != 0 {
		t.Fatalf("empty list: %+v, %v", d, err)
	}

	for _, status := range []int64{99, 0, -1, pastSmallint, pastInt} {
		_, err := fx.st.SetAllStatuses(ctx, trip, status)
		wantValidation(t, fmt.Sprintf("status %d", status), err, fmt.Sprintf("status_id: there is no status %d", status))
	}
	for _, list := range []int64{999, 0, pastSmallint, pastInt} {
		_, err := fx.st.SetAllStatuses(ctx, list, fx.packed)
		wantNotFound(t, fmt.Sprintf("list %d", list), err)
	}
	fx.want(trip, "car", "bag in car x2 #blue", "socks in bag x3 pairs", "socks in car")
}
