package store

// What is on a list: adding, moving, editing and removing List_Contents rows.
//
// A row puts an item on a list inside a container, and the container is
// named by its item (List_Contents.Container_ID -> Items.ID, as spec.md has
// it). For that to describe a tree, every function here keeps three rules,
// checked inside the transaction that makes the change (the list's row in
// Lists is locked first, so two changes to one list cannot interleave):
//
//  1. A row's container is an item flagged Is_Container that is itself on
//     the same list.
//  2. A container is on a list at most once, so "inside the Blue Backpack"
//     means one place. (Anything else can be on a list several times, in
//     different containers: socks in the suitcase and socks in the day bag.)
//  3. Containers do not hold one another in a circle.
//
// A row can also say which compartment of its container it is in
// (Compartment_ID, optional). The compartment must be one of that container's;
// the database enforces that with a two-column foreign key, and the functions
// here check it first so as to say what is wrong.
//
// Items.Is_Container can only change while that keeps the rules (ref.go
// guardContainerFlag).

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

const (
	maxQuantity = 2147483647
	maxUnit     = 127
	maxNotes    = 255
)

// lockList takes the list's row lock, which serialises changes to its contents.
func lockList(ctx context.Context, tx pgx.Tx, id int64) error {
	if !fitsSmallint(id) {
		return ErrNotFound
	}
	var one int
	err := tx.QueryRow(ctx, `SELECT 1 FROM Lists WHERE ID = $1 FOR UPDATE`, id).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func statusExists(ctx context.Context, q querier, id int64) error {
	if !fitsSmallint(id) {
		return Invalid("status_id: there is no status %d", id)
	}
	var one int
	err := q.QueryRow(ctx, `SELECT 1 FROM Statuses WHERE ID = $1`, id).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return Invalid("status_id: there is no status %d", id)
	}
	return err
}

// itemInfo reads an item's name and container flag, holding the flag still
// until the transaction ends (see guardContainerFlag).
func itemInfo(ctx context.Context, tx pgx.Tx, id int64, field string) (name string, isContainer bool, err error) {
	if !fitsInt(id) {
		return "", false, Invalid("%s: there is no item %d", field, id)
	}
	err = tx.QueryRow(ctx, `SELECT Name, Is_Container FROM Items WHERE ID = $1 FOR SHARE`, id).Scan(&name, &isContainer)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, Invalid("%s: there is no item %d", field, id)
	}
	return name, isContainer, err
}

// checkContainer verifies rule 1 for a row about to sit inside `container`
// and returns the container's name.
func checkContainer(ctx context.Context, tx pgx.Tx, listID, container int64) (string, error) {
	name, is, err := itemInfo(ctx, tx, container, "container_id")
	if err != nil {
		return "", err
	}
	if !is {
		return "", Invalid("%q is not a container: mark it as one in the Library first", name)
	}
	var one int
	err = tx.QueryRow(ctx, `SELECT 1 FROM List_Contents WHERE List_ID = $1 AND Item_ID = $2 LIMIT 1`, listID, container).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", Invalid("%q is not on this list, so nothing can be put inside it here", name)
	}
	return name, err
}

// checkFree refuses a placement that is already taken (the rule behind
// UN_List_Contents, with a message a person can act on). except is the row
// being moved, or 0.
func checkFree(ctx context.Context, tx pgx.Tx, listID int64, container *int64, itemID, except int64, itemName, containerName string) error {
	var one int
	err := tx.QueryRow(ctx, `
		SELECT 1 FROM List_Contents
		WHERE List_ID = $1 AND Container_ID IS NOT DISTINCT FROM $2 AND Item_ID = $3 AND ID <> $4`,
		listID, container, itemID, except).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if container == nil {
		return Conflict("%q is already at the top level of this list: change its quantity instead", itemName)
	}
	return Conflict("%q is already in %q on this list: change its quantity instead", itemName, containerName)
}

// optionalText reads a nullable text field: "" and null both store null.
func optionalText(f Fields, key string, max int) (val *string, ok bool, err error) {
	s, ok, err := f.String(key)
	if err != nil || !ok {
		return nil, ok, err
	}
	if s == "" {
		return nil, true, nil
	}
	if len([]rune(s)) > max {
		return nil, true, Invalid("%s is too long (at most %d characters)", key, max)
	}
	return &s, true, nil
}

