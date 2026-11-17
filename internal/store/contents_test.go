package store_test

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"packrat/internal/store"
)

// ---------------------------------------------------------------- add

func TestAddContentDefaults(t *testing.T) {
	fx := newFx(t)
	ctx := context.Background()
	trip := fx.list("Trip")

	// Only the item: one of it, at the top level, with the first status.
	c, err := fx.st.AddContent(ctx, trip, fields(t, mustJSON(t, map[string]any{"item_id": fx.socks})))
	if err != nil {
		t.Fatal(err)
	}
	want := &store.Content{ID: 1, ListID: trip, ItemID: fx.socks, Quantity: 1, StatusID: fx.stored}
	if !reflect.DeepEqual(c, want) {
		t.Fatalf("added %+v, want %+v", c, want)
	}
	// Explicit nulls say the same.
	c, err = fx.st.AddContent(ctx, trip, fields(t, mustJSON(t, map[string]any{
		"item_id": fx.comb, "container_id": nil, "unit": nil, "status_id": nil, "notes": nil,
	})))
	want = &store.Content{ID: 2, ListID: trip, ItemID: fx.comb, Quantity: 1, StatusID: fx.stored}
	if err != nil || !reflect.DeepEqual(c, want) {
		t.Fatalf("added with nulls %+v, %v; want %+v", c, err, want)
	}
	if got := fx.get(trip); !reflect.DeepEqual(got.Contents, []store.Content{
		{ID: 1, ListID: trip, ItemID: fx.socks, Quantity: 1, StatusID: fx.stored},
		{ID: 2, ListID: trip, ItemID: fx.comb, Quantity: 1, StatusID: fx.stored},
	}) {
		t.Fatalf("the list holds %+v", got.Contents)
	}
}

// The status a new row starts with is the lowest-numbered one there is.
func TestAddContentDefaultStatus(t *testing.T) {
	fx := newFx(t)
	ctx := context.Background()
	statuses := store.RefTableByKey("statuses")
	trip := fx.list("Trip")

	// A status made later does not take over, whatever its name.
	mustCreate(t, fx.st, "statuses", `{"name": "A First", "color": "#000000", "shortcut": "a"}`)
	if c := fx.add(trip, "socks"); c.StatusID != fx.stored {
		t.Fatalf("status %d, want %d", c.StatusID, fx.stored)
	}
	fx.del(fx.row(trip, "socks").ID, false)

	if err := fx.st.DeleteRef(ctx, statuses, fx.stored); err != nil {
		t.Fatal(err)
	}
	if c := fx.add(trip, "socks"); c.StatusID != fx.packed {
		t.Fatalf("status %d once the first is gone, want %d", c.StatusID, fx.packed)
	}
	if c := fx.add(trip, "comb @missing"); c.StatusID != fx.missing {
		t.Fatalf("an explicit status: %d", c.StatusID)
	}

	// With no statuses at all nothing can be added, and the message says why.
	fx.del(fx.row(trip, "socks").ID, false)
	fx.del(fx.row(trip, "comb").ID, false)
	for _, r := range mustList(t, fx.st, "statuses") {
		if err := fx.st.DeleteRef(ctx, statuses, rowID(r)); err != nil {
			t.Fatal(err)
		}
	}
	_, err := fx.addErr(trip, "socks")
	wantConflict(t, "add with no statuses", err, "there are no statuses yet")
	fx.want(trip)
}

