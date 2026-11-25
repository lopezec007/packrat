package store

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// The merge tests need no database: they pack these items. The first three
// are containers; "ghost" is an item no list ever has a row for.
const (
	car int64 = iota + 1
	bag
	kit
	socks
	comb
	lamp
	towel
	ghost
)

// Compartments: the bag has a lid and a pocket, the car a boot, the kit a
// mesh. (Merge and sound only carry the IDs: which container a compartment
// belongs to is the database's business.)
const (
	lid int64 = iota + 101
	pocket
	boot
	mesh
)

var (
	mergeNames        = map[int64]string{car: "car", bag: "bag", kit: "kit", socks: "socks", comb: "comb", lamp: "lamp", towel: "towel", ghost: "ghost"}
	mergeCompartments = map[int64]string{lid: "lid", pocket: "pocket", boot: "boot", mesh: "mesh"}
	mergeContainers   = map[int64]bool{car: true, bag: true, kit: true}
	mergeStatuses     = []string{1: "stored", 2: "packed", 3: "missing"} // 1 is the default status
)

const (
	stored int64 = iota + 1
	packed
	missing
)

// Rows are written as one line each:
//
//	item [in container[/compartment]] [xQUANTITY] [unit] [@status] [#notes]
//
// e.g. "socks in bag/lid x2 pairs @packed #wool". The quantity is 1 and the
// status "stored" unless the line says otherwise; show leaves both out again
// when they are just that.
func parseRow(line string) Entry {
	find := func(names map[int64]string, what, name string) int64 {
		for id, n := range names {
			if n == name {
				return id
			}
		}
		panic(fmt.Sprintf("row %q: no %s %q", line, what, name))
	}
	id := func(name string) int64 { return find(mergeNames, "item", name) }
	tok := strings.Fields(line)
	e := Entry{ItemID: id(tok[0]), Quantity: 1, StatusID: stored}
	tok = tok[1:]
	if len(tok) >= 2 && tok[0] == "in" {
		container, compartment, has := strings.Cut(tok[1], "/")
		e.ContainerID = new(id(container))
		if has {
			e.CompartmentID = new(find(mergeCompartments, "compartment", compartment))
		}
		tok = tok[2:]
	}
	for _, w := range tok {
		switch {
		case w[0] == 'x' && len(w) > 1 && strings.Trim(w[1:], "0123456789") == "":
			e.Quantity, _ = strconv.ParseInt(w[1:], 10, 64)
		case w[0] == '@':
			e.StatusID = 0
			for i, n := range mergeStatuses {
				if n == w[1:] {
					e.StatusID = int64(i)
				}
			}
			if e.StatusID == 0 {
				panic(fmt.Sprintf("row %q: no status %q", line, w[1:]))
			}
		case w[0] == '#':
			e.Notes = new(w[1:])
		default:
			e.Unit = new(w)
		}
	}
	return e
}

func rows(lines ...string) []Entry {
	out := make([]Entry, len(lines))
	for i, l := range lines {
		out[i] = parseRow(l)
	}
	return out
}

// list is an ordinary source list; template is one flagged as a template.
func list(lines ...string) Source     { return Source{Rows: rows(lines...)} }
func template(lines ...string) Source { return Source{IsTemplate: true, Rows: rows(lines...)} }

func show(rows []Entry) []string {
	out := []string{}
	for _, e := range rows {
		s := mergeNames[e.ItemID]
		if c := containerOf(e); c != 0 {
			s += " in " + mergeNames[c]
		}
		if e.CompartmentID != nil {
			s += "/" + mergeCompartments[*e.CompartmentID]
		}
		if e.Quantity != 1 {
			s += fmt.Sprintf(" x%d", e.Quantity)
		}
		if e.Unit != nil {
			s += " " + *e.Unit
		}
		if e.StatusID != stored {
			s += " @" + mergeStatuses[e.StatusID]
		}
		if e.Notes != nil {
			s += " #" + *e.Notes
		}
		out = append(out, s)
	}
	return out
}

// wantRows checks rows, in order, against lines in the notation above.
func wantRows(t *testing.T, what string, got []Entry, lines ...string) {
	t.Helper()
	if lines == nil {
		lines = []string{}
	}
	if s := show(got); !reflect.DeepEqual(s, lines) {
		t.Errorf("%s:\n got  %q\n want %q", what, s, lines)
	}
}

