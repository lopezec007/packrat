package store_test

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"packrat/internal/store"
)

// Compartments: the parts of a container (a backpack's lid, a car's boot)
// and the list entries that say which of them they are in. The fixture
// (lists_test.go) has four: the bag's lid and pocket, the car's boot and the
// kit's mesh; the crate has none.

// compartmentNames lists the compartments as "item/name", in the order ListRef gives them.
func (f *fx) compartmentNames() []string {
	f.t.Helper()
	out := []string{}
	for _, r := range mustList(f.t, f.st, "compartments") {
		out = append(out, f.itemName[num(r["item_id"])]+"/"+r["name"].(string))
	}
	return out
}

// useCount is how many list entries are in a compartment.
func (f *fx) useCount(compartment int64) int64 {
	f.t.Helper()
	for _, r := range mustList(f.t, f.st, "compartments") {
		if rowID(r) == compartment {
			return num(r["use_count"])
		}
	}
	f.t.Fatalf("no compartment %d", compartment)
	return 0
}

// ---------------------------------------------------------------- the compartments themselves

func TestCompartmentCRUD(t *testing.T) {
	fx := newFx(t)
	ctx := context.Background()
	compartments := store.RefTableByKey("compartments")

	row := mustCreate(t, fx.st, "compartments", mustJSON(t, map[string]any{"item_id": fx.crate, "name": "  Top Tray "}))
	want := store.Row{"id": int32(5), "item_id": int32(fx.crate), "name": "Top Tray", "use_count": int32(0)}
	if !reflect.DeepEqual(row, want) {
		t.Fatalf("created %v, want %v", row, want)
	}
	tray := rowID(row)
	mustCreate(t, fx.st, "compartments", mustJSON(t, map[string]any{"item_id": fx.bag, "name": "A First"}))
	// Listed container by container, each one's in the order they were made
	// (not by name).
	all := []string{"car/boot", "bag/lid", "bag/pocket", "bag/A First", "kit/mesh", "crate/Top Tray"}
	if got := fx.compartmentNames(); !reflect.DeepEqual(got, all) {
		t.Fatalf("listed %v, want %v", got, all)
	}
	everything, err := fx.st.ListAllRef(ctx)
	if err != nil || len(everything["compartments"]) != 6 {
		t.Fatalf("ListAllRef: %d compartments, %v", len(everything["compartments"]), err)
	}

	// The name can change; the item it belongs to cannot, though a client
	// may send back the item it has (and the id and the count).
	if got := mustUpdate(t, fx.st, "compartments", tray, `{"name": " Tray "}`); got["name"] != "Tray" || num(got["item_id"]) != fx.crate {
		t.Fatalf("renamed %v", got)
	}
	if got := mustUpdate(t, fx.st, "compartments", tray, `{}`); got["name"] != "Tray" {
		t.Fatalf("empty update %v", got)
	}
	echo := mustJSON(t, map[string]any{"id": 77, "item_id": fx.crate, "name": "Top Tray", "use_count": 9})
	if got := mustUpdate(t, fx.st, "compartments", tray, echo); !reflect.DeepEqual(got, want) {
		t.Fatalf("echoed row %v, want %v", got, want)
	}
	if got := mustUpdate(t, fx.st, "compartments", tray, mustJSON(t, map[string]any{"item_id": fx.crate})); !reflect.DeepEqual(got, want) {
		t.Fatalf("the same item alone %v, want %v", got, want)
	}
	const fixed = "item_id cannot be changed once the compartment exists"
	for _, body := range []map[string]any{
		{"item_id": fx.bag},
		{"item_id": fx.bag, "name": "Moved"},
		{"item_id": fx.socks},
		{"item_id": 999},
	} {
		_, err := fx.st.UpdateRef(ctx, compartments, tray, fields(t, mustJSON(t, body)))
		wantValidation(t, fmt.Sprintf("update %v", body), err, fixed)
	}
	if got := fx.compartmentNames(); !reflect.DeepEqual(got, all) {
		t.Fatalf("after the refused updates %v, want %v", got, all)
	}

	if err := fx.st.DeleteRef(ctx, compartments, tray); err != nil {
		t.Fatal(err)
	}
	if got := fx.compartmentNames(); !reflect.DeepEqual(got, all[:5]) {
		t.Fatalf("after the delete %v", got)
	}
	wantNotFound(t, "second delete", fx.st.DeleteRef(ctx, compartments, tray))
	// Compartments.ID is an int, like Items.ID.
	for _, id := range []int64{tray, 999, 0, -1, pastSmallint, pastInt, -pastInt} {
		for _, body := range []string{`{"name": "Again"}`, `{}`, mustJSON(t, map[string]any{"item_id": fx.crate})} {
			_, err := fx.st.UpdateRef(ctx, compartments, id, fields(t, body))
			wantNotFound(t, fmt.Sprintf("update compartment %d %s", id, body), err)
		}
		wantNotFound(t, fmt.Sprintf("delete compartment %d", id), fx.st.DeleteRef(ctx, compartments, id))
	}
	// The crate is as it was, and can have the compartment again.
	mustCreate(t, fx.st, "compartments", mustJSON(t, map[string]any{"item_id": fx.crate, "name": "Top Tray"}))
}