func TestAddContentFields(t *testing.T) {
	fx := newFx(t)
	ctx := context.Background()
	trip := fx.list("Trip", "bag")
	add := func(body map[string]any) (*store.Content, error) {
		return fx.st.AddContent(ctx, trip, fields(t, mustJSON(t, body)))
	}

	// Everything given; text is trimmed.
	c, err := add(map[string]any{"item_id": fx.socks, "container_id": fx.bag, "quantity": 3, "unit": " pairs ", "status_id": fx.packed, "notes": "  wool ones\n"})
	want := &store.Content{ID: 2, ListID: trip, ContainerID: &fx.bag, ItemID: fx.socks, Quantity: 3, Unit: new("pairs"), StatusID: fx.packed, Notes: new("wool ones")}
	if err != nil || !reflect.DeepEqual(c, want) {
		t.Fatalf("added %+v, %v; want %+v", c, err, want)
	}
	// Empty and blank text is no text.
	c, err = add(map[string]any{"item_id": fx.comb, "unit": "", "notes": "   "})
	if err != nil || c.Unit != nil || c.Notes != nil {
		t.Fatalf("blank unit and notes should be null: %+v, %v", c, err)
	}
	if got := fx.get(trip).Contents[2]; got.Unit != nil || got.Notes != nil {
		t.Fatalf("blank unit and notes were stored: %+v", got)
	}

	// The limits: 127 characters of unit, 255 of notes, the largest int of quantity.
	c, err = add(map[string]any{"item_id": fx.lamp, "unit": strings.Repeat("ü", 127), "notes": strings.Repeat("ñ", 255), "quantity": 2147483647})
	if err != nil || *c.Unit != strings.Repeat("ü", 127) || *c.Notes != strings.Repeat("ñ", 255) || c.Quantity != 2147483647 {
		t.Fatalf("at the limits: %+v, %v", c, err)
	}
	_, err = add(map[string]any{"item_id": fx.towel, "unit": strings.Repeat("u", 128)})
	wantValidation(t, "unit of 128", err, "unit is too long (at most 127 characters)")
	_, err = add(map[string]any{"item_id": fx.towel, "notes": strings.Repeat("n", 256)})
	wantValidation(t, "notes of 256", err, "notes is too long (at most 255 characters)")
	_, err = add(map[string]any{"item_id": fx.towel, "quantity": 2147483648})
	wantValidation(t, "quantity past int", err, "quantity must be a whole number from 1 up")
	fx.want(trip, "bag", "socks in bag x3 pairs @packed #wool ones", "comb",
		"lamp x2147483647 "+strings.Repeat("ü", 127)+" #"+strings.Repeat("ñ", 255))
}

func TestAddContentValidation(t *testing.T) {
	fx := newFx(t)
	ctx := context.Background()
	trip := fx.list("Trip", "bag")
	socks := fmt.Sprint(fx.socks)

	cases := []struct{ body, mention string }{
		{`{}`, "item_id is required"},
		{`{"item_id": null}`, "item_id is required"},
		{`{"quantity": 2}`, "item_id is required"},
		{`{"item_id": "` + socks + `"}`, "item_id must be a whole number"},
		{`{"item_id": 5.5}`, "item_id must be a whole number"},
		{`{"item_id": 999}`, "item_id: there is no item 999"},
		{`{"item_id": 0}`, "item_id: there is no item 0"},
		{`{"item_id": -1}`, "item_id: there is no item -1"},
		{`{"item_id": 3000000000}`, "item_id: there is no item 3000000000"},
		{`{"item_id": ` + socks + `, "container_id": "bag"}`, "container_id must be a whole number"},
		{`{"item_id": ` + socks + `, "container_id": 999}`, "container_id: there is no item 999"},
		{`{"item_id": ` + socks + `, "container_id": 0}`, "container_id: there is no item 0"},
		{`{"item_id": ` + socks + `, "container_id": -3000000000}`, "container_id: there is no item -3000000000"},
		{`{"item_id": ` + socks + `, "quantity": 0}`, "quantity must be a whole number from 1 up"},
		{`{"item_id": ` + socks + `, "quantity": -3}`, "quantity must be a whole number from 1 up"},
		{`{"item_id": ` + socks + `, "quantity": null}`, "quantity must be a whole number from 1 up"},
		{`{"item_id": ` + socks + `, "quantity": 1.5}`, "quantity must be a whole number"},
		{`{"item_id": ` + socks + `, "quantity": "2"}`, "quantity must be a whole number"},
		{`{"item_id": ` + socks + `, "unit": 5}`, "unit must be text"},
		{`{"item_id": ` + socks + `, "notes": ["a"]}`, "notes must be text"},
		{`{"item_id": ` + socks + `, "status_id": 99}`, "status_id: there is no status 99"},
		{`{"item_id": ` + socks + `, "status_id": 0}`, "status_id: there is no status 0"},
		{`{"item_id": ` + socks + `, "status_id": 40000}`, "status_id: there is no status 40000"},
		{`{"item_id": ` + socks + `, "status_id": "p"}`, "status_id must be a whole number"},
		{`{"item_id": ` + socks + `, "list_id": 2}`, `unknown field "list_id" for a list entry`},
		{`{"item_id": ` + socks + `, "id": 7}`, `unknown field "id"`},
		{`{"item_id": ` + socks + `, "note": "x"}`, `unknown field "note"`},
		{`{"item_id": ` + socks + `, "notes": "a\u0000b"}`, "list entry"},
	}
	for _, c := range cases {
		_, err := fx.st.AddContent(ctx, trip, fields(t, c.body))
		wantValidation(t, "add "+c.body, err, c.mention)
	}
	fx.want(trip, "bag")

	// A list that is not there; but a bad body is refused first.
	for _, list := range []int64{999, 0, -1, pastSmallint, pastInt} {
		_, err := fx.st.AddContent(ctx, list, fields(t, `{"item_id": `+socks+`}`))
		wantNotFound(t, fmt.Sprintf("add to list %d", list), err)
	}
	_, err := fx.st.AddContent(ctx, 999, fields(t, `{}`))
	wantValidation(t, "bad body for a missing list", err, "item_id is required")
	if n := fx.count(`SELECT count(*) FROM List_Contents`); n != 1 {
		t.Fatalf("%d rows after the refused adds, want 1", n)
	}
}

