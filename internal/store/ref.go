package store

// Generic create/read/update/delete for the library and settings tables:
// Item_Categories, Storage_Locations, Statuses and Items (every table except
// Lists and List_Contents, which have their own files).
//
// Each table is described once in RefTables below; the SQL is generated from
// that description. Rows travel as JSON objects whose keys are the column
// names in lower case (id, name, item_category_id, ...).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
)

// Kind is how a column is validated on the way in and rendered on the way out.
type Kind int

const (
	KString   Kind = iota // text; JSON string
	KInt                  // smallint/int; JSON number
	KBool                 // bool
	KColor                // char(6) hex in the database; "#rrggbb" in JSON (the "#" and upper case are accepted on the way in)
	KShortcut             // char(1): one visible character, stored in lower case
)

// Col describes one column.
type Col struct {
	Name     string // column name as in the schema, e.g. "Item_Category_ID"; the JSON key is its lower case
	Kind     Kind
	MaxLen   int   // KString: varchar length
	Min, Max int64 // KInt: inclusive bounds (both zero = smallint range)
	// Default is used on create when the key is absent (nil = the field is required).
	Default any
}

func (c Col) key() string { return strings.ToLower(c.Name) }

// Extra is a read-only computed value added to each row.
type Extra struct {
	Key  string // JSON key
	Expr string // SQL expression over the table's columns
}

// RefTable describes one reference table.
type RefTable struct {
	Key     string // JSON/URL name: "item_categories"
	Table   string // SQL name: "Item_Categories"
	What    string // singular noun for messages: "item category"
	Cols    []Col  // every column except ID
	Extras  []Extra
	OrderBy string
}

var colorRe = regexp.MustCompile(`^[0-9a-f]{6}$`)

func nameCol() Col { return Col{Name: "Name", Kind: KString, MaxLen: 127} }

// RefTables lists the reference tables in dependency order (a table comes
// after the tables it references).
var RefTables = []RefTable{
	{Key: "item_categories", Table: "Item_Categories", What: "item category", OrderBy: "lower(Name)",
		Cols: []Col{nameCol()},
		Extras: []Extra{
			{Key: "item_count", Expr: "(SELECT count(*) FROM Items i WHERE i.Item_Category_ID = Item_Categories.ID)::int"},
		}},
	{Key: "storage_locations", Table: "Storage_Locations", What: "storage location", OrderBy: "lower(Name)",
		Cols: []Col{nameCol()},
		Extras: []Extra{
			{Key: "item_count", Expr: "(SELECT count(*) FROM Items i WHERE i.Storage_Location_ID = Storage_Locations.ID)::int"},
		}},
	// Statuses keep the order they were created in: the first is the status a
	// newly added item starts with (see DefaultStatusID).
	{Key: "statuses", Table: "Statuses", What: "status", OrderBy: "ID",
		Cols: []Col{
			nameCol(),
			{Name: "Color", Kind: KColor},
			{Name: "Shortcut", Kind: KShortcut},
		},
		Extras: []Extra{
			{Key: "use_count", Expr: "(SELECT count(*) FROM List_Contents lc WHERE lc.Status_ID = Statuses.ID)::int"},
		}},
	{Key: "items", Table: "Items", What: "item", OrderBy: "lower(Name)",
		Cols: []Col{
			{Name: "Item_Category_ID", Kind: KInt},
			nameCol(),
			{Name: "Storage_Location_ID", Kind: KInt},
			{Name: "Is_Container", Kind: KBool, Default: false},
		},
		Extras: []Extra{
			// How many lists the item is on.
			{Key: "list_count", Expr: "(SELECT count(DISTINCT lc.List_ID) FROM List_Contents lc WHERE lc.Item_ID = Items.ID)::int"},
		}},
}

var refByKey = func() map[string]*RefTable {
	m := map[string]*RefTable{}
	for i := range RefTables {
		m[RefTables[i].Key] = &RefTables[i]
	}
	return m
}()

// RefTableByKey returns the description of a reference table, or nil.
func RefTableByKey(key string) *RefTable { return refByKey[key] }

// fitsID reports whether id can be a row of the table at all: Items.ID is
// serial, the other tables' smallserial (see fitsSmallint).
func (t *RefTable) fitsID(id int64) bool {
	if t.Key == "items" {
		return fitsInt(id)
	}
	return fitsSmallint(id)
}

// Row is one reference row as it goes out in JSON.
type Row = map[string]any

// selectList is the table's output columns, in order: id, the columns, the extras.
func (t *RefTable) selectList() (exprs []string, keys []string) {
	exprs, keys = []string{"ID"}, []string{"id"}
	for _, c := range t.Cols {
		var e string
		switch c.Kind {
		case KColor:
			e = "'#' || " + c.Name + "::text"
		case KString, KShortcut:
			e = c.Name + "::text" // char(n) comes back without its padding
		default:
			e = c.Name
		}
		exprs = append(exprs, e)
		keys = append(keys, c.key())
	}
	for _, x := range t.Extras {
		exprs = append(exprs, x.Expr)
		keys = append(keys, x.Key)
	}
	return exprs, keys
}

