package api

import (
	"database/sql"
	"io/fs"
	"net/http"
	"strings"
)

// Server holds shared dependencies and implements http.Handler.
type Server struct {
	db  *sql.DB
	mux *http.ServeMux
}

// NewServer wires up all routes. frontend is the embedded React build served
// at the root; pass nil to skip frontend serving (useful in tests).
func NewServer(db *sql.DB, frontend fs.FS) *Server {
	s := &Server{db: db, mux: http.NewServeMux()}
	s.routes(frontend)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes(frontend fs.FS) {
	s.mux.HandleFunc("GET /health", s.handleHealth)

	s.mux.HandleFunc("GET /api/tournament", s.handleGetTournament)
	s.mux.HandleFunc("POST /api/tournament/import", s.handleImport)
	s.mux.HandleFunc("POST /api/tournament/clear", s.handleClearTournament)

	s.mux.HandleFunc("GET /api/competitors", s.handleListCompetitors)
	s.mux.HandleFunc("GET /api/competitors/removed", s.handleListRemovedCompetitors)
	s.mux.HandleFunc("POST /api/competitors", s.handleAddCompetitor)
	s.mux.HandleFunc("DELETE /api/competitors/{id}", s.handleDeleteCompetitor)
	s.mux.HandleFunc("POST /api/competitors/{id}/restore", s.handleRestoreCompetitor)

	s.mux.HandleFunc("GET /api/groups", s.handleListGroups)

	s.mux.HandleFunc("GET /api/matches", s.handleListMatches)
	s.mux.HandleFunc("PATCH /api/matches/{id}", s.handleUpdateMatch)

	s.mux.HandleFunc("POST /api/tournament/draw", s.handleDraw)
	s.mux.HandleFunc("POST /api/tournament/advance", s.handleAdvance)

	s.mux.HandleFunc("GET /api/bracket", s.handleGetBracket)

	if frontend != nil {
		s.mux.Handle("/", spaHandler(frontend))
	}
}

// spaHandler serves static files and falls back to index.html for unknown
// paths so that React Router can handle client-side navigation.
func spaHandler(fsys fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(fsys))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		_, err := fsys.Open(path)
		if err != nil {
			http.ServeFileFS(w, r, fsys, "index.html")
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}