// ---------------------------------------------------------------- the three rules

// Rule 1: a row's container is a container item that is itself on the same list.
func TestContentsRule1ContainerIsAContainerOnTheList(t *testing.T) {
	fx := newFx(t)
	trip := fx.list("Trip", "bag", "socks", "comb in bag")
	fx.list("Other", "kit")

	const notContainer = `"socks" is not a container: mark it as one in the Library first`
	const notHere = `is not on this list, so nothing can be put inside it here`

	_, err := fx.addErr(trip, "lamp in socks")
	wantValidation(t, "add into a non-container", err, notContainer)
	_, err = fx.addErr(trip, "lamp in kit")
	wantValidation(t, "add into a container that is on another list", err, `"kit" `+notHere)
	_, err = fx.addErr(trip, "lamp in crate")
	wantValidation(t, "add into a container that is on no list", err, `"crate" `+notHere)
	_, err = fx.addErr(trip, "crate in crate")
	wantValidation(t, "add a container into itself", err, `"crate" cannot be put inside itself`)
	// The same on a move.
	comb, bag := fx.row(trip, "comb in bag"), fx.row(trip, "bag")
	_, err = fx.move(comb.ID, fx.socks)
	wantValidation(t, "move into a non-container", err, notContainer)
	_, err = fx.move(comb.ID, fx.kit)
	wantValidation(t, "move into a container that is on another list", err, `"kit" `+notHere)
	_, err = fx.move(bag.ID, fx.crate)
	wantValidation(t, "move into a container that is on no list", err, `"crate" `+notHere)
	_, err = fx.move(bag.ID, fx.bag)
	wantValidation(t, "move a container into itself", err, `"bag" cannot be put inside itself`)
	_, err = fx.move(comb.ID, 999)
	wantValidation(t, "move into an item that is not there", err, "container_id: there is no item 999")
	_, err = fx.move(comb.ID, pastInt)
	wantValidation(t, "move into an item that cannot be", err, "container_id: there is no item 3000000000")
	_, err = fx.patchErr(comb.ID, `{"container_id": "bag"}`)
	wantValidation(t, "move into a name", err, "container_id must be a whole number")
	fx.want(trip, "bag", "socks", "comb in bag")

	// Once the kit is on this list too, things can go into it.
	fx.add(trip, "kit in bag")
	fx.add(trip, "lamp in kit")
	if _, err = fx.move(comb.ID, fx.kit); err != nil {
		t.Fatalf("move into a container on the list: %v", err)
	}
	fx.want(trip, "bag", "socks", "comb in kit", "kit in bag", "lamp in kit")
}