func TestCompartmentValidation(t *testing.T) {
	fx := newFx(t)
	ctx := context.Background()
	compartments := store.RefTableByKey("compartments")
	bag, socks := fmt.Sprint(fx.bag), fmt.Sprint(fx.socks)

	cases := []struct{ body, mention string }{
		{`{}`, "item_id is required"},
		{`{"name": "Side"}`, "item_id is required"},
		{`{"item_id": null, "name": "Side"}`, "item_id is required"},
		{`{"item_id": ` + bag + `}`, "name is required"},
		{`{"item_id": ` + bag + `, "name": " "}`, "name is required"},
		{`{"item_id": ` + bag + `, "name": null}`, "name is required"},
		{`{"item_id": ` + bag + `, "name": 5}`, "name must be text"},
		{`{"item_id": ` + bag + `, "name": "` + strings.Repeat("n", 128) + `"}`, "name is too long (at most 127 characters)"},
		{`{"item_id": "` + bag + `", "name": "Side"}`, "item_id must be a whole number"},
		{`{"item_id": 2.5, "name": "Side"}`, "item_id must be a whole number"},
		{`{"item_id": 0, "name": "Side"}`, "item_id is out of range"},
		{`{"item_id": -1, "name": "Side"}`, "item_id is out of range"},
		{`{"item_id": 3000000000, "name": "Side"}`, "item_id is out of range"},
		{`{"item_id": ` + bag + `, "name": "Side", "list_id": 1}`, `unknown field "list_id" for compartments`},
		{`{"item_id": ` + bag + `, "name": "Side", "container_id": 1}`, `unknown field "container_id"`},
		// The item must be there, and must be a container.
		{`{"item_id": 999, "name": "Side"}`, "item_id: there is no item 999"},
		{`{"item_id": 40000, "name": "Side"}`, "item_id: there is no item 40000"},
		{`{"item_id": ` + socks + `, "name": "Side"}`, `"socks" is not a container: only a container has compartments`},
	}
	for _, c := range cases {
		_, err := fx.st.CreateRef(ctx, compartments, fields(t, c.body))
		wantValidation(t, "create "+c.body, err, c.mention)
	}
	updates := []struct{ body, mention string }{
		{`{"name": ""}`, "name is required"},
		{`{"name": null}`, "name is required"},
		{`{"name": "` + strings.Repeat("n", 128) + `"}`, "name is too long"},
		{`{"item_id": "` + bag + `"}`, "item_id must be a whole number"},
		{`{"item_id": 0}`, "item_id is out of range"},
		{`{"item_id": null}`, "item_id is required"},
		{`{"item": ` + bag + `}`, `unknown field "item"`},
	}
	for _, c := range updates {
		_, err := fx.st.UpdateRef(ctx, compartments, fx.lid, fields(t, c.body))
		wantValidation(t, "update "+c.body, err, c.mention)
	}
	if got, want := fx.compartmentNames(), []string{"car/boot", "bag/lid", "bag/pocket", "kit/mesh"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after the refused requests %v, want %v", got, want)
	}
	// 127 characters are fine.
	mustCreate(t, fx.st, "compartments", mustJSON(t, map[string]any{"item_id": fx.bag, "name": strings.Repeat("é", 127)}))
}

