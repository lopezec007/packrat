// Package api exposes the store over a JSON HTTP API.
//
// Conventions (docs/api.md has the full reference):
//   - Requests and responses are JSON; errors are {"error": "message"} with
//     400 (invalid request), 404 (no such row), 409 (the data forbids it) or
//     500.
//   - PATCH bodies are partial: absent keys are left alone, an explicit null
//     clears a nullable field.
//
// Each area of the API lives in its own file and adds its routes from an
// init() via register(), so the files do not depend on one another.
package api

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"

	"packrat/internal/store"
)

// Info is what GET /api/version reports.
type Info struct {
	Version string `json:"version"`
	Date    string `json:"date"`
	Build   string `json:"build"`
}

// Server holds what the handlers need.
type Server struct {
	St   *store.Store
	Info Info
}

var registrars []func(*Server, *http.ServeMux)

// register queues a function that adds routes to the mux; call it from init().
func register(f func(*Server, *http.ServeMux)) { registrars = append(registrars, f) }

// Handler returns the router for everything under /api/.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	for _, f := range registrars {
		f(s, mux)
	}
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such API endpoint: " + r.Method + " " + r.URL.Path})
	})
	return s.guard(mux)
}

// guard recovers from handler panics.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				log.Printf("panic in %s %s: %v", r.Method, r.URL.Path, v)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// maxJSONBody bounds JSON request bodies.
const maxJSONBody = 1 << 20

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}

func writeErr(w http.ResponseWriter, err error) {
	var ve *store.ValidationError
	var ce *store.ConflictError
	var tooBig *http.MaxBytesError
	switch {
	case errors.As(err, &ve):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": ve.Msg})
	case errors.As(err, &ce):
		writeJSON(w, http.StatusConflict, map[string]string{"error": ce.Msg})
	case errors.Is(err, store.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	case errors.As(err, &tooBig):
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "request is too large"})
	default:
		log.Printf("internal error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
}

// pathID reads the {id} path value as a positive integer.
func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, store.ErrNotFound
	}
	return id, nil
}

// decode reads a JSON body into v, rejecting unknown fields and trailing data.
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return err
		}
		return store.Invalid("invalid JSON body: %v", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return store.Invalid("invalid JSON body: unexpected data after the object")
	}
	return nil
}

// decodeFields reads a JSON object body as store.Fields (for partial updates).
func decodeFields(w http.ResponseWriter, r *http.Request) (store.Fields, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	var f store.Fields
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&f); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return nil, err
		}
		return nil, store.Invalid("invalid JSON body: %v", err)
	}
	if f == nil {
		return nil, store.Invalid("invalid JSON body: expected an object")
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, store.Invalid("invalid JSON body: unexpected data after the object")
	}
	return f, nil
}

func init() {
	register(func(s *Server, mux *http.ServeMux) {
		mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, s.Info)
		})
	})
}