// Rule 2: a container is on a list at most once; anything else can be in
// several containers.
func TestContentsRule2ContainerOncePerList(t *testing.T) {
	fx := newFx(t)
	trip := fx.list("Trip", "car", "bag in car", "socks in bag", "kit")

	const once = `is a container and is already on this list: a container can only be in one place, so move it instead`
	for _, line := range []string{"bag", "bag in car", "bag in kit", "bag x2 @packed", "car", "car in bag", "kit in bag", "kit"} {
		_, err := fx.addErr(trip, line)
		wantConflict(t, "add "+line, err, `"`+strings.Fields(line)[0]+`" `+once)
	}
	fx.want(trip, "car", "bag in car", "socks in bag", "kit")

	// Socks in the bag, in the car, in the kit and loose.
	fx.add(trip, "socks in car x2")
	fx.add(trip, "socks in kit x3")
	fx.add(trip, "socks x4")
	fx.want(trip, "car", "bag in car", "socks in bag", "kit", "socks in car x2", "socks in kit x3", "socks x4")

	// Another list has its own bag.
	other := fx.list("Other", "bag", "socks in bag")
	_, err := fx.addErr(other, "bag")
	wantConflict(t, "add a second bag to the other list", err, once)
	fx.want(other, "bag", "socks in bag")
}

func TestContentsDuplicatePlacement(t *testing.T) {
	fx := newFx(t)
	trip := fx.list("Trip", "bag", "socks in bag x2", "socks x3", "kit", "comb in kit")

	const atTop = `"socks" is already at the top level of this list: change its quantity instead`
	const inBag = `"socks" is already in "bag" on this list: change its quantity instead`
	_, err := fx.addErr(trip, "socks")
	wantConflict(t, "add at the top level again", err, atTop)
	_, err = fx.addErr(trip, "socks in bag x9 pairs @packed #more")
	wantConflict(t, "add into the bag again", err, inBag)
	// Moving one pile onto the other is the same conflict.
	_, err = fx.move(fx.row(trip, "socks").ID, fx.bag)
	wantConflict(t, "move onto the pile in the bag", err, inBag)
	_, err = fx.move(fx.row(trip, "socks in bag").ID, 0)
	wantConflict(t, "move onto the pile at the top level", err, atTop)
	fx.want(trip, "bag", "socks in bag x2", "socks x3", "kit", "comb in kit")

	// A different place, or another list, is free.
	fx.add(trip, "socks in kit")
	other := fx.list("Other", "bag")
	fx.add(other, "socks in bag")
	fx.add(other, "socks")
	if n := fx.count(`SELECT count(*) FROM List_Contents WHERE Item_ID = $1`, fx.socks); n != 5 {
		t.Fatalf("%d rows of socks, want 5", n)
	}
}

