package api

// Lists and what is on them.
//
//	GET    /api/lists                    every list with its row and status counts
//	POST   /api/lists                    create: {name, is_template, sources: [list ids to combine]}; returns the list with its contents (201)
//	POST   /api/lists/preview            {sources: [list ids]} -> {rows: n}: how many entries a list made from them would have
//	GET    /api/lists/{id}               one list with its contents
//	PATCH  /api/lists/{id}               {name, is_template}; returns the list with its contents
//	DELETE /api/lists/{id}               delete the list and everything on it
//	POST   /api/lists/{id}/statuses      {status_id}: give every row that status; returns the list with its contents
//	POST   /api/lists/{id}/contents      add an item: {item_id, container_id, compartment_id, quantity, unit, status_id, notes}; returns the row (201)
//	PATCH  /api/contents/{id}            {container_id, compartment_id, quantity, unit, status_id, notes}; returns the row
//	DELETE /api/contents/{id}            remove a row; ?contents=keep moves what is inside it up instead of removing it too

import (
	"net/http"

	"packrat/internal/store"
)

func init() {
	register(func(s *Server, mux *http.ServeMux) {
		mux.HandleFunc("GET /api/lists", s.listLists)
		mux.HandleFunc("POST /api/lists", s.createList)
		mux.HandleFunc("POST /api/lists/preview", s.previewList)
		mux.HandleFunc("GET /api/lists/{id}", s.getList)
		mux.HandleFunc("PATCH /api/lists/{id}", s.updateList)
		mux.HandleFunc("DELETE /api/lists/{id}", s.deleteList)
		mux.HandleFunc("POST /api/lists/{id}/statuses", s.setAllStatuses)
		mux.HandleFunc("POST /api/lists/{id}/contents", s.addContent)
		mux.HandleFunc("PATCH /api/contents/{id}", s.updateContent)
		mux.HandleFunc("DELETE /api/contents/{id}", s.deleteContent)
	})
}

func (s *Server) listLists(w http.ResponseWriter, r *http.Request) {
	lists, err := s.St.ListLists(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lists)
}

func (s *Server) createList(w http.ResponseWriter, r *http.Request) {
	var n store.NewList
	if err := decode(w, r, &n); err != nil {
		writeErr(w, err)
		return
	}
	list, err := s.St.CreateList(r.Context(), n)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, list)
}

func (s *Server) previewList(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Sources []int64 `json:"sources"`
	}
	if err := decode(w, r, &body); err != nil {
		writeErr(w, err)
		return
	}
	p, err := s.St.PreviewMerge(r.Context(), body.Sources)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) getList(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	list, err := s.St.GetList(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) updateList(w http.ResponseWriter, r *http.Request) {
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
	list, err := s.St.UpdateList(r.Context(), id, f)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) deleteList(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.St.DeleteList(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) setAllStatuses(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var body struct {
		StatusID *int64 `json:"status_id"`
	}
	if err := decode(w, r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if body.StatusID == nil {
		writeErr(w, store.Invalid("status_id is required"))
		return
	}
	list, err := s.St.SetAllStatuses(r.Context(), id, *body.StatusID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) addContent(w http.ResponseWriter, r *http.Request) {
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
	row, err := s.St.AddContent(r.Context(), id, f)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (s *Server) updateContent(w http.ResponseWriter, r *http.Request) {
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
	row, err := s.St.UpdateContent(r.Context(), id, f)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (s *Server) deleteContent(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	keep := false
	switch r.URL.Query().Get("contents") {
	case "", "remove":
	case "keep":
		keep = true
	default:
		writeErr(w, store.Invalid(`contents must be "remove" or "keep"`))
		return
	}
	if err := s.St.DeleteContent(r.Context(), id, keep); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
