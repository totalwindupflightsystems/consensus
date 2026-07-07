// Package chronicle provides the Chronicle Investigation Workbench.
//
// Serves an embedded dark-theme operational dashboard for AI-powered
// investigations — dense data, transparent reasoning, operator-first workflows.
//
// Design system: specs/026-dashboard-ui.md, DESIGN.md, docs/diagrams.md
package chronicle

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/wojons/consensus/chronicle"
)

// Server serves the Chronicle investigation workbench UI.
type Server struct {
	mux    *http.ServeMux
	apiURL string
}

// NewServer creates a new Chronicle UI server.
// apiURL is the base URL of the Consensus API (e.g., "http://localhost:8080").
func NewServer(apiURL string) *Server {
	apiURL = strings.TrimRight(apiURL, "/")
	s := &Server{
		mux:    http.NewServeMux(),
		apiURL: apiURL,
	}

	// Static assets from embedded filesystem
	cssFS, _ := fs.Sub(chronicle.Assets, "css")
	s.mux.Handle("/css/", http.StripPrefix("/css/", http.FileServer(http.FS(cssFS))))

	// Index page
	s.mux.HandleFunc("/", s.handleIndex)
	s.mux.HandleFunc("/health", s.handleHealth)

	return s
}

// Handler returns the http.Handler for mounting under a parent server.
func (s *Server) Handler() http.Handler {
	return s.corsMiddleware(s.mux)
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	index, err := chronicle.Assets.ReadFile("index.html")
	if err != nil {
		http.Error(w, "Chronicle UI not available", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(index)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"healthy":true,"ui":"chronicle","phase":"1.1-design-system"}`))
}