// A container's compartments have different names; two containers can each
// have a "lid".
func TestCompartmentDuplicateNames(t *testing.T) {
	fx := newFx(t)
	ctx := context.Background()
	compartments := store.RefTableByKey("compartments")
	const taken = "that container already has a compartment with that name"

	for _, name := range []string{"lid", "  lid ", "pocket"} {
		_, err := fx.st.CreateRef(ctx, compartments, fields(t, mustJSON(t, map[string]any{"item_id": fx.bag, "name": name})))
		wantConflict(t, "create a second "+name, err, taken)
	}
	_, err := fx.st.UpdateRef(ctx, compartments, fx.pocket, fields(t, `{"name": "lid"}`))
	wantConflict(t, "rename onto another of the bag's", err, taken)
	_, err = fx.st.UpdateRef(ctx, compartments, fx.pocket, fields(t, mustJSON(t, map[string]any{"item_id": fx.bag, "name": "lid"})))
	wantConflict(t, "rename onto another of the bag's, item echoed", err, taken)
	if got, want := fx.compartmentNames(), []string{"car/boot", "bag/lid", "bag/pocket", "kit/mesh"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after the refused requests %v, want %v", got, want)
	}

	// The same name on other containers, and a compartment's own name.
	for _, item := range []int64{fx.car, fx.kit, fx.crate} {
		mustCreate(t, fx.st, "compartments", mustJSON(t, map[string]any{"item_id": item, "name": "lid"}))
	}
	mustUpdate(t, fx.st, "compartments", fx.lid, `{"name": "lid"}`)
	mustUpdate(t, fx.st, "compartments", fx.mesh, `{"name": "pocket"}`)
	want := []string{"car/boot", "car/lid", "bag/lid", "bag/pocket", "kit/pocket", "kit/lid", "crate/lid"}
	if got := fx.compartmentNames(); !reflect.DeepEqual(got, want) {
		t.Fatalf("compartments %v, want %v", got, want)
	}
	// A name freed by a rename can be used again.
	mustUpdate(t, fx.st, "compartments", fx.lid, `{"name": "top"}`)
	mustUpdate(t, fx.st, "compartments", fx.pocket, `{"name": "lid"}`)
}

// use_count counts the entries in a compartment, on every list. Deleting a
// compartment that is in use leaves those entries in the container.
func TestCompartmentUseCountAndDeleteInUse(t *testing.T) {
	fx := newFx(t)
	ctx := context.Background()
	compartments := store.RefTableByKey("compartments")
	trip := fx.list("Trip", "car", "bag in car/boot", "socks in bag/lid x2 @packed #wool", "comb in bag/lid", "kit in bag/pocket", "towel in bag")
	other := fx.list("Other", "bag", "socks in bag/lid", "comb in bag/pocket")

	for name, want := range map[string]int64{"lid": 3, "pocket": 2, "boot": 1, "mesh": 0} {
		if got := fx.useCount(fx.compartmentID[name]); got != want {
			t.Errorf("use_count of %s = %d, want %d", name, got, want)
		}
	}
	// It comes back from an update too, and follows the lists.
	if got := mustUpdate(t, fx.st, "compartments", fx.lid, `{"name": "Lid Pocket"}`); num(got["use_count"]) != 3 {
		t.Errorf("use_count in the updated row: %v", got)
	}
	fx.patch(fx.row(trip, "comb in bag").ID, `{"compartment_id": null}`)
	fx.del(fx.row(other, "socks in bag").ID, false)
	if got := fx.useCount(fx.lid); got != 1 {
		t.Errorf("use_count of the lid after two entries left it = %d, want 1", got)
	}

	// Deleting the pocket: what was in it is still in the bag, on both
	// lists, with everything else as it was.
	if err := fx.st.DeleteRef(ctx, compartments, fx.pocket); err != nil {
		t.Fatalf("delete a compartment in use: %v", err)
	}
	fx.want(trip, "car", "bag in car/boot", "socks in bag/lid x2 @packed #wool", "comb in bag", "kit in bag", "towel in bag")
	fx.want(other, "bag", "comb in bag")
	if got, want := fx.compartmentNames(), []string{"car/boot", "bag/Lid Pocket", "kit/mesh"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("compartments %v, want %v", got, want)
	}
	if a, b := fx.useCount(fx.lid), fx.useCount(fx.boot); a != 1 || b != 1 {
		t.Errorf("use counts of the others changed: lid %d, boot %d", a, b)
	}
	wantNotFound(t, "second delete", fx.st.DeleteRef(ctx, compartments, fx.pocket))
	// The deleted compartment cannot be used any more.
	_, err := fx.patchErr(fx.row(trip, "towel in bag").ID, mustJSON(t, map[string]any{"compartment_id": fx.pocket}))
	wantValidation(t, "use a deleted compartment", err, fmt.Sprintf("compartment_id: there is no compartment %d", fx.pocket))
}