func quantityField(f Fields) (val int64, ok bool, err error) {
	v, isNull, ok, err := f.Int("quantity")
	if err != nil || !ok {
		return 0, ok, err
	}
	if isNull || v < 1 || v > maxQuantity {
		return 0, true, Invalid("quantity must be a whole number from 1 up")
	}
	return v, true, nil
}

// AddContent puts an item on a list. Fields: item_id (required),
// container_id (absent or null = top level), compartment_id (a compartment
// of that container; absent or null = none), quantity (default 1), unit,
// status_id (default: the first status) and notes.
func (s *Store) AddContent(ctx context.Context, listID int64, f Fields) (*Content, error) {
	if err := f.only("a list entry", "item_id", "container_id", "compartment_id", "quantity", "unit", "status_id", "notes"); err != nil {
		return nil, err
	}
	itemID, isNull, ok, err := f.Int("item_id")
	if err != nil {
		return nil, err
	}
	if !ok || isNull {
		return nil, Invalid("item_id is required")
	}
	var container *int64
	if v, isNull, ok, err := f.Int("container_id"); err != nil {
		return nil, err
	} else if ok && !isNull {
		container = &v
	}
	var compartment *int64
	if v, isNull, ok, err := f.Int("compartment_id"); err != nil {
		return nil, err
	} else if ok && !isNull {
		compartment = &v
	}
	quantity := int64(1)
	if v, ok, err := quantityField(f); err != nil {
		return nil, err
	} else if ok {
		quantity = v
	}
	unit, _, err := optionalText(f, "unit", maxUnit)
	if err != nil {
		return nil, err
	}
	notes, _, err := optionalText(f, "notes", maxNotes)
	if err != nil {
		return nil, err
	}
	statusID, statusNull, statusSent, err := f.Int("status_id")
	if err != nil {
		return nil, err
	}

	var out Content
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		if err := lockList(ctx, tx, listID); err != nil {
			return err
		}
		itemName, isContainer, err := itemInfo(ctx, tx, itemID, "item_id")
		if err != nil {
			return err
		}
		containerName := ""
		if container != nil {
			if *container == itemID {
				return Invalid("%q cannot be put inside itself", itemName)
			}
			if containerName, err = checkContainer(ctx, tx, listID, *container); err != nil {
				return err
			}
		}
		if isContainer {
			// Rule 2. (It is not on the list yet, so nothing is inside it
			// and it cannot close a circle.)
			var one int
			err := tx.QueryRow(ctx, `SELECT 1 FROM List_Contents WHERE List_ID = $1 AND Item_ID = $2 LIMIT 1`, listID, itemID).Scan(&one)
			if err == nil {
				return Conflict("%q is a container and is already on this list: a container can only be in one place, so move it instead", itemName)
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		if err := checkFree(ctx, tx, listID, container, itemID, 0, itemName, containerName); err != nil {
			return err
		}
		if err := checkCompartment(ctx, tx, compartment, container, containerName); err != nil {
			return err
		}
		if !statusSent || statusNull {
			if statusID, err = defaultStatusID(ctx, tx); err != nil {
				return err
			}
		} else if err := statusExists(ctx, tx, statusID); err != nil {
			return err
		}
		out, err = scanContent(tx.QueryRow(ctx, `
			INSERT INTO List_Contents (List_ID, Container_ID, Compartment_ID, Item_ID, Quantity, Unit, Status_ID, Notes)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING `+contentCols,
			listID, container, compartment, itemID, quantity, unit, statusID, notes))
		return err
	})
	if err != nil {
		return nil, TranslateErr(err, "list entry")
	}
	return &out, nil
}