func merge(sources ...Source) []Entry {
	return Merge(sources, mergeContainers, stored)
}

// ---------------------------------------------------------------- several sources

func TestMergeSamePlacement(t *testing.T) {
	a := list("bag", "socks in bag x2", "comb in bag x3 #mine", "lamp x4 l", "towel x2 kg")
	b := list("bag x2", "socks in bag x5", "comb in bag #yours", "lamp x6 l", "towel x9 lb")
	// One row per placement: the larger quantity when both count in the same
	// unit (none, or the same one); otherwise the first list's row stands.
	// Unit and notes are the first list's.
	wantRows(t, "a+b", merge(a, b),
		"bag x2", "socks in bag x5", "comb in bag x3 #mine", "lamp x6 l", "towel x2 kg")
	wantRows(t, "b+a", merge(b, a),
		"bag x2", "socks in bag x5", "comb in bag x3 #yours", "lamp x6 l", "towel x9 lb")

	// A unit on one side only is a different unit.
	wantRows(t, "unit against none", merge(list("socks x2"), list("socks x5 pairs")), "socks x2")
	wantRows(t, "none against unit", merge(list("socks x2 pairs"), list("socks x5")), "socks x2 pairs")
}

func TestMergeEverythingOnAnyList(t *testing.T) {
	a := list("bag", "socks in bag", "towel")
	b := list("kit", "comb in kit", "towel", "lamp")
	c := list("lamp x3", "car")
	wantRows(t, "a+b+c", merge(a, b, c),
		"bag", "socks in bag", "towel", "kit", "comb in kit", "lamp x3", "car")

	// The same thing in two containers is two rows: socks in the bag and
	// socks in the kit.
	wantRows(t, "two places", merge(list("bag", "socks in bag x2"), list("kit", "socks in kit x3"), list("socks")),
		"bag", "socks in bag x2", "kit", "socks in kit x3", "socks")
}

func TestMergeContainerStaysWhereTheFirstListPutsIt(t *testing.T) {
	a := list("car", "bag in car", "socks in bag")
	b := list("bag x2", "comb in bag", "kit in bag", "lamp in kit", "towel")
	// The bag is in the car (a had it first); what b packs into the bag
	// follows it there, and b's own row for the bag is dropped whole.
	wantRows(t, "a+b", merge(a, b),
		"car", "bag in car", "socks in bag", "comb in bag", "kit in bag", "lamp in kit", "towel")
	// The other way round the bag stays at the top level.
	wantRows(t, "b+a", merge(b, a),
		"bag x2", "comb in bag", "kit in bag", "lamp in kit", "towel", "car", "socks in bag")

	// Lists that nest two containers opposite ways cannot make a circle.
	x := list("car", "bag in car", "socks in bag")
	y := list("bag", "car in bag", "comb in car")
	wantRows(t, "x+y", merge(x, y), "car", "bag in car", "socks in bag", "comb in car")
	wantRows(t, "y+x", merge(y, x), "bag", "car in bag", "comb in car", "socks in bag")
}

func TestMergeStatuses(t *testing.T) {
	tpl := template("bag @packed", "socks in bag @missing", "towel")
	old := list("bag @missing", "comb in bag @packed", "socks in bag @packed")
	// A template's rows keep their status; an ordinary list's rows start at
	// the default. A placement on both takes the first list's.
	wantRows(t, "template first", merge(tpl, old),
		"bag @packed", "socks in bag @missing", "towel", "comb in bag")
	wantRows(t, "template second", merge(old, tpl),
		"bag", "comb in bag", "socks in bag", "towel")
	wantRows(t, "two templates", merge(tpl, template("comb @packed", "towel @missing")),
		"bag @packed", "socks in bag @missing", "towel", "comb @packed")

	// The default is whatever the caller says it is.
	got := Merge([]Source{old, tpl}, mergeContainers, missing)
	wantRows(t, "default missing", got, "bag @missing", "comb in bag @missing", "socks in bag @missing", "towel")
}

// ---------------------------------------------------------------- compartments

