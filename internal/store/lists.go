package store

// Lists: the packing/equipment lists themselves. What is on a list is in
// contents.go; how lists are combined into a new one is in merge.go.

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

// List is one row of Lists with a summary of its contents.
type List struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	IsTemplate bool   `json:"is_template"`
	// Rows is the number of List_Contents rows; StatusCounts splits them by
	// status ID (statuses with no rows are left out).
	Rows         int           `json:"rows"`
	StatusCounts map[int64]int `json:"status_counts"`
}

// Content is one List_Contents row.
type Content struct {
	ID          int64  `json:"id"`
	ListID      int64  `json:"list_id"`
	ContainerID *int64 `json:"container_id"` // the item this row is inside; null = top level
	// CompartmentID is the compartment of that container the row is in; null
	// = in the container, in no particular compartment.
	CompartmentID *int64  `json:"compartment_id"`
	ItemID        int64   `json:"item_id"`
	Quantity      int64   `json:"quantity"`
	Unit          *string `json:"unit"`
	StatusID      int64   `json:"status_id"`
	Notes         *string `json:"notes"`
}

// ListDetail is a list with everything on it.
type ListDetail struct {
	List
	Contents []Content `json:"contents"`
}

const contentCols = `ID, List_ID, Container_ID, Compartment_ID, Item_ID, Quantity, Unit, Status_ID, Notes`

func scanContent(row pgx.Row) (Content, error) {
	var c Content
	err := row.Scan(&c.ID, &c.ListID, &c.ContainerID, &c.CompartmentID, &c.ItemID, &c.Quantity, &c.Unit, &c.StatusID, &c.Notes)
	return c, err
}

func queryContents(ctx context.Context, q querier, listID int64) ([]Content, error) {
	rows, err := q.Query(ctx, `SELECT `+contentCols+` FROM List_Contents WHERE List_ID = $1 ORDER BY ID`, listID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Content{}
	for rows.Next() {
		c, err := scanContent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ListLists returns every list, by name, with its row and status counts.
func (s *Store) ListLists(ctx context.Context) ([]List, error) {
	rows, err := s.Pool.Query(ctx, `SELECT ID, Name, Is_Template FROM Lists ORDER BY lower(Name), ID`)
	if err != nil {
		return nil, err
	}
	out := []List{}
	at := map[int64]int{}
	for rows.Next() {
		l := List{StatusCounts: map[int64]int{}}
		if err := rows.Scan(&l.ID, &l.Name, &l.IsTemplate); err != nil {
			rows.Close()
			return nil, err
		}
		at[l.ID] = len(out)
		out = append(out, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	counts, err := s.Pool.Query(ctx, `SELECT List_ID, Status_ID, count(*) FROM List_Contents GROUP BY List_ID, Status_ID`)
	if err != nil {
		return nil, err
	}
	defer counts.Close()
	for counts.Next() {
		var listID, statusID int64
		var n int
		if err := counts.Scan(&listID, &statusID, &n); err != nil {
			return nil, err
		}
		if i, ok := at[listID]; ok {
			out[i].StatusCounts[statusID] = n
			out[i].Rows += n
		}
	}
	return out, counts.Err()
}

func getList(ctx context.Context, q querier, id int64) (*ListDetail, error) {
	if !fitsSmallint(id) {
		return nil, ErrNotFound
	}
	d := &ListDetail{List: List{StatusCounts: map[int64]int{}}}
	err := q.QueryRow(ctx, `SELECT ID, Name, Is_Template FROM Lists WHERE ID = $1`, id).Scan(&d.ID, &d.Name, &d.IsTemplate)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if d.Contents, err = queryContents(ctx, q, id); err != nil {
		return nil, err
	}
	d.Rows = len(d.Contents)
	for _, c := range d.Contents {
		d.StatusCounts[c.StatusID]++
	}
	return d, nil
}

// GetList returns one list with its contents.
func (s *Store) GetList(ctx context.Context, id int64) (*ListDetail, error) {
	return getList(ctx, s.Pool, id)
}

// NewList is what CreateList needs.
type NewList struct {
	Name       string  `json:"name"`
	IsTemplate bool    `json:"is_template"`
	Sources    []int64 `json:"sources"` // lists to combine into it, in order; empty = start from scratch
}

func checkListName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", Invalid("name is required")
	}
	if len([]rune(name)) > 127 {
		return "", Invalid("name is too long (at most 127 characters)")
	}
	return name, nil
}

// loadSources reads the source lists (in the order given, repeats dropped)
// and which of the items on them are containers.
func loadSources(ctx context.Context, q querier, ids []int64) ([]Source, map[int64]bool, error) {
	var sources []Source
	seen := map[int64]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if !fitsSmallint(id) {
			return nil, nil, Invalid("sources: there is no list %d", id)
		}
		var src Source
		// FOR SHARE: the source cannot be deleted while it is being copied.
		err := q.QueryRow(ctx, `SELECT Is_Template FROM Lists WHERE ID = $1 FOR SHARE`, id).Scan(&src.IsTemplate)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, Invalid("sources: there is no list %d", id)
		}
		if err != nil {
			return nil, nil, err
		}
		contents, err := queryContents(ctx, q, id)
		if err != nil {
			return nil, nil, err
		}
		for _, c := range contents {
			src.Rows = append(src.Rows, Entry{ContainerID: c.ContainerID, CompartmentID: c.CompartmentID, ItemID: c.ItemID, Quantity: c.Quantity, Unit: c.Unit, StatusID: c.StatusID, Notes: c.Notes})
		}
		sources = append(sources, src)
	}

	// Which of the items on the sources are containers. FOR SHARE holds each
	// flag still until the new list is written (guardContainerFlag takes the
	// same rows FOR UPDATE), so an item cannot become a container halfway
	// through and end up on the new list twice.
	var itemIDs []int64
	onSources := map[int64]bool{}
	for _, src := range sources {
		for _, r := range src.Rows {
			if !onSources[r.ItemID] {
				onSources[r.ItemID] = true
				itemIDs = append(itemIDs, r.ItemID)
			}
		}
	}
	isContainer := map[int64]bool{}
	rows, err := q.Query(ctx, `SELECT ID, Is_Container FROM Items WHERE ID = ANY($1) ORDER BY ID FOR SHARE`, itemIDs)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var is bool
		if err := rows.Scan(&id, &is); err != nil {
			return nil, nil, err
		}
		if is {
			isContainer[id] = true
		}
	}
	return sources, isContainer, rows.Err()
}