// Rule 3: containers do not hold one another in a circle.
func TestContentsRule3NoCircles(t *testing.T) {
	fx := newFx(t)
	trip := fx.list("Trip", "car", "bag in car", "kit in bag", "crate in kit", "socks in crate")
	lines := []string{"car", "bag in car", "kit in bag", "crate in kit", "socks in crate"}
	id := func(place string) int64 { return fx.row(trip, place).ID }

	refused := []struct {
		row     string
		into    int64
		mention string
	}{
		{"car", fx.car, `"car" cannot be put inside itself`},
		{"car", fx.bag, `"car" cannot be moved into "bag", which is inside it`},
		{"car", fx.kit, `"car" cannot be moved into "kit", which is inside it`},
		{"car", fx.crate, `"car" cannot be moved into "crate", which is inside it`},
		{"bag in car", fx.kit, `"bag" cannot be moved into "kit", which is inside it`},
		{"bag in car", fx.crate, `"bag" cannot be moved into "crate", which is inside it`},
		{"kit in bag", fx.crate, `"kit" cannot be moved into "crate", which is inside it`},
		{"kit in bag", fx.kit, `"kit" cannot be put inside itself`},
	}
	for _, c := range refused {
		_, err := fx.move(id(c.row), c.into)
		wantValidation(t, fmt.Sprintf("move %q into %s", c.row, fx.itemName[c.into]), err, c.mention)
	}
	fx.want(trip, lines...)

	// What is allowed depends on where things are now: with the crate out
	// of the kit, the kit can go into the crate, and so on round.
	for _, m := range []struct {
		row  string
		into int64
	}{{"crate in kit", fx.car}, {"kit in bag", fx.crate}, {"bag in car", fx.kit}} {
		if _, err := fx.move(id(m.row), m.into); err != nil {
			t.Fatalf("move %q into %s: %v", m.row, fx.itemName[m.into], err)
		}
	}
	fx.want(trip, "car", "bag in kit", "kit in crate", "crate in car", "socks in crate")
	_, err := fx.move(id("crate in car"), fx.bag)
	wantValidation(t, "the circle the other way round", err, `"crate" cannot be moved into "bag", which is inside it`)
	_, err = fx.move(id("car"), fx.bag)
	wantValidation(t, "the car into the bag", err, `"car" cannot be moved into "bag", which is inside it`)
	fx.want(trip, "car", "bag in kit", "kit in crate", "crate in car", "socks in crate")
}

// ---------------------------------------------------------------- move

func TestMoveContent(t *testing.T) {
	fx := newFx(t)
	trip := fx.list("Trip", "car", "bag in car x2 @packed #blue", "kit in bag", "socks in kit x2", "comb in bag", "crate", "socks in crate")
	bag := fx.row(trip, "bag in car")

	// A container takes what is inside it along: only its own row changes.
	c, err := fx.move(bag.ID, fx.crate)
	want := bag
	want.ContainerID = &fx.crate
	if err != nil || !reflect.DeepEqual(*c, want) {
		t.Fatalf("moved %+v, %v; want %+v", c, err, want)
	}
	fx.want(trip, "car", "bag in crate x2 @packed #blue", "kit in bag", "socks in kit x2", "comb in bag", "crate", "socks in crate")

	// To the top level with null.
	if c, err = fx.move(bag.ID, 0); err != nil || c.ContainerID != nil {
		t.Fatalf("to the top level: %+v, %v", c, err)
	}
	fx.want(trip, "car", "bag x2 @packed #blue", "kit in bag", "socks in kit x2", "comb in bag", "crate", "socks in crate")

	// Where it already is: nothing to do, at the top level or in a container.
	if c, err = fx.move(bag.ID, 0); err != nil || c.ContainerID != nil {
		t.Fatalf("to the top level again: %+v, %v", c, err)
	}
	kit := fx.row(trip, "kit in bag")
	if c, err = fx.move(kit.ID, fx.bag); err != nil || !reflect.DeepEqual(*c, kit) {
		t.Fatalf("into the bag again: %+v, %v", c, err)
	}

	// A move can come with other changes.
	c = fx.patch(kit.ID, mustJSON(t, map[string]any{"container_id": fx.car, "quantity": 2, "notes": "first aid", "status_id": fx.missing}))
	if c.ID != kit.ID || *c.ContainerID != fx.car || c.Quantity != 2 || *c.Notes != "first aid" || c.StatusID != fx.missing {
		t.Fatalf("moved and changed: %+v", c)
	}
	fx.want(trip, "car", "bag x2 @packed #blue", "kit in car x2 @missing #first aid", "socks in kit x2", "comb in bag", "crate", "socks in crate")

	// Something that is not a container moves on its own, to a free place.
	socks := fx.row(trip, "socks in kit")
	_, err = fx.move(socks.ID, fx.crate)
	wantConflict(t, "onto the socks in the crate", err, `"socks" is already in "crate" on this list`)
	if c, err = fx.move(socks.ID, fx.bag); err != nil || *c.ContainerID != fx.bag || c.Quantity != 2 {
		t.Fatalf("socks into the bag: %+v, %v", c, err)
	}
	fx.want(trip, "car", "bag x2 @packed #blue", "kit in car x2 @missing #first aid", "socks in bag x2", "comb in bag", "crate", "socks in crate")

	// A refused move takes the rest of the change with it.
	_, err = fx.patchErr(socks.ID, mustJSON(t, map[string]any{"container_id": fx.crate, "quantity": 50}))
	wantConflict(t, "refused move with a quantity", err, "is already in")
	_, err = fx.patchErr(socks.ID, mustJSON(t, map[string]any{"container_id": fx.car, "quantity": 0}))
	wantValidation(t, "move with a bad quantity", err, "quantity must be")
	fx.want(trip, "car", "bag x2 @packed #blue", "kit in car x2 @missing #first aid", "socks in bag x2", "comb in bag", "crate", "socks in crate")
}