func TestMergeCompartments(t *testing.T) {
	// A copy keeps every row in its compartment.
	lines := []string{"car", "bag in car/boot", "socks in bag/lid x2", "kit in bag/pocket", "comb in kit/mesh", "towel in bag", "lamp in car"}
	wantRows(t, "copy of a list", merge(list(lines...)), lines...)
	wantRows(t, "copy of a template", merge(template(lines...)), lines...)

	// The same placement in different compartments is still one row: the
	// first list's compartment stands (and "none" is a compartment like any
	// other), while the quantity is the larger as ever.
	a := list("bag", "socks in bag/lid x2", "comb in bag", "towel in bag/pocket x3", "lamp in bag/lid")
	b := list("bag", "socks in bag/pocket x5", "comb in bag/lid x4", "towel in bag", "lamp in bag/lid x2")
	wantRows(t, "a+b", merge(a, b), "bag", "socks in bag/lid x5", "comb in bag x4", "towel in bag/pocket x3", "lamp in bag/lid x2")
	wantRows(t, "b+a", merge(b, a), "bag", "socks in bag/pocket x5", "comb in bag/lid x4", "towel in bag x3", "lamp in bag/lid x2")

	// A container goes where the first list has it, compartment included;
	// what a later list packs into its compartments follows it there, since
	// a compartment belongs to the container, wherever that is.
	x := list("car", "bag in car/boot", "socks in bag/lid")
	y := list("bag", "comb in bag/pocket", "kit in bag/lid", "lamp in kit/mesh", "socks in bag/pocket")
	wantRows(t, "x+y", merge(x, y), "car", "bag in car/boot", "socks in bag/lid", "comb in bag/pocket", "kit in bag/lid", "lamp in kit/mesh")
	wantRows(t, "y+x", merge(y, x), "bag", "comb in bag/pocket", "kit in bag/lid", "lamp in kit/mesh", "socks in bag/pocket", "car")

	// Something in two containers keeps the compartment it has in each.
	wantRows(t, "two places", merge(list("bag", "socks in bag/lid"), list("kit", "socks in kit/mesh", "socks")),
		"bag", "socks in bag/lid", "kit", "socks in kit/mesh", "socks")
}

// ---------------------------------------------------------------- one source, no sources

func TestMergeSingleSourceIsACopy(t *testing.T) {
	lines := []string{"car x2", "bag in car @packed", "socks in bag x2 pairs @missing #wool", "socks in car", "towel x3 kg #big", "kit in bag"}
	reset := []string{"car x2", "bag in car", "socks in bag x2 pairs #wool", "socks in car", "towel x3 kg #big", "kit in bag"}
	// A template is copied as it is; an ordinary list with every status back
	// at the default.
	wantRows(t, "a template", merge(template(lines...)), lines...)
	wantRows(t, "a list", merge(list(lines...)), reset...)
	// Twice the same list is that list.
	wantRows(t, "a template and itself", merge(template(lines...), template(lines...)), lines...)
	wantRows(t, "a list and itself", merge(list(lines...), list(lines...)), reset...)
}

func TestMergeEmptySources(t *testing.T) {
	a := list("bag", "socks in bag")
	if got := Merge(nil, mergeContainers, stored); got != nil {
		t.Errorf("no lists: %v", show(got))
	}
	if got := merge(list()); len(got) != 0 {
		t.Errorf("an empty list: %v", show(got))
	}
	if got := merge(list(), template()); len(got) != 0 {
		t.Errorf("two empty lists: %v", show(got))
	}
	wantRows(t, "a list and an empty one", merge(a, list()), "bag", "socks in bag")
	wantRows(t, "an empty list and another", merge(list(), a), "bag", "socks in bag")
	wantRows(t, "a list between empty ones", merge(template(), a, list()), "bag", "socks in bag")
}

func TestMergeLeavesItsSourcesAlone(t *testing.T) {
	lines := [][]string{
		{"car", "bag in car", "kit in bag", "socks in kit x2", "socks in bag x3", "comb in ghost"},
		{"kit", "bag in kit", "socks in bag x9", "towel @packed"},
	}
	sources := []Source{list(lines[0]...), template(lines[1]...)}
	merge(sources...)
	for i, src := range sources {
		wantRows(t, fmt.Sprintf("source %d afterwards", i), src.Rows, lines[i]...)
	}
}

// ---------------------------------------------------------------- sound

