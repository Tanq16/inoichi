package server

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/tanq16/inoichi/internal/mindmap"
	"github.com/tanq16/inoichi/internal/storage"
)

//go:embed static
var staticFiles embed.FS

type Server struct {
	host    string
	port    int
	version string
	mux     *http.ServeMux
	store   *storage.Store
}

func New(host string, port int, version string, store *storage.Store) *Server {
	return &Server{host: host, port: port, version: version, mux: http.NewServeMux(), store: store}
}

func (s *Server) Setup() error {
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return err
	}
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/maps", s.handleListMaps)
	s.mux.HandleFunc("POST /api/maps", s.handleCreateMap)
	s.mux.HandleFunc("POST /api/maps/import", s.handleImportMap)
	s.mux.HandleFunc("GET /api/maps/{id}", s.handleGetMap)
	s.mux.HandleFunc("PUT /api/maps/{id}", s.handleSaveMap)
	s.mux.HandleFunc("DELETE /api/maps/{id}", s.handleDeleteMap)
	s.mux.HandleFunc("POST /api/layout", s.handleLayout)

	s.mux.HandleFunc("GET /", s.handleIndex)
	return nil
}

func (s *Server) SeedSample() error {
	n, err := s.store.Count()
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	sample := mindmap.Sample()
	sample.Normalize()
	if err := sample.Validate(); err != nil {
		return err
	}
	if err := s.store.Save(sample); err != nil {
		return err
	}
	log.Info().Str("id", sample.ID).Msg("seeded the sample map into an empty data directory")
	return nil
}

func (s *Server) Run() error {
	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	log.Info().Str("addr", addr).Str("data", s.store.Dir()).Msg("starting")
	return srv.ListenAndServe()
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"version": s.version,
		"dataDir": s.store.Dir(),
	})
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	data, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(data)
}