// ---------------------------------------------------------------- update

func TestUpdateContentFields(t *testing.T) {
	fx := newFx(t)
	trip := fx.list("Trip", "bag", "socks in bag x2 pairs @packed #wool")
	row := fx.row(trip, "socks in bag")
	check := func(what string, got *store.Content, want store.Content) {
		t.Helper()
		if !reflect.DeepEqual(*got, want) {
			t.Fatalf("%s:\n got  %s (%+v)\n want %s (%+v)", what, fx.line(*got), *got, fx.line(want), want)
		}
		if stored := fx.row(trip, "socks in bag"); !reflect.DeepEqual(stored, want) {
			t.Fatalf("%s: stored %+v, want %+v", what, stored, want)
		}
	}

	// Each field on its own; what is not sent stays.
	want := row
	want.Quantity = 5
	check("quantity", fx.patch(row.ID, `{"quantity": 5}`), want)
	want.Unit = new("kg")
	check("unit", fx.patch(row.ID, `{"unit": " kg "}`), want)
	want.Notes = new("thick ones")
	check("notes", fx.patch(row.ID, `{"notes": "thick ones  "}`), want)
	want.StatusID = fx.missing
	check("status", fx.patch(row.ID, mustJSON(t, map[string]any{"status_id": fx.missing})), want)
	check("nothing", fx.patch(row.ID, `{}`), want)

	// "" and null both clear unit and notes.
	want.Unit = nil
	check("unit to empty", fx.patch(row.ID, `{"unit": ""}`), want)
	want.Notes = nil
	check("notes to null", fx.patch(row.ID, `{"notes": null}`), want)
	want.Unit, want.Notes = new("l"), new("n")
	check("set again", fx.patch(row.ID, `{"unit": "l", "notes": "n"}`), want)
	want.Unit, want.Notes = nil, nil
	check("unit to null, notes to blank", fx.patch(row.ID, `{"unit": null, "notes": "   "}`), want)

	// The limits.
	want.Unit, want.Notes, want.Quantity = new(strings.Repeat("ü", 127)), new(strings.Repeat("ñ", 255)), 2147483647
	check("at the limits", fx.patch(row.ID, mustJSON(t, map[string]any{"unit": *want.Unit, "notes": *want.Notes, "quantity": want.Quantity})), want)

	bad := []struct{ body, mention string }{
		{`{"unit": "` + strings.Repeat("u", 128) + `"}`, "unit is too long (at most 127 characters)"},
		{`{"notes": "` + strings.Repeat("n", 256) + `"}`, "notes is too long (at most 255 characters)"},
		{`{"unit": 5}`, "unit must be text"},
		{`{"notes": {}}`, "notes must be text"},
		{`{"notes": "a\u0000b"}`, "list entry"},
		{`{"quantity": 0}`, "quantity must be a whole number from 1 up"},
		{`{"quantity": -1}`, "quantity must be a whole number from 1 up"},
		{`{"quantity": null}`, "quantity must be a whole number from 1 up"},
		{`{"quantity": 2147483648}`, "quantity must be a whole number from 1 up"},
		{`{"quantity": 1.5}`, "quantity must be a whole number"},
		{`{"quantity": "many"}`, "quantity must be a whole number"},
		{`{"status_id": null}`, "status_id is required"},
		{`{"status_id": 99}`, "status_id: there is no status 99"},
		{`{"status_id": 40000}`, "status_id: there is no status 40000"},
		{`{"status_id": "p"}`, "status_id must be a whole number"},
		// The item cannot be changed, nor the list.
		{`{"item_id": 1}`, `unknown field "item_id" for a list entry`},
		{`{"list_id": 1}`, `unknown field "list_id"`},
		{`{"id": 1}`, `unknown field "id"`},
		// One bad field refuses the whole change.
		{`{"quantity": 7, "unit": "g", "notes": "x", "status_id": 99}`, "there is no status 99"},
		{`{"container_id": null, "quantity": 0}`, "quantity must be"},
	}
	for _, c := range bad {
		_, err := fx.patchErr(row.ID, c.body)
		wantValidation(t, "patch "+c.body, err, c.mention)
	}
	if stored := fx.row(trip, "socks in bag"); !reflect.DeepEqual(stored, want) {
		t.Fatalf("refused changes changed the row: %+v, want %+v", stored, want)
	}

	// A row that is not there; but an unknown field is refused first.
	for _, id := range []int64{999, 0, -1, pastInt} {
		_, err := fx.patchErr(id, `{"quantity": 2}`)
		wantNotFound(t, fmt.Sprintf("patch row %d", id), err)
	}
	_, err := fx.patchErr(999, `{"item_id": 1}`)
	wantValidation(t, "unknown field for a missing row", err, "unknown field")
	fx.del(row.ID, false)
	_, err = fx.patchErr(row.ID, `{"quantity": 2}`)
	wantNotFound(t, "patch a deleted row", err)
}