func TestSound(t *testing.T) {
	cases := []struct {
		name     string
		in, want []string
	}{
		{"nothing", nil, nil},
		{"a sound list is left as it is",
			[]string{"socks in bag x2", "car", "bag in car", "socks x3", "kit in bag", "comb in kit"},
			[]string{"socks in bag x2", "car", "bag in car", "socks x3", "kit in bag", "comb in kit"}},
		// A container that is not on the list: the row goes to the top level...
		{"orphan",
			[]string{"socks in ghost", "bag", "comb in bag"},
			[]string{"socks", "bag", "comb in bag"}},
		// ...and takes what it holds along, whichever of the two rows comes first.
		{"orphaned container",
			[]string{"kit in ghost", "comb in kit x2", "socks"},
			[]string{"kit", "comb in kit x2", "socks"}},
		{"orphaned container, contents listed first",
			[]string{"comb in kit x2", "kit in ghost", "socks"},
			[]string{"comb in kit x2", "kit", "socks"}},
		// What hangs from a circle stays where it is while the circle is opened.
		{"contents of a circle, listed first",
			[]string{"comb in kit", "bag in kit", "kit in bag"},
			[]string{"comb in kit", "bag", "kit in bag"}},
		{"inside itself",
			[]string{"bag in bag", "socks in bag"},
			[]string{"bag", "socks in bag"}},
		// A circle is opened at its first row; the rest still hangs together.
		{"circle of two",
			[]string{"socks", "bag in kit", "kit in bag"},
			[]string{"socks", "bag", "kit in bag"}},
		{"circle of three",
			[]string{"car in kit", "bag in car", "kit in bag", "comb in kit"},
			[]string{"car", "bag in car", "kit in bag", "comb in kit"}},
		{"orphan and circle",
			[]string{"towel in ghost", "bag in kit", "kit in bag", "car", "lamp in car"},
			[]string{"towel", "bag", "kit in bag", "car", "lamp in car"}},
		// A repair can land a row on a placement that is taken: the first stays.
		{"repair makes a duplicate",
			[]string{"socks x2", "socks in ghost x5", "comb"},
			[]string{"socks x2", "comb"}},
		{"duplicate placements",
			[]string{"bag", "socks in bag x2", "socks in bag x3 pairs", "socks"},
			[]string{"bag", "socks in bag x2", "socks"}},
		// Compartments are left alone in a sound list...
		{"sound, with compartments",
			[]string{"car", "bag in car/boot", "socks in bag/lid", "kit in bag/pocket", "comb in kit/mesh"},
			[]string{"car", "bag in car/boot", "socks in bag/lid", "kit in bag/pocket", "comb in kit/mesh"}},
		// ...but a row that is moved to the top level is in no container, so
		// in no compartment; what hangs from it keeps its own.
		{"orphan in a compartment",
			[]string{"socks in ghost/lid", "bag", "comb in bag/pocket"},
			[]string{"socks", "bag", "comb in bag/pocket"}},
		{"orphaned container in a compartment",
			[]string{"comb in kit/mesh x2", "kit in ghost/boot", "socks in kit"},
			[]string{"comb in kit/mesh x2", "kit", "socks in kit"}},
		{"circle through compartments",
			[]string{"comb in kit/mesh", "bag in kit/mesh", "kit in bag/lid", "socks in bag/pocket"},
			[]string{"comb in kit/mesh", "bag", "kit in bag/lid", "socks in bag/pocket"}},
		{"inside itself, in a compartment",
			[]string{"bag in bag/lid", "socks in bag/lid"},
			[]string{"bag", "socks in bag/lid"}},
		// Same placement twice, in two compartments: the first stays.
		{"duplicate placements in two compartments",
			[]string{"bag", "socks in bag/lid x2", "socks in bag/pocket x3"},
			[]string{"bag", "socks in bag/lid x2"}},
		{"repair lands on a taken placement",
			[]string{"socks x2", "socks in ghost/lid x5"},
			[]string{"socks x2"}},
	}
	for _, c := range cases {
		got := sound(rows(c.in...))
		wantRows(t, c.name, got, c.want...)
		if err := checkTree(got); err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
		for _, e := range got {
			if e.ContainerID == nil && e.CompartmentID != nil {
				t.Errorf("%s: %s is at the top level and in a compartment", c.name, mergeNames[e.ItemID])
			}
		}
	}
}