// A compartment is part of its item: it goes when the item does. (An item on
// a list cannot be deleted, compartments or not.)
func TestDeleteItemWithCompartments(t *testing.T) {
	fx := newFx(t)
	ctx := context.Background()
	items := store.RefTableByKey("items")
	trip := fx.list("Trip", "bag", "socks in bag/lid", "kit")

	wantConflict(t, "delete a container whose compartment is in use", fx.st.DeleteRef(ctx, items, fx.bag), "take it off every list before deleting it")
	wantConflict(t, "delete a container that is on a list, empty", fx.st.DeleteRef(ctx, items, fx.kit), "this item is on a list")
	if got, want := fx.compartmentNames(), []string{"car/boot", "bag/lid", "bag/pocket", "kit/mesh"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after the refused deletes %v, want %v", got, want)
	}
	fx.want(trip, "bag", "socks in bag/lid", "kit")

	// The car is on no list.
	if err := fx.st.DeleteRef(ctx, items, fx.car); err != nil {
		t.Fatalf("delete an item with a compartment: %v", err)
	}
	if got, want := fx.compartmentNames(), []string{"bag/lid", "bag/pocket", "kit/mesh"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after deleting the car %v, want %v", got, want)
	}
	// Nor is the bag, once the list is gone.
	if err := fx.st.DeleteList(ctx, trip); err != nil {
		t.Fatal(err)
	}
	if err := fx.st.DeleteRef(ctx, items, fx.bag); err != nil {
		t.Fatalf("delete the bag once it is off the list: %v", err)
	}
	if got, want := fx.compartmentNames(), []string{"kit/mesh"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after deleting the bag %v, want %v", got, want)
	}
	if n := fx.count(`SELECT count(*) FROM Compartments`); n != 1 {
		t.Fatalf("%d compartments left, want 1", n)
	}
}

// An item with compartments can stop being a container while nothing is
// inside it on any list. Its compartments stay (it can get no new ones), and
// are there again when it becomes a container once more.
func TestContainerFlagWithCompartments(t *testing.T) {
	fx := newFx(t)
	ctx := context.Background()
	compartments := store.RefTableByKey("compartments")
	trip := fx.list("Trip", "bag", "socks in bag/lid", "kit")
	flag := func(item int64, v bool) error {
		_, err := fx.st.UpdateRef(ctx, store.RefTableByKey("items"), item, fields(t, mustJSON(t, map[string]any{"is_container": v})))
		return err
	}
	all := []string{"car/boot", "bag/lid", "bag/pocket", "kit/mesh"}

	// Things in its compartments are things inside it: the rule is the old one.
	wantConflict(t, "unset while something is in a compartment", flag(fx.bag, false), `this item holds other items on the list "Trip"`)

	// The kit is on the list, empty; the car is on none.
	for _, item := range []int64{fx.kit, fx.car} {
		if err := flag(item, false); err != nil {
			t.Fatalf("unset %s: %v", fx.itemName[item], err)
		}
	}
	if got := fx.compartmentNames(); !reflect.DeepEqual(got, all) {
		t.Fatalf("compartments after the flags went off %v, want %v", got, all)
	}
	// What is no container gets no new compartments and holds nothing, in a
	// compartment or out of one; the compartments it has can still be
	// renamed and deleted.
	_, err := fx.st.CreateRef(ctx, compartments, fields(t, mustJSON(t, map[string]any{"item_id": fx.kit, "name": "side"})))
	wantValidation(t, "new compartment for a former container", err, `"kit" is not a container: only a container has compartments`)
	_, err = fx.addErr(trip, "comb in kit/mesh")
	wantValidation(t, "add into a former container's compartment", err, `"kit" is not a container`)
	if got := mustUpdate(t, fx.st, "compartments", fx.mesh, `{"name": "net"}`); got["name"] != "net" {
		t.Fatalf("rename a former container's compartment: %v", got)
	}
	if err := fx.st.DeleteRef(ctx, compartments, fx.boot); err != nil {
		t.Fatalf("delete a former container's compartment: %v", err)
	}

	// A container again: the compartment is still its own.
	if err := flag(fx.kit, true); err != nil {
		t.Fatal(err)
	}
	if c := fx.add(trip, "comb in kit/mesh"); c.CompartmentID == nil || *c.CompartmentID != fx.mesh {
		t.Fatalf("into the compartment of a container again: %+v", c)
	}
	if got, want := fx.compartmentNames(), []string{"bag/lid", "bag/pocket", "kit/net"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("compartments %v, want %v", got, want)
	}
}