func scanRows(rows pgx.Rows, keys []string) ([]Row, error) {
	defer rows.Close()
	out := []Row{}
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return nil, err
		}
		row := make(Row, len(keys))
		for i, k := range keys {
			row[k] = vals[i]
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// ListRef returns every row of one reference table.
func (s *Store) ListRef(ctx context.Context, t *RefTable) ([]Row, error) {
	exprs, keys := t.selectList()
	rows, err := s.Pool.Query(ctx, fmt.Sprintf("SELECT %s FROM %s ORDER BY %s, ID", strings.Join(exprs, ", "), t.Table, t.OrderBy))
	if err != nil {
		return nil, err
	}
	return scanRows(rows, keys)
}

// ListAllRef returns every reference table, keyed by RefTable.Key.
func (s *Store) ListAllRef(ctx context.Context) (map[string][]Row, error) {
	out := make(map[string][]Row, len(RefTables))
	for i := range RefTables {
		rows, err := s.ListRef(ctx, &RefTables[i])
		if err != nil {
			return nil, fmt.Errorf("list %s: %w", RefTables[i].Key, err)
		}
		out[RefTables[i].Key] = rows
	}
	return out, nil
}

// assignment is one column with the value to store in it.
type assignment struct {
	col string
	arg any
}

// ParseColor reads a colour as people write it ("#2E8540", "2e8540") into
// the six lower-case hex digits the database stores.
func ParseColor(v, key string) (string, error) {
	v = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(v), "#"))
	if !colorRe.MatchString(v) {
		return "", Invalid(`%s must be a hex colour such as "#2e8540"`, key)
	}
	return v, nil
}

// ParseShortcut reads a status's key: exactly one visible character, folded
// to lower case (the key is matched whatever the state of Shift).
func ParseShortcut(v, key string) (string, error) {
	r := []rune(strings.ToLower(strings.TrimSpace(v)))
	// IsGraphic leaves out control and format characters (a zero-width space, say).
	if len(r) != 1 || unicode.IsSpace(r[0]) || !unicode.IsGraphic(r[0]) {
		return "", Invalid("%s must be a single character (a letter, digit or symbol)", key)
	}
	return string(r), nil
}

// parseCol validates one incoming value for column c.
func parseCol(c Col, raw json.RawMessage) (a assignment, err error) {
	key := c.key()
	a.col = c.Name
	if IsNull(raw) {
		return a, Invalid("%s is required", key)
	}
	switch c.Kind {
	case KString, KColor, KShortcut:
		var v string
		if err := json.Unmarshal(raw, &v); err != nil {
			return a, Invalid("%s must be text", key)
		}
		v = strings.TrimSpace(v)
		if v == "" {
			return a, Invalid("%s is required", key)
		}
		switch c.Kind {
		case KColor:
			if v, err = ParseColor(v, key); err != nil {
				return a, err
			}
		case KShortcut:
			if v, err = ParseShortcut(v, key); err != nil {
				return a, err
			}
		default:
			if c.MaxLen > 0 && len([]rune(v)) > c.MaxLen {
				return a, Invalid("%s is too long (at most %d characters)", key, c.MaxLen)
			}
		}
		a.arg = v
	case KInt:
		var v int64
		if err := json.Unmarshal(raw, &v); err != nil {
			return a, Invalid("%s must be a whole number", key)
		}
		lo, hi := c.Min, c.Max
		if lo == 0 && hi == 0 {
			lo, hi = -32768, 32767
		}
		if v < lo || v > hi {
			return a, Invalid("%s is out of range", key)
		}
		a.arg = v
	case KBool:
		var v bool
		if err := json.Unmarshal(raw, &v); err != nil {
			return a, Invalid("%s must be true or false", key)
		}
		a.arg = v
	default:
		return a, fmt.Errorf("parseCol: unexpected kind for %s", c.Name)
	}
	return a, nil
}

// assignments turns the fields the caller sent into column assignments.
// Unknown keys are an error; computed keys are ignored (so a client can send
// back a row it was given).
func (t *RefTable) assignments(f Fields, insert bool) ([]assignment, error) {
	known := map[string]bool{"id": true}
	for _, x := range t.Extras {
		known[x.Key] = true
	}
	var out []assignment
	for _, c := range t.Cols {
		key := c.key()
		known[key] = true
		raw, ok := f[key]
		if !ok {
			if insert {
				if c.Default == nil {
					return nil, Invalid("%s is required", key)
				}
				out = append(out, assignment{col: c.Name, arg: c.Default})
			}
			continue
		}
		a, err := parseCol(c, raw)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	for k := range f {
		if !known[k] {
			return nil, Invalid("unknown field %q for %s", k, t.Key)
		}
	}
	return out, nil
}

// CreateRef inserts a row and returns it.
func (s *Store) CreateRef(ctx context.Context, t *RefTable, f Fields) (Row, error) {
	as, err := t.assignments(f, true)
	if err != nil {
		return nil, err
	}
	cols, vals, args := make([]string, len(as)), make([]string, len(as)), make([]any, len(as))
	for i, a := range as {
		cols[i], vals[i], args[i] = a.col, fmt.Sprintf("$%d", i+1), a.arg
	}
	exprs, keys := t.selectList()
	q := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) RETURNING %s",
		t.Table, strings.Join(cols, ", "), strings.Join(vals, ", "), strings.Join(exprs, ", "))
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, TranslateErr(err, t.What)
	}
	out, err := scanRows(rows, keys)
	if err != nil {
		return nil, TranslateErr(err, t.What)
	}
	return out[0], nil
}