// Merge repairs whatever it is given.
func TestMergeOfUnsoundLists(t *testing.T) {
	a := list("socks in ghost", "bag in kit", "kit in bag", "comb in kit")
	b := list("kit", "bag in kit", "socks", "comb in bag")
	wantRows(t, "a", merge(a), "socks", "bag", "kit in bag", "comb in kit")
	wantRows(t, "a+b", merge(a, b), "socks", "bag", "kit in bag", "comb in kit", "comb in bag")
	wantRows(t, "b+a", merge(b, a), "kit", "bag in kit", "socks", "comb in bag", "comb in kit")

	// Rows that are detached lose their compartment on the way.
	c := list("socks in ghost/lid", "bag in kit/mesh", "kit in bag/pocket", "comb in kit/mesh")
	wantRows(t, "c", merge(c), "socks", "bag", "kit in bag/pocket", "comb in kit/mesh")
	wantRows(t, "b+c", merge(b, c), "kit", "bag in kit", "socks", "comb in bag", "comb in kit/mesh")
}

// ---------------------------------------------------------------- properties

// checkTree reports the first way in which rows fail to be a tree: a
// container that is not among the rows, a placement that is there twice, or
// containers that hold one another in a circle.
func checkTree(rows []Entry) error {
	items := map[int64]bool{}
	for _, r := range rows {
		items[r.ItemID] = true
	}
	seen := map[place]bool{}
	containers := map[int64][]int64{} // item -> every container it is in
	for _, r := range rows {
		c := containerOf(r)
		if c != 0 && !items[c] {
			return fmt.Errorf("item %d is in container %d, which is not among the rows", r.ItemID, c)
		}
		if p := (place{c, r.ItemID}); seen[p] {
			return fmt.Errorf("item %d is in container %d twice", r.ItemID, c)
		} else {
			seen[p] = true
		}
		containers[r.ItemID] = append(containers[r.ItemID], c)
	}
	// Walking up from any row, by every route, must end at the top level.
	const onPath, done = 1, 2
	state := map[int64]int{}
	var up func(item int64) bool
	up = func(item int64) bool {
		if item == 0 || state[item] == done {
			return true
		}
		if state[item] == onPath {
			return false
		}
		state[item] = onPath
		for _, c := range containers[item] {
			if !up(c) {
				return false
			}
		}
		state[item] = done
		return true
	}
	for _, r := range rows {
		if !up(r.ItemID) {
			return fmt.Errorf("item %d is in a circle of containers", r.ItemID)
		}
	}
	return nil
}

// The random lists give container c the compartments 10c+1 and 10c+2.
// checkCompartments reports a row that is in a compartment without being in
// that compartment's container.
func checkCompartments(rows []Entry) error {
	for _, r := range rows {
		if r.CompartmentID != nil && *r.CompartmentID/10 != containerOf(r) {
			return fmt.Errorf("item %d is in container %d and in compartment %d, which is not one of that container's", r.ItemID, containerOf(r), *r.CompartmentID)
		}
	}
	return nil
}

// checkRules is checkTree and checkCompartments plus rules 1 and 2 of
// contents.go: only containers hold things, and a container is on the list
// once.
func checkRules(rows []Entry, isContainer map[int64]bool) error {
	if err := checkTree(rows); err != nil {
		return err
	}
	if err := checkCompartments(rows); err != nil {
		return err
	}
	times := map[int64]int{}
	for _, r := range rows {
		times[r.ItemID]++
		if c := containerOf(r); c != 0 && !isContainer[c] {
			return fmt.Errorf("item %d is inside %d, which is not a container", r.ItemID, c)
		}
		if r.Quantity < 1 {
			return fmt.Errorf("item %d has quantity %d", r.ItemID, r.Quantity)
		}
	}
	for item, n := range times {
		if isContainer[item] && n > 1 {
			return fmt.Errorf("container %d is on the list %d times", item, n)
		}
	}
	return nil
}

// The random lists draw on 6 containers (items 1 to 6) and 10 other items
// (7 to 16).
const (
	randContainers = 6
	randItems      = 16
)

var randIsContainer = func() map[int64]bool {
	m := map[int64]bool{}
	for id := int64(1); id <= randContainers; id++ {
		m[id] = true
	}
	return m
}()