// ---------------------------------------------------------------- entries in compartments

func TestAddContentInCompartment(t *testing.T) {
	fx := newFx(t)
	ctx := context.Background()
	trip := fx.list("Trip", "car", "bag in car")
	add := func(body map[string]any) (*store.Content, error) {
		return fx.st.AddContent(ctx, trip, fields(t, mustJSON(t, body)))
	}

	c, err := add(map[string]any{"item_id": fx.socks, "container_id": fx.bag, "compartment_id": fx.lid, "quantity": 2})
	want := &store.Content{ID: 3, ListID: trip, ContainerID: &fx.bag, CompartmentID: &fx.lid, ItemID: fx.socks, Quantity: 2, StatusID: fx.stored}
	if err != nil || !reflect.DeepEqual(c, want) {
		t.Fatalf("added %+v, %v; want %+v", c, err, want)
	}
	// Null is no compartment, as is leaving it out.
	if c, err = add(map[string]any{"item_id": fx.comb, "container_id": fx.bag, "compartment_id": nil}); err != nil || c.CompartmentID != nil {
		t.Fatalf("added with a null compartment %+v, %v", c, err)
	}
	if c, err = add(map[string]any{"item_id": fx.lamp, "compartment_id": nil}); err != nil || c.CompartmentID != nil || c.ContainerID != nil {
		t.Fatalf("added at the top level with a null compartment %+v, %v", c, err)
	}
	// A container can be in a compartment, and have things in its own.
	fx.add(trip, "kit in bag/pocket")
	fx.add(trip, "towel in kit/mesh")
	lines := []string{"car", "bag in car", "socks in bag/lid x2", "comb in bag", "lamp", "kit in bag/pocket", "towel in kit/mesh"}
	fx.want(trip, lines...)

	const atTop = "an entry at the top level of a list is in no container, so it cannot be in a compartment"
	bad := []struct {
		body    map[string]any
		mention string
	}{
		// Never at the top level.
		{map[string]any{"item_id": fx.towel, "compartment_id": fx.lid}, atTop},
		{map[string]any{"item_id": fx.towel, "container_id": nil, "compartment_id": fx.boot}, atTop},
		{map[string]any{"item_id": fx.towel, "compartment_id": 999}, atTop},
		// It must exist...
		{map[string]any{"item_id": fx.towel, "container_id": fx.bag, "compartment_id": 999}, "compartment_id: there is no compartment 999"},
		{map[string]any{"item_id": fx.towel, "container_id": fx.bag, "compartment_id": 0}, "compartment_id: there is no compartment 0"},
		{map[string]any{"item_id": fx.towel, "container_id": fx.bag, "compartment_id": -1}, "compartment_id: there is no compartment -1"},
		{map[string]any{"item_id": fx.towel, "container_id": fx.bag, "compartment_id": pastInt}, "compartment_id: there is no compartment 3000000000"},
		// ...and be one of the container's.
		{map[string]any{"item_id": fx.towel, "container_id": fx.bag, "compartment_id": fx.boot}, `that compartment is not one of "bag"'s`},
		{map[string]any{"item_id": fx.towel, "container_id": fx.car, "compartment_id": fx.lid}, `that compartment is not one of "car"'s`},
		{map[string]any{"item_id": fx.towel, "container_id": fx.bag, "compartment_id": fx.mesh}, `that compartment is not one of "bag"'s`},
		{map[string]any{"item_id": fx.towel, "container_id": fx.bag, "compartment_id": "lid"}, "compartment_id must be a whole number"},
		{map[string]any{"item_id": fx.towel, "container_id": fx.bag, "compartment_id": 1.5}, "compartment_id must be a whole number"},
		// What is wrong with the container comes first.
		{map[string]any{"item_id": fx.towel, "container_id": fx.socks, "compartment_id": fx.lid}, `"socks" is not a container`},
		{map[string]any{"item_id": fx.towel, "container_id": fx.crate, "compartment_id": fx.lid}, `"crate" is not on this list`},
	}
	for _, c := range bad {
		_, err := add(c.body)
		wantValidation(t, fmt.Sprintf("add %v", c.body), err, c.mention)
	}

	// The compartment does not make a placement a different one: the socks
	// are in the bag once, whichever compartment.
	const inBag = `"socks" is already in "bag" on this list: change its quantity instead`
	for _, line := range []string{"socks in bag/pocket", "socks in bag/lid", "socks in bag"} {
		_, err := fx.addErr(trip, line)
		wantConflict(t, "add "+line, err, inBag)
	}
	_, err = fx.addErr(trip, "comb in bag/lid")
	wantConflict(t, "add into a compartment what is in the container, in none", err, `"comb" is already in "bag" on this list`)
	fx.want(trip, lines...)

	// In another container, or on another list, they are welcome.
	fx.add(trip, "socks in car/boot")
	other := fx.list("Other", "bag", "socks in bag/pocket")
	fx.want(other, "bag", "socks in bag/pocket")
	fx.want(trip, append(lines, "socks in car/boot")...)
}

