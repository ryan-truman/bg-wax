package api

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	mathrand "math/rand"
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

	var t Tournament
	if err := row.Scan(&t.ID, &t.Name, &t.Status, &t.Config, &t.CreatedAt); err != nil {
		writeError(w, http.StatusNotFound, "no tournament found")
		return
	}

	writeJSON(w, http.StatusOK, t)
}

// --- competitors -------------------------------------------------------------

func (s *Server) handleListCompetitors(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT
			c.id, c.name, c.email, c.ticket_tailor_id, c.seed, c.group_id,
			COUNT(CASE WHEN m.winner_id = c.id THEN 1 END) AS wins,
			COUNT(CASE WHEN m.status = 'complete' AND m.winner_id != c.id
			           AND (m.player1_id = c.id OR m.player2_id = c.id) THEN 1 END) AS losses
		FROM competitors c
		LEFT JOIN matches m ON (m.player1_id = c.id OR m.player2_id = c.id)
		WHERE c.tournament_id = (SELECT id FROM tournaments ORDER BY created_at DESC LIMIT 1)
		GROUP BY c.id
		ORDER BY c.name
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	competitors := []Competitor{}
	for rows.Next() {
		var c Competitor
		if err := rows.Scan(&c.ID, &c.Name, &c.Email, &c.TicketTailorID, &c.Seed, &c.GroupID, &c.Wins, &c.Losses); err != nil {
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
	ctx := r.Context()

	var tournamentID string
	if err := s.db.QueryRowContext(ctx, `
		SELECT id FROM tournaments ORDER BY created_at DESC LIMIT 1
	`).Scan(&tournamentID); err != nil {
		writeError(w, http.StatusNotFound, "no tournament found")
		return
	}

	groupRows, err := s.db.QueryContext(ctx, `
		SELECT id, name FROM groups WHERE tournament_id = ? ORDER BY name
	`, tournamentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer groupRows.Close()

	var groups []Group
	for groupRows.Next() {
		var g Group
		if err := groupRows.Scan(&g.ID, &g.Name); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		g.Competitors = []CompetitorStanding{}
		groups = append(groups, g)
	}
	groupRows.Close()

	for i, g := range groups {
		cRows, err := s.db.QueryContext(ctx, `
			SELECT id, name FROM competitors WHERE group_id = ? ORDER BY name
		`, g.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "query failed")
			return
		}
		standings := map[string]*CompetitorStanding{}
		var orderedIDs []string
		for cRows.Next() {
			var cs CompetitorStanding
			if err := cRows.Scan(&cs.ID, &cs.Name); err != nil {
				cRows.Close()
				writeError(w, http.StatusInternalServerError, "scan failed")
				return
			}
			standings[cs.ID] = &cs
			orderedIDs = append(orderedIDs, cs.ID)
		}
		cRows.Close()

		mRows, err := s.db.QueryContext(ctx, `
			SELECT player1_id, player2_id, winner_id
			FROM matches WHERE group_id = ? AND status = 'complete'
		`, g.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "query failed")
			return
		}
		for mRows.Next() {
			var p1ID, p2ID string
			var winnerID *string
			if err := mRows.Scan(&p1ID, &p2ID, &winnerID); err != nil {
				mRows.Close()
				writeError(w, http.StatusInternalServerError, "scan failed")
				return
			}
			if p1, ok := standings[p1ID]; ok {
				p1.Played++
				if winnerID != nil && *winnerID == p1ID {
					p1.Won++
					p1.Points += 2
				} else if winnerID != nil {
					p1.Lost++
				}
			}
			if p2, ok := standings[p2ID]; ok {
				p2.Played++
				if winnerID != nil && *winnerID == p2ID {
					p2.Won++
					p2.Points += 2
				} else if winnerID != nil {
					p2.Lost++
				}
			}
		}
		mRows.Close()

		for _, id := range orderedIDs {
			groups[i].Competitors = append(groups[i].Competitors, *standings[id])
		}
	}

	if groups == nil {
		groups = []Group{}
	}
	writeJSON(w, http.StatusOK, groups)
}

// --- matches -----------------------------------------------------------------

func (s *Server) handleListMatches(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			m.id, m.stage, m.group_id, m.round, m.position,
			m.player1_id, p1.name, m.player2_id, p2.name,
			m.winner_id, m.player1_score, m.player2_score, m.status
		FROM matches m
		LEFT JOIN competitors p1 ON p1.id = m.player1_id
		LEFT JOIN competitors p2 ON p2.id = m.player2_id
		WHERE m.tournament_id = (SELECT id FROM tournaments ORDER BY created_at DESC LIMIT 1)
		ORDER BY m.stage, m.group_id, m.round, m.position
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	matches := []Match{}
	for rows.Next() {
		var m Match
		if err := rows.Scan(
			&m.ID, &m.Stage, &m.GroupID, &m.Round, &m.Position,
			&m.Player1ID, &m.Player1Name, &m.Player2ID, &m.Player2Name,
			&m.WinnerID, &m.Player1Score, &m.Player2Score, &m.Status,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		matches = append(matches, m)
	}
	writeJSON(w, http.StatusOK, matches)
}

func (s *Server) handleUpdateMatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")

	var body struct {
		WinnerID string `json:"winner_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.WinnerID == "" {
		writeError(w, http.StatusBadRequest, "winner_id is required")
		return
	}

	// Confirm winner_id is one of the two players.
	var p1ID, p2ID string
	if err := s.db.QueryRowContext(ctx, `
		SELECT player1_id, player2_id FROM matches WHERE id = ?
	`, id).Scan(&p1ID, &p2ID); err != nil {
		writeError(w, http.StatusNotFound, "match not found")
		return
	}
	if body.WinnerID != p1ID && body.WinnerID != p2ID {
		writeError(w, http.StatusBadRequest, "winner_id must be one of the two players")
		return
	}

	if _, err := s.db.ExecContext(ctx, `
		UPDATE matches SET winner_id = ?, status = 'complete' WHERE id = ?
	`, body.WinnerID, id); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}

	// Return the updated match with player names.
	var m Match
	if err := s.db.QueryRowContext(ctx, `
		SELECT
			m.id, m.stage, m.group_id, m.round, m.position,
			m.player1_id, p1.name, m.player2_id, p2.name,
			m.winner_id, m.player1_score, m.player2_score, m.status
		FROM matches m
		LEFT JOIN competitors p1 ON p1.id = m.player1_id
		LEFT JOIN competitors p2 ON p2.id = m.player2_id
		WHERE m.id = ?
	`, id).Scan(
		&m.ID, &m.Stage, &m.GroupID, &m.Round, &m.Position,
		&m.Player1ID, &m.Player1Name, &m.Player2ID, &m.Player2Name,
		&m.WinnerID, &m.Player1Score, &m.Player2Score, &m.Status,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "fetch failed")
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// --- tournament actions ------------------------------------------------------

const numGroups = 8
const groupSize = 5

var groupNames = []string{"Group A", "Group B", "Group C", "Group D", "Group E", "Group F", "Group G", "Group H"}

func (s *Server) handleDraw(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var tournamentID string
	if err := s.db.QueryRowContext(ctx, `
		SELECT id FROM tournaments WHERE status = 'setup' ORDER BY created_at DESC LIMIT 1
	`).Scan(&tournamentID); err != nil {
		writeError(w, http.StatusBadRequest, "no tournament in setup state")
		return
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id FROM competitors WHERE tournament_id = ?
	`, tournamentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	var competitorIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		competitorIDs = append(competitorIDs, id)
	}
	rows.Close()

	required := numGroups * groupSize
	if len(competitorIDs) != required {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("expected %d competitors for %d groups of %d, got %d", required, numGroups, groupSize, len(competitorIDs)))
		return
	}

	mathrand.Shuffle(len(competitorIDs), func(i, j int) {
		competitorIDs[i], competitorIDs[j] = competitorIDs[j], competitorIDs[i]
	})

	for i := 0; i < numGroups; i++ {
		groupID := newID()
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO groups (id, tournament_id, name) VALUES (?, ?, ?)
		`, groupID, tournamentID, groupNames[i]); err != nil {
			writeError(w, http.StatusInternalServerError, "create group failed")
			return
		}

		members := competitorIDs[i*groupSize : (i+1)*groupSize]

		for _, cID := range members {
			if _, err := s.db.ExecContext(ctx, `
				UPDATE competitors SET group_id = ? WHERE id = ?
			`, groupID, cID); err != nil {
				writeError(w, http.StatusInternalServerError, "assign group failed")
				return
			}
		}

		// Round-robin: every pair plays once
		for a := 0; a < len(members); a++ {
			for b := a + 1; b < len(members); b++ {
				if _, err := s.db.ExecContext(ctx, `
					INSERT INTO matches (id, tournament_id, stage, group_id, player1_id, player2_id, status)
					VALUES (?, ?, 'group', ?, ?, ?, 'pending')
				`, newID(), tournamentID, groupID, members[a], members[b]); err != nil {
					writeError(w, http.StatusInternalServerError, "create match failed")
					return
				}
			}
		}
	}

	if _, err := s.db.ExecContext(ctx, `
		UPDATE tournaments SET status = 'group_stage' WHERE id = ?
	`, tournamentID); err != nil {
		writeError(w, http.StatusInternalServerError, "update status failed")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdvance(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not yet implemented")
}

func (s *Server) handleGetBracket(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not yet implemented")
}
