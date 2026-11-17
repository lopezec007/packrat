// Package store is Packrat's data access layer over PostgreSQL.
//
// Conventions shared by every file in the package:
//   - Methods take a context and return ErrNotFound, a *ValidationError (the
//     caller sent something wrong; 400) or a *ConflictError (the request is
//     well formed but the data forbids it; 409). Anything else is an
//     internal error.
//   - Rows travel as JSON objects whose keys are the column names in lower
//     case (id, name, item_category_id, ...).
//
// The files:
//
//	ref.go       the library and settings tables (categories, storage locations, statuses, items)
//	lists.go     lists: create (empty or from other lists), rename, delete
//	contents.go  what is on a list: add, move, edit and remove rows, keeping the tree sound
//	merge.go     how several lists are combined into a new one: their union (pure functions)
package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store wraps the connection pool.
type Store struct {
	Pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store { return &Store{Pool: pool} }

// ErrNotFound is returned when the addressed row does not exist.
var ErrNotFound = errors.New("not found")

// ValidationError reports a request the caller must fix (HTTP 400).
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// Invalid makes a *ValidationError.
func Invalid(format string, args ...any) error {
	return &ValidationError{Msg: fmt.Sprintf(format, args...)}
}

// ConflictError reports a well-formed request the current data forbids, such
// as a duplicate name or deleting a row that is in use (HTTP 409).
type ConflictError struct{ Msg string }

func (e *ConflictError) Error() string { return e.Msg }

// Conflict makes a *ConflictError.
func Conflict(format string, args ...any) error {
	return &ConflictError{Msg: fmt.Sprintf(format, args...)}
}

// PG error codes the store translates.
const (
	pgForeignKeyViolation = "23503"
	pgUniqueViolation     = "23505"
	pgCheckViolation      = "23514"
	pgNotNullViolation    = "23502"
	pgStringTooLong       = "22001"
	pgNumericOutOfRange   = "22003"
	pgInvalidText         = "22P02"
	pgInvalidEncoding     = "22021" // text PostgreSQL cannot store: a NUL character
)

// constraintMessages says, in words a person can act on, what breaking each
// constraint means: a duplicate for the UN_ ones, a delete of a row in use
// for the FK_ ones. (PostgreSQL reports constraint names in lower case.)
var constraintMessages = map[string]string{
	"un_lists":                      "there is already a list with that name",
	"un_item_categories":            "there is already an item category with that name",
	"un_storage_locations":          "there is already a storage location with that name",
	"un_items":                      "there is already an item with that name (item names are unique across all categories)",
	"un_statuses":                   "there is already a status with that name",
	"un_statuses_shortcut":          "another status already uses that shortcut key",
	"un_list_contents":              "that item is already in that place on the list",
	"fk_items_item_category_id":     "this item category still has items in the library: move them to another category first",
	"fk_items_storage_location_id":  "this storage location still has items in the library: move them to another location first",
	"fk_list_contents_item_id":      "this item is on a list: take it off every list before deleting it",
	"fk_list_contents_container_id": "this item holds other items on a list: take it off every list before deleting it",
	"fk_list_contents_status_id":    "this status is in use on a list: give those entries another status first",
}

// TranslateErr turns the PostgreSQL errors a caller can cause (duplicates,
// rows in use, values out of range) into Validation/Conflict errors that
// name `what` (e.g. "item"). Other errors pass through unchanged.
func TranslateErr(err error, what string) error {
	var pe *pgconn.PgError
	if !errors.As(err, &pe) {
		return err
	}
	if msg, ok := constraintMessages[pe.ConstraintName]; ok && (pe.Code == pgUniqueViolation ||
		(pe.Code == pgForeignKeyViolation && strings.HasPrefix(pe.Message, "update or delete"))) {
		return Conflict("%s", msg)
	}
	switch pe.Code {
	case pgUniqueViolation:
		return Conflict("that %s already exists (%s)", what, pe.ConstraintName)
	case pgForeignKeyViolation:
		// PostgreSQL words the two cases differently: "update or delete on
		// table X violates ..." is a delete (or key change) of a row in use;
		// "insert or update on table X violates ..." points at a row that is
		// not there.
		if strings.HasPrefix(pe.Message, "update or delete") {
			return Conflict("this %s is in use (%s) and cannot be deleted", what, pe.ConstraintName)
		}
		return Invalid("%s refers to a row that does not exist (%s)", what, pe.ConstraintName)
	case pgCheckViolation:
		return Invalid("%s breaks the rule %s", what, pe.ConstraintName)
	case pgNotNullViolation:
		return Invalid("%s: %s is required", what, pe.ColumnName)
	case pgStringTooLong, pgNumericOutOfRange, pgInvalidText, pgInvalidEncoding:
		return Invalid("%s: %s", what, pe.Message)
	}
	return err
}

// fitsSmallint and fitsInt report whether an ID the caller sent can be in a
// smallserial or serial column at all. A number beyond the column's range
// cannot be sent to PostgreSQL as that column's type (pgx fails to encode
// it, which would come out as an internal error), so the functions that
// look a row up by ID check first and answer what is true: no such row.
func fitsSmallint(id int64) bool { return id >= math.MinInt16 && id <= math.MaxInt16 }
func fitsInt(id int64) bool      { return id >= math.MinInt32 && id <= math.MaxInt32 }

// inTx runs fn in a transaction, committing when it returns nil.
func (s *Store) inTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	return pgx.BeginFunc(ctx, s.Pool, fn)
}

// ---------------------------------------------------------------- JSON helpers

// Fields is a partially decoded JSON object: the keys the caller actually
// sent, so PATCH handlers can tell "absent" from "null".
type Fields map[string]json.RawMessage

// IsNull reports whether raw is the JSON literal null.
func IsNull(raw json.RawMessage) bool { return bytes.Equal(bytes.TrimSpace(raw), []byte("null")) }

// Has reports whether the key was sent at all (null included).
func (f Fields) Has(key string) bool { _, ok := f[key]; return ok }

// String decodes a JSON string field, trimmed. ok is false when the key is
// absent; a JSON null yields "" with ok true.
func (f Fields) String(key string) (val string, ok bool, err error) {
	raw, ok := f[key]
	if !ok {
		return "", false, nil
	}
	if IsNull(raw) {
		return "", true, nil
	}
	if err := json.Unmarshal(raw, &val); err != nil {
		return "", true, Invalid("%s must be text", key)
	}
	return strings.TrimSpace(val), true, nil
}

// Int decodes an integer field. null is reported through isNull.
func (f Fields) Int(key string) (val int64, isNull, ok bool, err error) {
	raw, ok := f[key]
	if !ok {
		return 0, false, false, nil
	}
	if IsNull(raw) {
		return 0, true, true, nil
	}
	if err := json.Unmarshal(raw, &val); err != nil {
		return 0, false, true, Invalid("%s must be a whole number", key)
	}
	return val, false, true, nil
}

// Bool decodes a boolean field.
func (f Fields) Bool(key string) (val, ok bool, err error) {
	raw, ok := f[key]
	if !ok {
		return false, false, nil
	}
	if err := json.Unmarshal(raw, &val); err != nil || IsNull(raw) {
		return false, true, Invalid("%s must be true or false", key)
	}
	return val, true, nil
}

// only refuses keys the caller sent that are not among `known`.
func (f Fields) only(what string, known ...string) error {
	for k := range f {
		found := false
		for _, n := range known {
			if k == n {
				found = true
				break
			}
		}
		if !found {
			return Invalid("unknown field %q for %s", k, what)
		}
	}
	return nil
}
