package server

import (
	"encoding/json/v2"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/tanq16/inoichi/internal/mindmap"
	"github.com/tanq16/inoichi/internal/storage"
)

type errorBody struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	if err := json.MarshalWrite(w, payload); err != nil {
		log.Error().Err(err).Msg("failed to write the response body")
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorBody{Error: msg})
}

func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	if !sameOrigin(r) {
		writeError(w, http.StatusForbidden, "Cross-origin requests are not accepted.")
		return false
	}
	body := http.MaxBytesReader(w, r.Body, mindmap.MaxDocumentBytes)
	if err := json.UnmarshalRead(body, dst); err != nil {
		if _, tooBig := errors.AsType[*http.MaxBytesError](err); tooBig {
			writeError(w, http.StatusRequestEntityTooLarge, "That map is larger than Inoichi stores.")
			return false
		}
		writeError(w, http.StatusBadRequest, "The request body is not valid JSON for this endpoint.")
		return false
	}
	return true
}

// A cross-origin simple POST arrives with no preflight, so the mutating routes refuse a foreign Origin themselves.
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return u.Host == r.Host
}

func (s *Server) handleListMaps(w http.ResponseWriter, r *http.Request) {
	summaries, unreadable, err := s.store.List()
	if err != nil {
		log.Error().Err(err).Msg("failed to list maps")
		writeError(w, http.StatusInternalServerError, "Could not read the maps directory.")
		return
	}
	for _, name := range unreadable {
		log.Warn().Str("file", name).Msg("skipped a map file that did not parse")
	}
	writeJSON(w, http.StatusOK, summaries)
}

func (s *Server) handleCreateMap(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title string `json:"title"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	title := strings.TrimSpace(body.Title)
	if title == "" {
		writeError(w, http.StatusUnprocessableEntity, "Give the map a title.")
		return
	}
	m := mindmap.New(title)
	m.Normalize()
	if err := m.Validate(); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err := s.store.Save(m); errors.Is(err, storage.ErrTooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, "That map is larger than Inoichi stores.")
		return
	} else if err != nil {
		log.Error().Err(err).Str("id", m.ID).Msg("failed to save a new map")
		writeError(w, http.StatusInternalServerError, "Could not write the map to disk.")
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

func (s *Server) handleImportMap(w http.ResponseWriter, r *http.Request) {
	var incoming mindmap.Map
	if !decodeBody(w, r, &incoming) {
		return
	}
	incoming.ID = mindmap.NewID()
	incoming.Sample = false
	incoming.Normalize()
	if err := incoming.Validate(); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "That file is not a map Inoichi can read: "+err.Error())
		return
	}
	if err := s.store.Save(&incoming); errors.Is(err, storage.ErrTooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, "That map is larger than Inoichi stores.")
		return
	} else if err != nil {
		log.Error().Err(err).Str("id", incoming.ID).Msg("failed to save an imported map")
		writeError(w, http.StatusInternalServerError, "Could not write the imported map to disk.")
		return
	}
	writeJSON(w, http.StatusCreated, &incoming)
}

func (s *Server) handleGetMap(w http.ResponseWriter, r *http.Request) {
	m, ok := s.loadOr404(w, r.PathValue("id"))
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) handleSaveMap(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, ok := s.loadOr404(w, id)
	if !ok {
		return
	}
	var incoming mindmap.Map
	if !decodeBody(w, r, &incoming) {
		return
	}
	if !incoming.UpdatedAt.IsZero() && !incoming.UpdatedAt.Equal(existing.UpdatedAt) {
		writeError(w, http.StatusConflict, "This map changed elsewhere since you opened it. Reload before saving, or export your copy first.")
		return
	}
	incoming.ID = id
	incoming.CreatedAt = existing.CreatedAt
	incoming.Sample = existing.Sample
	incoming.Normalize()
	if err := incoming.Validate(); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err := s.store.Save(&incoming); errors.Is(err, storage.ErrTooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, "That map is larger than Inoichi stores.")
		return
	} else if err != nil {
		log.Error().Err(err).Str("id", id).Msg("failed to save a map")
		writeError(w, http.StatusInternalServerError, "Could not write the map to disk.")
		return
	}
	writeJSON(w, http.StatusOK, &incoming)
}

func (s *Server) handleDeleteMap(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !mindmap.ValidID(id) {
		writeError(w, http.StatusBadRequest, "That is not a valid map id.")
		return
	}
	if err := s.store.Delete(id); errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "No map with that id.")
		return
	} else if err != nil {
		log.Error().Err(err).Str("id", id).Msg("failed to delete a map")
		writeError(w, http.StatusInternalServerError, "Could not delete the map file.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleLayout(w http.ResponseWriter, r *http.Request) {
	var incoming mindmap.Map
	if !decodeBody(w, r, &incoming) {
		return
	}
	incoming.Normalize()
	if err := incoming.Validate(); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	mindmap.Layout(&incoming)
	writeJSON(w, http.StatusOK, &incoming)
}

func (s *Server) loadOr404(w http.ResponseWriter, id string) (*mindmap.Map, bool) {
	if !mindmap.ValidID(id) {
		writeError(w, http.StatusBadRequest, "That is not a valid map id.")
		return nil, false
	}
	m, err := s.store.Load(id)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "No map with that id.")
		return nil, false
	}
	if err != nil {
		log.Error().Err(err).Str("id", id).Msg("failed to load a map")
		writeError(w, http.StatusInternalServerError, "Could not read the map file.")
		return nil, false
	}
	return m, true
}