func TestUpdateContentCompartment(t *testing.T) {
	fx := newFx(t)
	trip := fx.list("Trip", "car", "bag in car/boot", "socks in bag/lid x2 pairs @packed #wool", "kit in bag/pocket", "comb in kit/mesh", "towel", "crate")
	row := fx.row(trip, "socks in bag")
	id := row.ID
	// step patches the socks and checks the whole row, returned and stored.
	want := row
	step := func(what string, body map[string]any, container, compartment *int64) {
		t.Helper()
		want.ContainerID, want.CompartmentID = container, compartment
		got, err := fx.patchErr(id, mustJSON(t, body))
		if err != nil || !reflect.DeepEqual(*got, want) {
			t.Fatalf("%s:\n got  %s (%+v), %v\n want %s", what, fx.line(*got), got, err, fx.line(want))
		}
		if stored := fx.get(trip).Contents[2]; !reflect.DeepEqual(stored, want) {
			t.Fatalf("%s: stored %s, want %s", what, fx.line(stored), fx.line(want))
		}
	}
	bag, car, kit, crate := &fx.bag, &fx.car, &fx.kit, &fx.crate
	lid, pocket, boot, mesh := &fx.lid, &fx.pocket, &fx.boot, &fx.mesh

	// A key that is absent leaves the compartment alone.
	want.Quantity = 3
	step("quantity only", map[string]any{"quantity": 3}, bag, lid)
	step("nothing", map[string]any{}, bag, lid)
	// compartment_id alone changes or clears it within the container.
	step("to the pocket", map[string]any{"compartment_id": fx.pocket}, bag, pocket)
	step("to the pocket again", map[string]any{"compartment_id": fx.pocket}, bag, pocket)
	step("to none", map[string]any{"compartment_id": nil}, bag, nil)
	step("to none again", map[string]any{"compartment_id": nil}, bag, nil)
	step("from none to the lid", map[string]any{"compartment_id": fx.lid}, bag, lid)
	// Naming the container it is in is not a move: the compartment stays
	// unless the same request changes it.
	step("the same container", map[string]any{"container_id": fx.bag}, bag, lid)
	step("the same container, another compartment", map[string]any{"container_id": fx.bag, "compartment_id": fx.pocket}, bag, pocket)
	step("the same container, no compartment", map[string]any{"container_id": fx.bag, "compartment_id": nil}, bag, nil)
	step("back in the lid", map[string]any{"compartment_id": fx.lid}, bag, lid)

	// A move to another container takes the row out of its compartment...
	step("into the car", map[string]any{"container_id": fx.car}, car, nil)
	// ...unless one of the new container's is given along with it.
	step("into the bag's lid", map[string]any{"container_id": fx.bag, "compartment_id": fx.lid}, bag, lid)
	step("into the car's boot", map[string]any{"container_id": fx.car, "compartment_id": fx.boot}, car, boot)
	step("into the kit", map[string]any{"container_id": fx.kit}, kit, nil)
	step("into the mesh", map[string]any{"compartment_id": fx.mesh}, kit, mesh)
	step("into the crate, explicitly in no compartment", map[string]any{"container_id": fx.crate, "compartment_id": nil}, crate, nil)
	step("into the kit's mesh", map[string]any{"container_id": fx.kit, "compartment_id": fx.mesh}, kit, mesh)
	// To the top level: out of the container, out of its compartment.
	step("to the top level", map[string]any{"container_id": nil}, nil, nil)
	step("clearing what is clear", map[string]any{"compartment_id": nil}, nil, nil)
	step("to the top level again, both null", map[string]any{"container_id": nil, "compartment_id": nil}, nil, nil)
	const atTop = "an entry at the top level of a list is in no container, so it cannot be in a compartment"
	for _, body := range []map[string]any{
		{"compartment_id": fx.lid},
		{"container_id": nil, "compartment_id": fx.lid},
		{"compartment_id": 999},
	} {
		_, err := fx.patchErr(id, mustJSON(t, body))
		wantValidation(t, fmt.Sprintf("at the top level, patch %v", body), err, atTop)
	}
	// With other fields in the same request.
	want.Quantity, want.Notes = 4, new("thick")
	step("moved, counted and noted", map[string]any{"container_id": fx.bag, "compartment_id": fx.lid, "quantity": 4, "notes": "thick"}, bag, lid)

	bad := []struct {
		body    map[string]any
		mention string
	}{
		{map[string]any{"compartment_id": fx.boot}, `that compartment is not one of "bag"'s`},
		{map[string]any{"compartment_id": fx.mesh}, `that compartment is not one of "bag"'s`},
		{map[string]any{"compartment_id": 999}, "compartment_id: there is no compartment 999"},
		{map[string]any{"compartment_id": 0}, "compartment_id: there is no compartment 0"},
		{map[string]any{"compartment_id": pastInt}, "compartment_id: there is no compartment 3000000000"},
		{map[string]any{"compartment_id": "lid"}, "compartment_id must be a whole number"},
		// The compartment it is in is not one of the container it is moved to.
		{map[string]any{"container_id": fx.car, "compartment_id": fx.lid}, `that compartment is not one of "car"'s`},
		{map[string]any{"container_id": fx.crate, "compartment_id": fx.lid}, `that compartment is not one of "crate"'s`},
		{map[string]any{"container_id": fx.bag, "compartment_id": fx.boot}, `that compartment is not one of "bag"'s`},
		{map[string]any{"container_id": nil, "compartment_id": fx.lid}, atTop},
		// What is wrong with the move comes first.
		{map[string]any{"container_id": fx.towel, "compartment_id": fx.lid}, `"towel" is not a container`},
		// One bad field refuses the whole change.
		{map[string]any{"compartment_id": fx.pocket, "quantity": 0}, "quantity must be"},
		{map[string]any{"container_id": fx.car, "compartment_id": fx.boot, "status_id": 99}, "there is no status 99"},
		{map[string]any{"quantity": 9, "compartment_id": fx.boot}, `that compartment is not one of "bag"'s`},
	}
	for _, c := range bad {
		_, err := fx.patchErr(id, mustJSON(t, c.body))
		wantValidation(t, fmt.Sprintf("patch %v", c.body), err, c.mention)
	}
	step("after the refused changes", map[string]any{}, bag, lid)

	// The unique rule is the old one: socks cannot be moved into the bag
	// while socks are in it, whatever the compartments.
	loose := fx.add(trip, "socks in car/boot")
	for _, body := range []map[string]any{
		{"container_id": fx.bag},
		{"container_id": fx.bag, "compartment_id": fx.pocket},
		{"container_id": fx.bag, "compartment_id": nil},
	} {
		_, err := fx.patchErr(loose.ID, mustJSON(t, body))
		wantConflict(t, fmt.Sprintf("patch %v", body), err, `"socks" is already in "bag" on this list`)
	}

	// A container that moves takes its contents along, each still in the
	// compartment of its own container; only the moved row loses its own.
	fx.want(trip, "car", "bag in car/boot", "socks in bag/lid x4 pairs @packed #thick", "kit in bag/pocket", "comb in kit/mesh", "towel", "crate", "socks in car/boot")
	fx.patch(fx.row(trip, "bag in car").ID, mustJSON(t, map[string]any{"container_id": fx.crate}))
	fx.want(trip, "car", "bag in crate", "socks in bag/lid x4 pairs @packed #thick", "kit in bag/pocket", "comb in kit/mesh", "towel", "crate", "socks in car/boot")
	fx.patch(fx.row(trip, "kit in bag").ID, mustJSON(t, map[string]any{"container_id": fx.car, "compartment_id": fx.boot}))
	fx.want(trip, "car", "bag in crate", "socks in bag/lid x4 pairs @packed #thick", "kit in car/boot", "comb in kit/mesh", "towel", "crate", "socks in car/boot")
}