// ---------------------------------------------------------------- delete

func TestDeleteContentRemovesWhatIsInside(t *testing.T) {
	fx := newFx(t)
	ctx := context.Background()
	trip := fx.list("Trip",
		"car", "bag in car", "kit in bag", "crate in kit", "socks in crate", "comb in kit",
		"lamp in car", "towel", "socks in bag", "socks")
	other := fx.list("Other", "bag", "kit in bag", "socks in kit")

	// The bag goes with everything in it, at every depth; what was beside
	// it stays, and so does the bag on the other list.
	fx.del(fx.row(trip, "bag in car").ID, false)
	fx.want(trip, "car", "lamp in car", "towel", "socks")
	fx.want(other, "bag", "kit in bag", "socks in kit")

	// Something with nothing in it goes alone.
	fx.del(fx.row(trip, "lamp in car").ID, false)
	fx.want(trip, "car", "towel", "socks")
	fx.del(fx.row(trip, "car").ID, false)
	fx.want(trip, "towel", "socks")

	// From the top of a tree, the whole tree.
	fx.del(fx.row(other, "bag").ID, false)
	fx.want(other)

	towel := fx.row(trip, "towel").ID
	fx.del(towel, false)
	for _, keep := range []bool{false, true} {
		wantNotFound(t, "delete a deleted row", fx.st.DeleteContent(ctx, towel, keep))
		for _, id := range []int64{999, 0, -1, pastInt} {
			wantNotFound(t, fmt.Sprintf("delete row %d", id), fx.st.DeleteContent(ctx, id, keep))
		}
	}
	fx.want(trip, "socks")
}

func TestDeleteContentKeepingWhatIsInside(t *testing.T) {
	fx := newFx(t)
	trip := fx.list("Trip", "car", "bag in car", "kit in bag x2 #spare", "socks in kit", "comb in bag x3 @packed", "lamp in car", "towel", "crate in car")

	// What was directly in the bag is now in the car, as it was; what is
	// deeper stays in its own container.
	fx.del(fx.row(trip, "bag in car").ID, true)
	fx.want(trip, "car", "kit in car x2 #spare", "socks in kit", "comb in car x3 @packed", "lamp in car", "towel", "crate in car")

	// From the top level, the contents come up to the top level.
	fx.del(fx.row(trip, "car").ID, true)
	fx.want(trip, "kit x2 #spare", "socks in kit", "comb x3 @packed", "lamp", "towel", "crate")

	// With nothing inside there is nothing to keep.
	fx.del(fx.row(trip, "crate").ID, true)
	fx.del(fx.row(trip, "towel").ID, true)
	fx.want(trip, "kit x2 #spare", "socks in kit", "comb x3 @packed", "lamp")
}