func randomRow(r *rand.Rand, container, item int64) Entry {
	e := Entry{ItemID: item, Quantity: 1 + r.Int64N(5), StatusID: 1 + r.Int64N(3)}
	if container != 0 {
		e.ContainerID = &container
		if n := r.Int64N(4); n < 2 { // half the rows in a container are in one of its compartments
			e.CompartmentID = new(container*10 + 1 + n)
		}
	}
	if u := []string{"", "", "kg", "pairs"}[r.IntN(4)]; u != "" {
		e.Unit = &u
	}
	if r.IntN(4) == 0 {
		e.Notes = new(fmt.Sprintf("note %d", r.IntN(100)))
	}
	return e
}

// randomList makes a list that keeps the rules of contents.go, as every list
// in the database does: some of the containers, each once, nested in one
// another without a circle, and the other items scattered over them (some
// of them in several places). The rows are in no particular order.
func randomList(r *rand.Rand) Source {
	var rows []Entry
	places := []int64{0} // the top level and the containers placed so far
	for _, c := range r.Perm(randContainers) {
		if r.IntN(3) == 0 {
			continue
		}
		id := int64(c + 1)
		rows = append(rows, randomRow(r, places[r.IntN(len(places))], id))
		places = append(places, id)
	}
	for item := int64(randContainers + 1); item <= randItems; item++ {
		n := min(r.IntN(4), len(places))
		for _, p := range r.Perm(len(places))[:n] {
			rows = append(rows, randomRow(r, places[p], item))
		}
	}
	r.Shuffle(len(rows), func(i, j int) { rows[i], rows[j] = rows[j], rows[i] })
	return Source{IsTemplate: r.IntN(2) == 0, Rows: rows}
}

func itemSet(rows []Entry) map[int64]bool {
	m := map[int64]bool{}
	for _, r := range rows {
		m[r.ItemID] = true
	}
	return m
}

func copySources(in []Source) []Source {
	out := make([]Source, len(in))
	for i, s := range in {
		out[i] = Source{IsTemplate: s.IsTemplate, Rows: append([]Entry(nil), s.Rows...)}
	}
	return out
}

// Whatever sound lists go in, what comes out is a sound list holding exactly
// what is on any of them.
func TestMergeOfRandomListsIsATree(t *testing.T) {
	const defaultStatus = 9 // not a status any source row has
	for seed := range uint64(1500) {
		r := rand.New(rand.NewPCG(seed, 1))
		sources := make([]Source, 1+r.IntN(4))
		for i := range sources {
			sources[i] = randomList(r)
		}
		before := copySources(sources)
		for i, src := range sources {
			if err := checkRules(src.Rows, randIsContainer); err != nil {
				t.Fatalf("seed %d: the generator made an unsound list %d: %v", seed, i, err)
			}
		}

		got := Merge(sources, randIsContainer, defaultStatus)
		fail := func(format string, args ...any) {
			t.Helper()
			t.Errorf("seed %d, %d lists: %s\n sources %v\n got     %v", seed, len(sources), fmt.Sprintf(format, args...), before, got)
		}
		if !reflect.DeepEqual(sources, before) {
			t.Fatalf("seed %d: Merge changed its sources", seed)
		}
		if err := checkRules(got, randIsContainer); err != nil {
			fail("%v", err)
			continue
		}

		// The items: those of every source, and no others.
		want := map[int64]bool{}
		for _, src := range sources {
			for id := range itemSet(src.Rows) {
				want[id] = true
			}
		}
		if items := itemSet(got); !reflect.DeepEqual(items, want) {
			fail("items %v, want %v", items, want)
		}

		// The placements: every row that came out is a placement on some
		// source; every placement on the first list is there, and so is
		// every placement of something that is not a container, wherever it
		// was; each holds at least as many as any source counting in its unit.
		at := map[place]Entry{}
		for _, e := range got {
			at[place{containerOf(e), e.ItemID}] = e
		}
		onSources := map[place]bool{}
		firstCompartment := map[place]*int64{}       // the compartment a placement has on the first list that has it
		templateStatus := map[int64]map[int64]bool{} // item -> the statuses it has on templates
		for i, src := range sources {
			for _, r := range src.Rows {
				p := place{containerOf(r), r.ItemID}
				if !onSources[p] {
					firstCompartment[p] = r.CompartmentID
				}
				onSources[p] = true
				if src.IsTemplate {
					if templateStatus[r.ItemID] == nil {
						templateStatus[r.ItemID] = map[int64]bool{}
					}
					templateStatus[r.ItemID][r.StatusID] = true
				}
				e, ok := at[p]
				switch {
				case !ok && (i == 0 || !randIsContainer[r.ItemID]):
					fail("list %d's item %d in %d is missing", i, p.item, p.container)
				case ok && i == 0 && !sameUnit(e.Unit, r.Unit):
					fail("the first list's item %d in %d changed its unit", p.item, p.container)
				case ok && sameUnit(e.Unit, r.Unit) && e.Quantity < r.Quantity:
					fail("list %d's item %d in %d (x%d) came out as x%d", i, p.item, p.container, r.Quantity, e.Quantity)
				}
			}
		}
		for _, e := range got {
			p := place{containerOf(e), e.ItemID}
			if !onSources[p] {
				fail("item %d in %d is on no source", e.ItemID, containerOf(e))
			}
			if !sameID(e.CompartmentID, firstCompartment[p]) {
				fail("item %d in %d is not in the compartment the first list with it has it in", e.ItemID, containerOf(e))
			}
			if e.StatusID != defaultStatus && !templateStatus[e.ItemID][e.StatusID] {
				fail("item %d has status %d, which is neither the default nor a template's", e.ItemID, e.StatusID)
			}
		}

		if len(sources) == 1 {
			// A copy: the same rows in the same order, statuses aside.
			want := append([]Entry(nil), sources[0].Rows...)
			if !sources[0].IsTemplate {
				for i := range want {
					want[i].StatusID = defaultStatus
				}
			}
			if !reflect.DeepEqual(got, want) {
				fail("not a copy of the only source")
			}
		}
	}
}