// Keeping a removed container's contents moves them up one level, out of
// the removed container's compartments (not into the compartment the
// container itself was in).
func TestDeleteContentKeepClearsCompartments(t *testing.T) {
	fx := newFx(t)
	trip := fx.list("Trip", "car", "bag in car/boot", "socks in bag/lid x2", "kit in bag/pocket", "comb in kit/mesh", "towel in bag", "lamp in car/boot")

	fx.del(fx.row(trip, "bag in car").ID, true)
	fx.want(trip, "car", "socks in car x2", "kit in car", "comb in kit/mesh", "towel in car", "lamp in car/boot")
	if got := fx.useCount(fx.lid) + fx.useCount(fx.pocket); got != 0 {
		t.Fatalf("%d entries are still in the removed bag's compartments", got)
	}

	// At the top level there is no container to be in a compartment of.
	day := fx.list("Day", "bag", "socks in bag/lid", "kit in bag/pocket", "comb in kit/mesh", "towel in bag")
	fx.del(fx.row(day, "bag").ID, true)
	fx.want(day, "socks", "kit", "comb in kit/mesh", "towel")

	// The clash rule ignores compartments, as the unique rule does.
	clash := fx.list("Clash", "car", "bag in car", "socks in bag/lid", "socks in car/boot")
	err := fx.st.DeleteContent(context.Background(), fx.row(clash, "bag in car").ID, true)
	wantConflict(t, "keep onto socks in another compartment", err, `the contents cannot be kept: "socks" is already in the place they would move to`)
	fx.want(clash, "car", "bag in car", "socks in bag/lid", "socks in car/boot")

	// Without keeping, the container goes with everything in it, compartments or not.
	fx.del(fx.row(clash, "bag in car").ID, false)
	fx.want(clash, "car", "socks in car/boot")
	fx.del(fx.row(trip, "car").ID, false)
	fx.want(trip)
	if got := fx.useCount(fx.boot) + fx.useCount(fx.mesh); got != 2 {
		t.Fatalf("%d entries in the boot and the mesh over all lists, want 2", got)
	}
}

