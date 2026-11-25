package store

// How a new list is made from other lists. Pure functions over rows already
// read from the database (lists.go does the reading and writing), so the
// rules can be tested without one.
//
// The new list is the union of its sources: everything that is on any of
// them. With a single source that is a copy of it.
//
// A source's rows keep their status when the source is a template (spec.md:
// on a template, Status_ID "serves as the default status"); rows taken from
// an ordinary list start at the default status instead, since "Packed" on
// last year's trip says nothing about this year's.

// Entry is one List_Contents row without its own ID and list.
type Entry struct {
	ContainerID   *int64
	CompartmentID *int64 // a compartment of ContainerID's item, or nil
	ItemID        int64
	Quantity      int64
	Unit          *string
	StatusID      int64
	Notes         *string
}

// Source is one of the lists a new list is made from.
type Source struct {
	IsTemplate bool
	Rows       []Entry
}

// place is where a row sits: an item inside a container (0 = top level).
type place struct{ container, item int64 }

func containerOf(e Entry) int64 {
	if e.ContainerID == nil {
		return 0
	}
	return *e.ContainerID
}

func sameUnit(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// Merge combines the sources, in the order given, into the rows of a new
// list: every placement (an item in a container) found on any source.
//
//   - The same placement on several lists is one row, with the largest of
//     their quantities (when they count in the same unit) and the
//     compartment, unit, status and notes of the first list that has it.
//   - A container can only be in one place on a list, so it goes where the
//     first list that has it puts it; what later lists pack into it follows it
//     there.
//
// isContainer says which items are containers; defaultStatus is the status
// given to rows that come from a list that is not a template.
func Merge(sources []Source, isContainer map[int64]bool, defaultStatus int64) []Entry {
	var out []Entry
	at := map[place]int{}
	placed := map[int64]bool{} // containers already on the new list
	for _, src := range sources {
		for _, r := range src.Rows {
			p := place{containerOf(r), r.ItemID}
			if i, ok := at[p]; ok {
				if sameUnit(out[i].Unit, r.Unit) && r.Quantity > out[i].Quantity {
					out[i].Quantity = r.Quantity
				}
				continue
			}
			if isContainer[r.ItemID] {
				if placed[r.ItemID] {
					continue
				}
				placed[r.ItemID] = true
			}
			e := r
			e.StatusID = statusFor(src, r, defaultStatus)
			at[p] = len(out)
			out = append(out, e)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return sound(out)
}

func statusFor(src Source, e Entry, defaultStatus int64) int64 {
	if src.IsTemplate {
		return e.StatusID
	}
	return defaultStatus
}

// sound makes sure the rows form a tree: a row whose container is not among
// the rows, or that is itself part of a circle of containers, is moved to the
// top level (what hangs from it stays with it). Sound source lists cannot
// produce either; this is the guarantee that what is written to the database
// is navigable whatever went in.
func sound(rows []Entry) []Entry {
	// reachesTop reports whether walking up from a row's container ends at
	// the top level; onCircle whether the walk comes back to the row's item.
	walk := func(parent map[int64]int64, r Entry) (reachesTop, onCircle bool) {
		c := containerOf(r)
		for steps := 0; steps <= len(rows); steps++ {
			if c == 0 {
				return true, false
			}
			if c == r.ItemID {
				return false, true
			}
			next, ok := parent[c]
			if !ok {
				return false, false
			}
			c = next
		}
		return false, false
	}
	// Each pass detaches one row and starts over (the map is then stale):
	// first the rows that are themselves the problem, then, should anything
	// still not reach the top (only possible when a container is in two
	// places), whatever is left hanging.
	for _, hanging := range []bool{false, true} {
		for range len(rows) + 1 {
			parent := map[int64]int64{}
			for _, r := range rows {
				parent[r.ItemID] = containerOf(r)
			}
			changed := false
			for i := range rows {
				c := containerOf(rows[i])
				if c == 0 {
					continue
				}
				_, present := parent[c]
				top, circle := walk(parent, rows[i])
				if !present || circle || (hanging && !top) {
					rows[i].ContainerID, rows[i].CompartmentID = nil, nil // a compartment is part of its container
					changed = true
					break
				}
			}
			if !changed {
				break
			}
		}
	}
	// Moving rows can make two of them the same placement: keep the first.
	seen := map[place]bool{}
	out := rows[:0]
	for _, r := range rows {
		if r.ContainerID == nil {
			r.CompartmentID = nil // a compartment is part of a container
		}
		p := place{containerOf(r), r.ItemID}
		if seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, r)
	}
	return out
}