// CreateList makes a list: empty, or the union of other lists (see merge.go).
func (s *Store) CreateList(ctx context.Context, n NewList) (*ListDetail, error) {
	name, err := checkListName(n.Name)
	if err != nil {
		return nil, err
	}
	var out *ListDetail
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		var entries []Entry
		if len(n.Sources) > 0 {
			sources, isContainer, err := loadSources(ctx, tx, n.Sources)
			if err != nil {
				return err
			}
			total := 0
			for _, src := range sources {
				total += len(src.Rows)
			}
			if total > 0 {
				def, err := defaultStatusID(ctx, tx)
				if err != nil {
					return err
				}
				entries = Merge(sources, isContainer, def)
			}
		}
		var id int64
		if err := tx.QueryRow(ctx, `INSERT INTO Lists (Name, Is_Template) VALUES ($1, $2) RETURNING ID`, name, n.IsTemplate).Scan(&id); err != nil {
			return err
		}
		if len(entries) > 0 {
			rows := make([][]any, len(entries))
			for i, e := range entries {
				rows[i] = []any{id, e.ContainerID, e.CompartmentID, e.ItemID, e.Quantity, e.Unit, e.StatusID, e.Notes}
			}
			if _, err := tx.CopyFrom(ctx, pgx.Identifier{"list_contents"},
				[]string{"list_id", "container_id", "compartment_id", "item_id", "quantity", "unit", "status_id", "notes"}, pgx.CopyFromRows(rows)); err != nil {
				return err
			}
		}
		out, err = getList(ctx, tx, id)
		return err
	})
	if err != nil {
		return nil, TranslateErr(err, "list")
	}
	return out, nil
}

// MergePreview says how many rows a list made from some sources would have.
type MergePreview struct {
	Rows int `json:"rows"`
}

// PreviewMerge counts what CreateList would produce, without creating anything.
func (s *Store) PreviewMerge(ctx context.Context, ids []int64) (*MergePreview, error) {
	p := &MergePreview{}
	if len(ids) == 0 {
		return p, nil
	}
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		sources, isContainer, err := loadSources(ctx, tx, ids)
		if err != nil {
			return err
		}
		// The count does not depend on the statuses, so any status will do.
		p.Rows = len(Merge(sources, isContainer, 0))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return p, nil
}

// UpdateList renames a list or changes its template flag (fields: name,
// is_template) and returns it with its contents.
func (s *Store) UpdateList(ctx context.Context, id int64, f Fields) (*ListDetail, error) {
	if err := f.only("a list", "name", "is_template"); err != nil {
		return nil, err
	}
	if !fitsSmallint(id) {
		return nil, ErrNotFound
	}
	var out *ListDetail
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if name, ok, err := f.String("name"); err != nil {
			return err
		} else if ok {
			if name, err = checkListName(name); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE Lists SET Name = $1 WHERE ID = $2`, name, id); err != nil {
				return err
			}
		}
		if v, ok, err := f.Bool("is_template"); err != nil {
			return err
		} else if ok {
			if _, err := tx.Exec(ctx, `UPDATE Lists SET Is_Template = $1 WHERE ID = $2`, v, id); err != nil {
				return err
			}
		}
		var err error
		out, err = getList(ctx, tx, id)
		return err
	})
	if err != nil {
		return nil, TranslateErr(err, "list")
	}
	return out, nil
}

// DeleteList removes a list and everything on it. (The library is untouched.)
func (s *Store) DeleteList(ctx context.Context, id int64) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		if err := lockList(ctx, tx, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM List_Contents WHERE List_ID = $1`, id); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM Lists WHERE ID = $1`, id)
		return err
	})
}

// SetAllStatuses gives every row of a list the same status (to start
// packing afresh, say) and returns the list.
func (s *Store) SetAllStatuses(ctx context.Context, id, statusID int64) (*ListDetail, error) {
	var out *ListDetail
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if err := lockList(ctx, tx, id); err != nil {
			return err
		}
		if err := statusExists(ctx, tx, statusID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE List_Contents SET Status_ID = $1 WHERE List_ID = $2 AND Status_ID <> $1`, statusID, id); err != nil {
			return err
		}
		var err error
		out, err = getList(ctx, tx, id)
		return err
	})
	if err != nil {
		return nil, TranslateErr(err, "list")
	}
	return out, nil
}
