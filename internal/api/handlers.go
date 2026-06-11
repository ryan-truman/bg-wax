package api

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"

	"backgammon/internal/tickettailor"
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

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	var body struct {
		APIKey    string `json:"api_key"`
		EventName string `json:"event_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.APIKey == "" || body.EventName == "" {
		writeError(w, http.StatusBadRequest, "api_key and event_name are required")
		return
	}

	client := tickettailor.New(body.APIKey)

	event, err := client.FindEventByName(body.EventName)
	if err != nil {
		writeError(w, http.StatusBadGateway, "could not find event: "+err.Error())
		return
	}

	tickets, err := client.TicketsForEvent(event.ID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "could not fetch tickets: "+err.Error())
		return
	}

	for _, table := range []string{"matches", "competitors", "groups", "tournaments"} {
		if _, err := s.db.ExecContext(r.Context(), "DELETE FROM "+table); err != nil {
			writeError(w, http.StatusInternalServerError, "clear failed: "+err.Error())
			return
		}
	}

	tournamentID := newID()
	if _, err := s.db.ExecContext(r.Context(),
		`INSERT INTO tournaments (id, name, status) VALUES (?, ?, 'setup')`,
		tournamentID, event.Name,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "create tournament failed: "+err.Error())
		return
	}

	for _, t := range tickets {
		if _, err := s.db.ExecContext(r.Context(),
			`INSERT INTO competitors (id, tournament_id, name, email, ticket_tailor_id) VALUES (?, ?, ?, ?, ?)`,
			newID(), tournamentID, t.FullName(), t.Email, t.ID,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "insert competitor failed: "+err.Error())
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"count": len(tickets), "tournament": event.Name})
}

func (s *Server) handleClearTournament(w http.ResponseWriter, r *http.Request) {
	for _, table := range []string{"matches", "competitors", "groups", "tournaments"} {
		if _, err := s.db.ExecContext(r.Context(), "DELETE FROM "+table); err != nil {
			writeError(w, http.StatusInternalServerError, "clear failed: "+err.Error())
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAddCompetitor(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not yet implemented")
}

func (s *Server) handleDeleteCompetitor(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not yet implemented")
}

func newID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
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