// UpdateRef applies a partial update and returns the row as it now is.
func (s *Store) UpdateRef(ctx context.Context, t *RefTable, id int64, f Fields) (Row, error) {
	as, err := t.assignments(f, false)
	if err != nil {
		return nil, err
	}
	if !t.fitsID(id) {
		return nil, ErrNotFound
	}
	exprs, keys := t.selectList()
	var out []Row
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		if t.Key == "items" {
			if err := guardContainerFlag(ctx, tx, id, as); err != nil {
				return err
			}
		}
		var q string
		args := []any{}
		if len(as) == 0 {
			q = fmt.Sprintf("SELECT %s FROM %s WHERE ID = $1", strings.Join(exprs, ", "), t.Table)
		} else {
			sets := make([]string, len(as))
			for i, a := range as {
				sets[i] = fmt.Sprintf("%s = $%d", a.col, i+1)
				args = append(args, a.arg)
			}
			q = fmt.Sprintf("UPDATE %s SET %s WHERE ID = $%d RETURNING %s",
				t.Table, strings.Join(sets, ", "), len(as)+1, strings.Join(exprs, ", "))
		}
		args = append(args, id)
		rows, err := tx.Query(ctx, q, args...)
		if err != nil {
			return err
		}
		out, err = scanRows(rows, keys)
		return err
	})
	if err != nil {
		return nil, TranslateErr(err, t.What)
	}
	if len(out) == 0 {
		return nil, ErrNotFound
	}
	return out[0], nil
}

// guardContainerFlag refuses a change to Items.Is_Container that would leave
// a list's tree unsound: an item holding things cannot stop being a
// container, and an item that is on one list several times cannot become
// one (a container has a single place on a list; see contents.go).
func guardContainerFlag(ctx context.Context, tx pgx.Tx, id int64, as []assignment) error {
	var want, sent bool
	for _, a := range as {
		if a.col == "Is_Container" {
			want, sent = a.arg.(bool), true
		}
	}
	if !sent {
		return nil
	}
	// Lock the item so rows cannot be added to or under it meanwhile (the
	// functions in contents.go read the flag FOR SHARE).
	var is bool
	err := tx.QueryRow(ctx, `SELECT Is_Container FROM Items WHERE ID = $1 FOR UPDATE`, id).Scan(&is)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if is == want {
		return nil
	}
	var list string
	if !want {
		err = tx.QueryRow(ctx, `
			SELECT l.Name FROM List_Contents lc JOIN Lists l ON l.ID = lc.List_ID
			WHERE lc.Container_ID = $1 ORDER BY lower(l.Name) LIMIT 1`, id).Scan(&list)
		if err == nil {
			return Conflict("this item holds other items on the list %q: empty it there before it stops being a container", list)
		}
	} else {
		err = tx.QueryRow(ctx, `
			SELECT l.Name FROM List_Contents lc JOIN Lists l ON l.ID = lc.List_ID
			WHERE lc.Item_ID = $1 GROUP BY l.ID, l.Name HAVING count(*) > 1 ORDER BY lower(l.Name) LIMIT 1`, id).Scan(&list)
		if err == nil {
			return Conflict("this item is on the list %q more than once, and a container can only be in one place on a list: leave it there once before making it a container", list)
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return err
}

// DeleteRef removes a row; a row something still refers to is a ConflictError.
func (s *Store) DeleteRef(ctx context.Context, t *RefTable, id int64) error {
	if !t.fitsID(id) {
		return ErrNotFound
	}
	tag, err := s.Pool.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE ID = $1", t.Table), id)
	if err != nil {
		return TranslateErr(err, t.What)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DefaultStatusID is the status a newly added item starts with: the
// lowest-numbered one (on a fresh install, "In Storage Location").
func defaultStatusID(ctx context.Context, q querier) (int64, error) {
	var id int64
	err := q.QueryRow(ctx, `SELECT ID FROM Statuses ORDER BY ID LIMIT 1`).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, Conflict("there are no statuses yet: add one under Settings before filling a list")
	}
	return id, err
}

// querier is what the pool and a transaction have in common.
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}