// lockContent finds a row and locks its list. (The list is read first,
// unlocked, then locked, then the row is read again under the lock.)
func lockContent(ctx context.Context, tx pgx.Tx, id int64) (Content, error) {
	if !fitsInt(id) {
		return Content{}, ErrNotFound
	}
	var listID int64
	err := tx.QueryRow(ctx, `SELECT List_ID FROM List_Contents WHERE ID = $1`, id).Scan(&listID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Content{}, ErrNotFound
	}
	if err != nil {
		return Content{}, err
	}
	if err := lockList(ctx, tx, listID); err != nil {
		return Content{}, err
	}
	c, err := scanContent(tx.QueryRow(ctx, `SELECT `+contentCols+` FROM List_Contents WHERE ID = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Content{}, ErrNotFound
	}
	return c, err
}

// UpdateContent changes a row. Fields: container_id (null = top level; a
// container takes what is inside it along), compartment_id (a compartment of
// the container the row is, or is being put, in; null = none), quantity,
// unit, status_id and notes. Moving a row to another container takes it out
// of its compartment unless one of the new container's is given. The item
// itself cannot be changed: remove the row and add another.
func (s *Store) UpdateContent(ctx context.Context, id int64, f Fields) (*Content, error) {
	if err := f.only("a list entry", "container_id", "compartment_id", "quantity", "unit", "status_id", "notes"); err != nil {
		return nil, err
	}
	var out Content
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		cur, err := lockContent(ctx, tx, id)
		if err != nil {
			return err
		}
		container, containerNull, containerSent, err := f.Int("container_id")
		if err != nil {
			return err
		}
		compartment, compartmentNull, compartmentSent, err := f.Int("compartment_id")
		if err != nil {
			return err
		}
		if containerSent || compartmentSent {
			to := cur.ContainerID
			if containerSent {
				to = nil
				if !containerNull {
					to = &container
				}
			}
			// The compartment asked for; else none after a move to another
			// container (the old one's compartments are not the new one's);
			// else the one it is in.
			var in *int64
			switch {
			case compartmentSent && !compartmentNull:
				in = &compartment
			case compartmentSent, !sameID(to, cur.ContainerID):
				in = nil
			default:
				in = cur.CompartmentID
			}
			if err := moveContent(ctx, tx, cur, to, in); err != nil {
				return err
			}
		}
		if v, ok, err := quantityField(f); err != nil {
			return err
		} else if ok {
			if _, err := tx.Exec(ctx, `UPDATE List_Contents SET Quantity = $1 WHERE ID = $2`, v, id); err != nil {
				return err
			}
		}
		if v, ok, err := optionalText(f, "unit", maxUnit); err != nil {
			return err
		} else if ok {
			if _, err := tx.Exec(ctx, `UPDATE List_Contents SET Unit = $1 WHERE ID = $2`, v, id); err != nil {
				return err
			}
		}
		if v, ok, err := optionalText(f, "notes", maxNotes); err != nil {
			return err
		} else if ok {
			if _, err := tx.Exec(ctx, `UPDATE List_Contents SET Notes = $1 WHERE ID = $2`, v, id); err != nil {
				return err
			}
		}
		if v, isNull, ok, err := f.Int("status_id"); err != nil {
			return err
		} else if ok {
			if isNull {
				return Invalid("status_id is required")
			}
			if err := statusExists(ctx, tx, v); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE List_Contents SET Status_ID = $1 WHERE ID = $2`, v, id); err != nil {
				return err
			}
		}
		out, err = scanContent(tx.QueryRow(ctx, `SELECT `+contentCols+` FROM List_Contents WHERE ID = $1`, id))
		return err
	})
	if err != nil {
		return nil, TranslateErr(err, "list entry")
	}
	return &out, nil
}

