package api

// The library and settings tables (item categories, storage locations,
// statuses, items): one generic set of routes over store.RefTables.
//
//	GET    /api/ref                 every table: {"item_categories": [...], "storage_locations": [...], "statuses": [...], "items": [...]}
//	GET    /api/ref/{table}         one table's rows
//	POST   /api/ref/{table}         create; body = the columns; returns the row (201)
//	PATCH  /api/ref/{table}/{id}    partial update; returns the row
//	DELETE /api/ref/{table}/{id}    delete (409 while the row is in use)

import (
	"net/http"

	"packrat/internal/store"
)

func init() {
	register(func(s *Server, mux *http.ServeMux) {
		mux.HandleFunc("GET /api/ref", s.listAllRef)
		mux.HandleFunc("GET /api/ref/{table}", s.listRef)
		mux.HandleFunc("POST /api/ref/{table}", s.createRef)
		mux.HandleFunc("PATCH /api/ref/{table}/{id}", s.updateRef)
		mux.HandleFunc("DELETE /api/ref/{table}/{id}", s.deleteRef)
	})
}

func refTable(r *http.Request) (*store.RefTable, error) {
	t := store.RefTableByKey(r.PathValue("table"))
	if t == nil {
		return nil, store.ErrNotFound
	}
	return t, nil
}

func (s *Server) listAllRef(w http.ResponseWriter, r *http.Request) {
	all, err := s.St.ListAllRef(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, all)
}

func (s *Server) listRef(w http.ResponseWriter, r *http.Request) {
	t, err := refTable(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	rows, err := s.St.ListRef(r.Context(), t)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) createRef(w http.ResponseWriter, r *http.Request) {
	t, err := refTable(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	f, err := decodeFields(w, r)
	if err != nil {
		writeErr(w, err)
		return
	}
	row, err := s.St.CreateRef(r.Context(), t, f)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (s *Server) updateRef(w http.ResponseWriter, r *http.Request) {
	t, err := refTable(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	f, err := decodeFields(w, r)
	if err != nil {
		writeErr(w, err)
		return
	}
	row, err := s.St.UpdateRef(r.Context(), t, id, f)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (s *Server) deleteRef(w http.ResponseWriter, r *http.Request) {
	t, err := refTable(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.St.DeleteRef(r.Context(), t, id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