func TestDeleteContentKeepConflict(t *testing.T) {
	fx := newFx(t)
	const clash = `the contents cannot be kept: "socks" is already in the place they would move to`

	// The socks in the bag cannot come up into the car: there are socks there.
	trip := fx.list("Trip", "car", "bag in car", "socks in bag x2", "comb in bag", "socks in car x3")
	lines := []string{"car", "bag in car", "socks in bag x2", "comb in bag", "socks in car x3"}
	err := fx.st.DeleteContent(context.Background(), fx.row(trip, "bag in car").ID, true)
	wantConflict(t, "keep into the car", err, clash)
	fx.want(trip, lines...)

	// The same at the top level.
	day := fx.list("Day", "bag", "socks in bag", "socks", "kit in bag", "lamp in kit")
	err = fx.st.DeleteContent(context.Background(), fx.row(day, "bag").ID, true)
	wantConflict(t, "keep at the top level", err, clash)
	fx.want(day, "bag", "socks in bag", "socks", "kit in bag", "lamp in kit")

	// With the other pile gone, the contents can be kept...
	fx.del(fx.row(trip, "socks in car").ID, false)
	fx.del(fx.row(trip, "bag in car").ID, true)
	fx.want(trip, "car", "socks in car x2", "comb in car")
	// ...and removing them with the container was never in question.
	fx.del(fx.row(day, "bag").ID, false)
	fx.want(day, "socks")
}

// Deleting one of several rows of the same item removes that row only.
func TestDeleteContentOfARepeatedItem(t *testing.T) {
	fx := newFx(t)
	trip := fx.list("Trip", "bag", "kit in bag", "socks in bag x2", "socks in kit x3", "socks x4", "comb in kit")
	other := fx.list("Other", "socks x5")

	fx.del(fx.row(trip, "socks in kit").ID, false)
	fx.want(trip, "bag", "kit in bag", "socks in bag x2", "socks x4", "comb in kit")
	fx.del(fx.row(trip, "socks").ID, true)
	fx.want(trip, "bag", "kit in bag", "socks in bag x2", "comb in kit")
	fx.del(fx.row(trip, "socks in bag").ID, false)
	fx.want(trip, "bag", "kit in bag", "comb in kit")
	fx.want(other, "socks x5")
}

// ---------------------------------------------------------------- at the same time

// Changes to one list do not interleave: of several attempts at once to put
// the same container on a list, in different places, exactly one succeeds.
func TestConcurrentAddsKeepAContainerOnce(t *testing.T) {
	fx := newFx(t)
	trip := fx.list("Trip", "car", "bag in car", "crate")
	lines := []string{"kit", "kit in car", "kit in bag", "kit in crate", "kit x2", "kit in car x3", "kit in bag x4", "kit in crate x5"}

	errs := make([]error, len(lines))
	var wg sync.WaitGroup
	for i, line := range lines {
		body := fields(t, mustJSON(t, fx.entry(line)))
		wg.Go(func() {
			_, errs[i] = fx.st.AddContent(context.Background(), trip, body)
		})
	}
	wg.Wait()

	added := 0
	for i, err := range errs {
		switch {
		case err == nil:
			added++
		case !isConflict(err):
			t.Errorf("add %q: want success or a conflict, got %v", lines[i], err)
		}
	}
	if n := fx.count(`SELECT count(*) FROM List_Contents WHERE List_ID = $1 AND Item_ID = $2`, trip, fx.kit); added != 1 || n != 1 {
		t.Fatalf("%d adds succeeded and the kit is on the list %d times; want 1 and 1", added, n)
	}
}