func sameID(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// checkCompartment verifies that a compartment (nil = none) is one of the
// container's (nil = top level).
func checkCompartment(ctx context.Context, tx pgx.Tx, compartment, container *int64, containerName string) error {
	if compartment == nil {
		return nil
	}
	if container == nil {
		return Invalid("an entry at the top level of a list is in no container, so it cannot be in a compartment")
	}
	if !fitsInt(*compartment) {
		return Invalid("compartment_id: there is no compartment %d", *compartment)
	}
	var itemID int64
	err := tx.QueryRow(ctx, `SELECT Item_ID FROM Compartments WHERE ID = $1`, *compartment).Scan(&itemID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Invalid("compartment_id: there is no compartment %d", *compartment)
	}
	if err != nil {
		return err
	}
	if itemID != *container {
		if containerName == "" {
			if containerName, _, err = itemInfo(ctx, tx, *container, "container_id"); err != nil {
				return err
			}
		}
		return Invalid("that compartment is not one of %q's", containerName)
	}
	return nil
}

// moveContent puts a row into another container (nil = the top level) and
// into one of that container's compartments (nil = none). What is inside a
// container moves with it, since its contents refer to the container's item,
// not to its row.
func moveContent(ctx context.Context, tx pgx.Tx, cur Content, to, compartment *int64) error {
	moved := !sameID(to, cur.ContainerID)
	if !moved && sameID(compartment, cur.CompartmentID) {
		return nil
	}
	containerName := ""
	if moved {
		itemName, _, err := itemInfo(ctx, tx, cur.ItemID, "item_id")
		if err != nil {
			return err
		}
		if to != nil {
			if *to == cur.ItemID {
				return Invalid("%q cannot be put inside itself", itemName)
			}
			if containerName, err = checkContainer(ctx, tx, cur.ListID, *to); err != nil {
				return err
			}
			// Rule 3: the destination must not be inside the thing being moved.
			// Walk up from the destination; containers are on the list once, so
			// each step has one answer.
			parent := map[int64]*int64{}
			rows, err := queryContents(ctx, tx, cur.ListID)
			if err != nil {
				return err
			}
			for _, r := range rows {
				parent[r.ItemID] = r.ContainerID
			}
			at := to
			for steps := 0; at != nil && steps <= len(rows); steps++ {
				if *at == cur.ItemID {
					return Invalid("%q cannot be moved into %q, which is inside it", itemName, containerName)
				}
				at = parent[*at]
			}
		}
		if err := checkFree(ctx, tx, cur.ListID, to, cur.ItemID, cur.ID, itemName, containerName); err != nil {
			return err
		}
	}
	if err := checkCompartment(ctx, tx, compartment, to, containerName); err != nil {
		return err
	}
	// Both columns in one statement: together they must name a compartment of the container.
	_, err := tx.Exec(ctx, `UPDATE List_Contents SET Container_ID = $1, Compartment_ID = $2 WHERE ID = $3`, to, compartment, cur.ID)
	return err
}

// DeleteContent takes a row off its list. When the row is a container with
// things inside it, keepContents says what becomes of them: false removes
// them too (everything inside, at every depth); true moves what was directly
// inside to where the container was.
func (s *Store) DeleteContent(ctx context.Context, id int64, keepContents bool) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		cur, err := lockContent(ctx, tx, id)
		if err != nil {
			return err
		}
		if !keepContents {
			// UNION (not UNION ALL) so that even an unsound list cannot loop.
			_, err := tx.Exec(ctx, `
				WITH RECURSIVE sub AS (
				    SELECT ID, Item_ID FROM List_Contents WHERE ID = $1
				    UNION
				    SELECT c.ID, c.Item_ID FROM List_Contents c JOIN sub ON c.Container_ID = sub.Item_ID
				    WHERE c.List_ID = $2
				)
				DELETE FROM List_Contents WHERE ID IN (SELECT ID FROM sub)`, id, cur.ListID)
			return err
		}
		// Keep the contents: they take the removed container's place, unless
		// one of them is already there.
		var clash string
		err = tx.QueryRow(ctx, `
			SELECT i.Name FROM List_Contents child
			JOIN List_Contents other ON other.List_ID = child.List_ID AND other.Item_ID = child.Item_ID
			     AND other.Container_ID IS NOT DISTINCT FROM $3 AND other.ID <> child.ID
			JOIN Items i ON i.ID = child.Item_ID
			WHERE child.List_ID = $1 AND child.Container_ID = $2
			ORDER BY lower(i.Name) LIMIT 1`, cur.ListID, cur.ItemID, cur.ContainerID).Scan(&clash)
		if err == nil {
			return Conflict("the contents cannot be kept: %q is already in the place they would move to", clash)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM List_Contents WHERE ID = $1`, id); err != nil {
			return err
		}
		// (Out of the removed container's compartments too: they are not the new container's.)
		_, err = tx.Exec(ctx, `UPDATE List_Contents SET Container_ID = $1, Compartment_ID = NULL WHERE List_ID = $2 AND Container_ID = $3`,
			cur.ContainerID, cur.ListID, cur.ItemID)
		return err
	})
}