// randomMess makes rows that break the rules every way a list of rows can
// while each container is still on the list at most once (as the unique
// placement of a container guarantees for anything Merge builds from sound
// lists): containers that are missing, inside themselves or in circles, and
// placements repeated.
func randomMess(r *rand.Rand) []Entry {
	inside := func() int64 { // anywhere: the top level, a container, a container that is not there
		return []int64{0, 0, 1, 2, 3, 4, 5, 6, 99}[r.IntN(9)]
	}
	var rows []Entry
	for id := int64(1); id <= randContainers; id++ {
		if r.IntN(3) != 0 {
			rows = append(rows, randomRow(r, inside(), id))
		}
	}
	for range r.IntN(8) {
		rows = append(rows, randomRow(r, inside(), int64(randContainers+1+r.IntN(4))))
	}
	r.Shuffle(len(rows), func(i, j int) { rows[i], rows[j] = rows[j], rows[i] })
	return rows
}

func TestSoundOfRandomRowsIsATree(t *testing.T) {
	for seed := range uint64(3000) {
		r := rand.New(rand.NewPCG(seed, 2))
		in := randomMess(r)
		before := append([]Entry(nil), in...)
		wasSound := checkTree(before) == nil

		got := sound(append([]Entry(nil), in...))
		if err := checkTree(got); err != nil {
			t.Errorf("seed %d: %v\n in  %v\n got %v", seed, err, before, got)
			continue
		}
		if err := checkCompartments(got); err != nil {
			t.Errorf("seed %d: %v\n in  %v\n got %v", seed, err, before, got)
		}
		if !reflect.DeepEqual(itemSet(got), itemSet(before)) {
			t.Errorf("seed %d: sound lost an item\n in  %v\n got %v", seed, before, got)
		}
		if wasSound && !reflect.DeepEqual(got, before) {
			t.Errorf("seed %d: sound changed rows that were a tree already\n in  %v\n got %v", seed, before, got)
		}
		if again := sound(append([]Entry(nil), got...)); !reflect.DeepEqual(again, got) {
			t.Errorf("seed %d: sound of its own output changed it\n first  %v\n second %v", seed, got, again)
		}

		// And through Merge, as the only source or with a sound list.
		for _, sources := range [][]Source{
			{{Rows: append([]Entry(nil), in...)}},
			{{Rows: append([]Entry(nil), in...)}, randomList(r)},
			{randomList(r), {Rows: append([]Entry(nil), in...)}},
		} {
			got := Merge(sources, randIsContainer, 1)
			if err := checkTree(got); err != nil {
				t.Errorf("seed %d: merge of %d lists: %v\n sources %v", seed, len(sources), err, sources)
			} else if err := checkCompartments(got); err != nil {
				t.Errorf("seed %d: merge of %d lists: %v\n sources %v", seed, len(sources), err, sources)
			}
		}
	}
}