// ---------------------------------------------------------------- lists made from lists

func TestCreateListFromSourcesWithCompartments(t *testing.T) {
	fx := newFx(t)
	ctx := context.Background()
	campLines := []string{"car", "bag in car/boot", "socks in bag/lid x2", "towel in bag/pocket @packed", "kit in bag", "comb in bag"}
	cityLines := []string{"bag", "socks in bag/pocket x5", "towel in bag", "comb in bag/lid x3", "kit in bag/lid", "lamp in kit/mesh", "socks"}
	camp := fx.template("Camping", campLines...)
	city := fx.list("City Break", cityLines...)

	cases := []struct {
		name    string
		sources []int64
		want    []string
	}{
		// A copy keeps every row in its compartment.
		{"Copy of a template", []int64{camp}, campLines},
		{"Copy of a list", []int64{city}, cityLines},
		// The same placement in different compartments is one row, in the
		// first list's compartment (or in none, if it has it in none); the
		// bag goes into the car's boot, where the first list has it, and
		// what the second packs into its compartments follows it.
		{"Both", []int64{camp, city},
			[]string{"car", "bag in car/boot", "socks in bag/lid x5", "towel in bag/pocket @packed", "kit in bag", "comb in bag x3",
				"lamp in kit/mesh", "socks"}},
		{"Both, city first", []int64{city, camp},
			[]string{"bag", "socks in bag/pocket x5", "towel in bag", "comb in bag/lid x3", "kit in bag/lid", "lamp in kit/mesh", "socks",
				"car"}},
	}
	for _, c := range cases {
		p, err := fx.st.PreviewMerge(ctx, c.sources)
		if err != nil || p.Rows != len(c.want) {
			t.Errorf("%s: preview %+v, %v; want %d rows", c.name, p, err, len(c.want))
		}
		d, err := fx.st.CreateList(ctx, store.NewList{Name: c.name, Sources: c.sources})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := fx.lines(d.Contents); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n got  %q\n want %q", c.name, got, c.want)
		}
		if again := fx.get(d.ID); !reflect.DeepEqual(again, d) {
			t.Errorf("%s: GetList differs from what CreateList returned:\n %+v\n %+v", c.name, again, d)
		}
	}
	fx.want(camp, campLines...)
	fx.want(city, cityLines...)
	// The new lists' entries count as uses of the compartments.
	for name, want := range map[string]int64{"boot": 3, "lid": 9, "pocket": 6, "mesh": 4} {
		if got := fx.useCount(fx.compartmentID[name]); got != want {
			t.Errorf("use_count of %s = %d, want %d", name, got, want)
		}
	}
}
