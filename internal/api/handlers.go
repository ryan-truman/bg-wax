package api

import (
	"encoding/json"
	"net/http"
)

// --- helpers -----------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// --- health ------------------------------------------------------------------

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- tournament --------------------------------------------------------------

func (s *Server) handleGetTournament(w http.ResponseWriter, r *http.Request) {
	row := s.db.QueryRowContext(r.Context(), `
		SELECT id, name, status, config_json, created_at
		FROM tournaments
		ORDER BY created_at DESC
		LIMIT 1
	`)

	var t struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Status     string `json:"status"`
		ConfigJSON string `json:"config"`
		CreatedAt  string `json:"created_at"`
	}
	if err := row.Scan(&t.ID, &t.Name, &t.Status, &t.ConfigJSON, &t.CreatedAt); err != nil {
		writeError(w, http.StatusNotFound, "no tournament found")
		return
	}

	writeJSON(w, http.StatusOK, t)
}

// --- competitors -------------------------------------------------------------

func (s *Server) handleListCompetitors(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT id, name, email, ticket_tailor_id, seed, group_id
		FROM competitors
		WHERE tournament_id = (SELECT id FROM tournaments ORDER BY created_at DESC LIMIT 1)
		ORDER BY name
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	type Competitor struct {
		ID             string  `json:"id"`
		Name           string  `json:"name"`
		Email          *string `json:"email"`
		TicketTailorID *string `json:"ticket_tailor_id"`
		Seed           *int    `json:"seed"`
		GroupID        *string `json:"group_id"`
	}

	competitors := []Competitor{}
	for rows.Next() {
		var c Competitor
		if err := rows.Scan(&c.ID, &c.Name, &c.Email, &c.TicketTailorID, &c.Seed, &c.GroupID); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		competitors = append(competitors, c)
	}

	writeJSON(w, http.StatusOK, competitors)
}

func (s *Server) handleAddCompetitor(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not yet implemented")
}

func (s *Server) handleDeleteCompetitor(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not yet implemented")
}

// --- groups ------------------------------------------------------------------

func (s *Server) handleListGroups(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not yet implemented")
}

// --- matches -----------------------------------------------------------------

func (s *Server) handleListMatches(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not yet implemented")
}

func (s *Server) handleUpdateMatch(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not yet implemented")
}

// --- tournament actions ------------------------------------------------------

func (s *Server) handleDraw(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not yet implemented")
}

func (s *Server) handleAdvance(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not yet implemented")
}

func (s *Server) handleGetBracket(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not yet implemented")
}
