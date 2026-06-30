package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	mathrand "math/rand"
	"net/http"
	"sort"

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

// withTx runs fn inside a transaction, committing if it returns nil and rolling
// back on any error or panic. Use it for handlers that issue several writes that
// must all land together (a draw, an advance, an import).
func (s *Server) withTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() // no-op once Commit succeeds
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
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
			           AND (m.player1_id = c.id OR m.player2_id = c.id) THEN 1 END) AS losses,
			SUM(CASE WHEN m.player1_id = c.id THEN COALESCE(m.player1_score, 0)
			         WHEN m.player2_id = c.id THEN COALESCE(m.player2_score, 0)
			         ELSE 0 END) AS points
		FROM competitors c
		LEFT JOIN matches m ON (m.player1_id = c.id OR m.player2_id = c.id)
		WHERE c.tournament_id = (SELECT id FROM tournaments ORDER BY created_at DESC LIMIT 1)
		  AND c.removed = 0
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
		if err := rows.Scan(&c.ID, &c.Name, &c.Email, &c.TicketTailorID, &c.Seed, &c.GroupID, &c.Wins, &c.Losses, &c.Points); err != nil {
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

	// Replace the existing tournament wholesale. Wrapped in a transaction so a
	// mid-import failure can't leave the tables half-cleared or half-populated.
	tournamentID := newID()
	if err := s.withTx(r.Context(), func(tx *sql.Tx) error {
		for _, table := range []string{"matches", "competitors", "groups", "tournaments"} {
			if _, err := tx.ExecContext(r.Context(), "DELETE FROM "+table); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(r.Context(),
			`INSERT INTO tournaments (id, name, status) VALUES (?, ?, 'setup')`,
			tournamentID, event.Name,
		); err != nil {
			return err
		}
		for _, t := range tickets {
			if _, err := tx.ExecContext(r.Context(),
				`INSERT INTO competitors (id, tournament_id, name, email, ticket_tailor_id) VALUES (?, ?, ?, ?, ?)`,
				newID(), tournamentID, t.FullName(), t.Email, t.ID,
			); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "import failed: "+err.Error())
		return
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
	ctx := r.Context()
	id := r.PathValue("id")

	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT 1 FROM competitors WHERE id = ?`, id).Scan(&exists); err != nil {
		writeError(w, http.StatusNotFound, "competitor not found")
		return
	}

	if _, err := s.db.ExecContext(ctx, `UPDATE competitors SET removed = 1 WHERE id = ?`, id); err != nil {
		writeError(w, http.StatusInternalServerError, "remove failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListRemovedCompetitors(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT id, name
		FROM competitors
		WHERE tournament_id = (SELECT id FROM tournaments ORDER BY created_at DESC LIMIT 1)
		  AND removed = 1
		ORDER BY name
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	competitors := []RemovedCompetitor{}
	for rows.Next() {
		var c RemovedCompetitor
		if err := rows.Scan(&c.ID, &c.Name); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		competitors = append(competitors, c)
	}
	writeJSON(w, http.StatusOK, competitors)
}

func (s *Server) handleRestoreCompetitor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")

	if _, err := s.db.ExecContext(ctx, `UPDATE competitors SET removed = 0 WHERE id = ?`, id); err != nil {
		writeError(w, http.StatusInternalServerError, "restore failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func newID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// --- groups ------------------------------------------------------------------

// groupStandings tallies completed matches for a group and returns its
// competitors ranked best-first (most points, then most wins). Removed
// competitors are excluded.
func (s *Server) groupStandings(ctx context.Context, groupID string) ([]CompetitorStanding, error) {
	cRows, err := s.db.QueryContext(ctx, `
		SELECT id, name FROM competitors WHERE group_id = ? AND removed = 0 ORDER BY name
	`, groupID)
	if err != nil {
		return nil, err
	}
	standings := map[string]*CompetitorStanding{}
	var orderedIDs []string
	for cRows.Next() {
		var st CompetitorStanding
		if err := cRows.Scan(&st.ID, &st.Name); err != nil {
			cRows.Close()
			return nil, err
		}
		standings[st.ID] = &st
		orderedIDs = append(orderedIDs, st.ID)
	}
	cRows.Close()

	mRows, err := s.db.QueryContext(ctx, `
		SELECT player1_id, player2_id, winner_id, player1_score, player2_score
		FROM matches WHERE group_id = ? AND status = 'complete'
	`, groupID)
	if err != nil {
		return nil, err
	}
	for mRows.Next() {
		var p1ID, p2ID string
		var winnerID *string
		var p1Score, p2Score *int
		if err := mRows.Scan(&p1ID, &p2ID, &winnerID, &p1Score, &p2Score); err != nil {
			mRows.Close()
			return nil, err
		}
		if p1, ok := standings[p1ID]; ok {
			p1.Played++
			if winnerID != nil && *winnerID == p1ID {
				p1.Won++
				if p1Score != nil {
					p1.Points += *p1Score
				}
			} else if winnerID != nil {
				p1.Lost++
			}
		}
		if p2, ok := standings[p2ID]; ok {
			p2.Played++
			if winnerID != nil && *winnerID == p2ID {
				p2.Won++
				if p2Score != nil {
					p2.Points += *p2Score
				}
			} else if winnerID != nil {
				p2.Lost++
			}
		}
	}
	mRows.Close()

	result := make([]CompetitorStanding, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		result = append(result, *standings[id])
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Points != result[j].Points {
			return result[i].Points > result[j].Points
		}
		return result[i].Won > result[j].Won
	})
	return result, nil
}

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

	type groupRow struct {
		id, name string
	}
	var groupList []groupRow
	for groupRows.Next() {
		var gr groupRow
		if err := groupRows.Scan(&gr.id, &gr.name); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		groupList = append(groupList, gr)
	}
	groupRows.Close()

	groups := []Group{}
	for _, gr := range groupList {
		standings, err := s.groupStandings(ctx, gr.id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "standings failed")
			return
		}
		if standings == nil {
			standings = []CompetitorStanding{}
		}
		groups = append(groups, Group{ID: gr.id, Name: gr.name, Competitors: standings})
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
		  AND m.player1_id IS NOT NULL AND m.player2_id IS NOT NULL
		  AND p1.removed = 0 AND p2.removed = 0
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
		Points   int    `json:"points"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.WinnerID == "" {
		writeError(w, http.StatusBadRequest, "winner_id and points are required")
		return
	}
	if body.Points < 1 || body.Points > 3 {
		writeError(w, http.StatusBadRequest, "points must be 1, 2, or 3")
		return
	}

	// Confirm winner_id is one of the two players.
	var p1ID, p2ID, stage, status, tournamentID string
	var round, position *int
	if err := s.db.QueryRowContext(ctx, `
		SELECT player1_id, player2_id, stage, round, position, status, tournament_id FROM matches WHERE id = ?
	`, id).Scan(&p1ID, &p2ID, &stage, &round, &position, &status, &tournamentID); err != nil {
		writeError(w, http.StatusNotFound, "match not found")
		return
	}
	// A completed knockout match has already fed its winner into the next round;
	// re-recording it with a different winner would fork the bracket below it.
	if status == "complete" {
		writeError(w, http.StatusConflict, "match result already recorded")
		return
	}
	if body.WinnerID != p1ID && body.WinnerID != p2ID {
		writeError(w, http.StatusBadRequest, "winner_id must be one of the two players")
		return
	}

	p1Score, p2Score := 0, 0
	if body.WinnerID == p1ID {
		p1Score = body.Points
	} else {
		p2Score = body.Points
	}

	if err := s.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			UPDATE matches SET winner_id = ?, player1_score = ?, player2_score = ?, status = 'complete' WHERE id = ?
		`, body.WinnerID, p1Score, p2Score, id); err != nil {
			return err
		}

		// Knockout matches feed their winner into the next round. The final
		// (round 1) completes the tournament.
		if stage == "knockout" && round != nil && position != nil {
			if *round > 1 {
				col := "player1_id"
				if *position%2 == 1 {
					col = "player2_id"
				}
				if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
					UPDATE matches SET %s = ?
					WHERE tournament_id = ? AND stage = 'knockout' AND round = ? AND position = ?
				`, col), body.WinnerID, tournamentID, *round-1, *position/2); err != nil {
					return err
				}
			} else {
				if _, err := tx.ExecContext(ctx, `UPDATE tournaments SET status = 'complete' WHERE id = ?`, tournamentID); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
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

func groupName(i int) string {
	return fmt.Sprintf("Group %c", 'A'+i)
}

func (s *Server) handleDraw(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var body struct {
		NumGroups int `json:"num_groups"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.NumGroups < 2 || body.NumGroups > 26 {
		body.NumGroups = 8
	}

	var tournamentID string
	if err := s.db.QueryRowContext(ctx, `
		SELECT id FROM tournaments WHERE status = 'setup' ORDER BY created_at DESC LIMIT 1
	`).Scan(&tournamentID); err != nil {
		writeError(w, http.StatusBadRequest, "no tournament in setup state")
		return
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id FROM competitors WHERE tournament_id = ? AND removed = 0
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

	total := len(competitorIDs)
	if total%body.NumGroups != 0 {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("%d competitors cannot be divided evenly into %d groups", total, body.NumGroups))
		return
	}
	groupSize := total / body.NumGroups

	mathrand.Shuffle(len(competitorIDs), func(i, j int) {
		competitorIDs[i], competitorIDs[j] = competitorIDs[j], competitorIDs[i]
	})

	if err := s.withTx(ctx, func(tx *sql.Tx) error {
		for i := 0; i < body.NumGroups; i++ {
			groupID := newID()
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO groups (id, tournament_id, name) VALUES (?, ?, ?)
			`, groupID, tournamentID, groupName(i)); err != nil {
				return err
			}

			members := competitorIDs[i*groupSize : (i+1)*groupSize]

			for _, cID := range members {
				if _, err := tx.ExecContext(ctx, `
					UPDATE competitors SET group_id = ? WHERE id = ?
				`, groupID, cID); err != nil {
					return err
				}
			}

			// Round-robin: every pair plays once
			for a := 0; a < len(members); a++ {
				for b := a + 1; b < len(members); b++ {
					if _, err := tx.ExecContext(ctx, `
						INSERT INTO matches (id, tournament_id, stage, group_id, player1_id, player2_id, status)
						VALUES (?, ?, 'group', ?, ?, ?, 'pending')
					`, newID(), tournamentID, groupID, members[a], members[b]); err != nil {
						return err
					}
				}
			}
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE tournaments SET status = 'group_stage' WHERE id = ?
		`, tournamentID); err != nil {
			return err
		}
		return nil
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "draw failed: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// advanceCount is how many competitors advance from each group to the knockout.
const advanceCount = 2

// nextPow2 returns the smallest power of two >= n.
func nextPow2(n int) int {
	p := 1
	for p < n {
		p <<= 1
	}
	return p
}

// seedPositions returns the seed numbers (1-based) ordered from the top of a
// single-elimination bracket to the bottom, so that top seeds are spread apart.
// size must be a power of two. e.g. size 4 -> [1 4 2 3], size 8 -> [1 8 4 5 2 7 3 6].
func seedPositions(size int) []int {
	order := []int{1}
	for len(order) < size {
		sz := len(order) * 2
		next := make([]int, 0, sz)
		for _, s := range order {
			next = append(next, s, sz+1-s)
		}
		order = next
	}
	return order
}

func (s *Server) handleAdvance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var tournamentID string
	if err := s.db.QueryRowContext(ctx, `
		SELECT id FROM tournaments WHERE status = 'group_stage' ORDER BY created_at DESC LIMIT 1
	`).Scan(&tournamentID); err != nil {
		writeError(w, http.StatusBadRequest, "no tournament in group stage")
		return
	}

	groupRows, err := s.db.QueryContext(ctx, `
		SELECT id FROM groups WHERE tournament_id = ? ORDER BY name
	`, tournamentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	var groupIDs []string
	for groupRows.Next() {
		var gid string
		if err := groupRows.Scan(&gid); err != nil {
			groupRows.Close()
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		groupIDs = append(groupIDs, gid)
	}
	groupRows.Close()

	if len(groupIDs) == 0 {
		writeError(w, http.StatusBadRequest, "no groups to advance from")
		return
	}

	// Collect qualifiers bucketed by their finishing place in the group, so we
	// can seed all group winners ahead of all runners-up.
	type qual struct {
		id     string
		points int
		won    int
	}
	buckets := make([][]qual, advanceCount)
	for _, gid := range groupIDs {
		standings, err := s.groupStandings(ctx, gid)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "standings failed")
			return
		}
		for place := 0; place < advanceCount && place < len(standings); place++ {
			buckets[place] = append(buckets[place], qual{standings[place].ID, standings[place].Points, standings[place].Won})
		}
	}

	var qualifiers []string
	for place := 0; place < advanceCount; place++ {
		b := buckets[place]
		sort.SliceStable(b, func(i, j int) bool {
			if b[i].points != b[j].points {
				return b[i].points > b[j].points
			}
			return b[i].won > b[j].won
		})
		for _, q := range b {
			qualifiers = append(qualifiers, q.id)
		}
	}

	if len(qualifiers) < 2 {
		writeError(w, http.StatusBadRequest, "not enough qualifiers to form a bracket")
		return
	}

	bracketSize := nextPow2(len(qualifiers))
	firstRound := 0
	for (1 << firstRound) < bracketSize {
		firstRound++
	}

	if err := s.withTx(ctx, func(tx *sql.Tx) error {
		// Pre-create skeleton matches for every round after the first, so the full
		// bracket tree exists with TBD slots that later winners feed into.
		parentMatch := map[int]map[int]string{}
		for round := firstRound - 1; round >= 1; round-- {
			parentMatch[round] = map[int]string{}
			for pos := 0; pos < (1 << (round - 1)); pos++ {
				mid := newID()
				if _, err := tx.ExecContext(ctx, `
					INSERT INTO matches (id, tournament_id, stage, round, position, status)
					VALUES (?, ?, 'knockout', ?, ?, 'pending')
				`, mid, tournamentID, round, pos); err != nil {
					return err
				}
				parentMatch[round][pos] = mid
			}
		}

		// Seed the first round. A seed number beyond the qualifier count is a bye —
		// the present player advances straight into the next round.
		order := seedPositions(bracketSize)
		for j := 0; j < bracketSize/2; j++ {
			seedA, seedB := order[2*j], order[2*j+1]
			var pA, pB *string
			if seedA <= len(qualifiers) {
				pA = &qualifiers[seedA-1]
			}
			if seedB <= len(qualifiers) {
				pB = &qualifiers[seedB-1]
			}

			if pA != nil && pB != nil {
				if _, err := tx.ExecContext(ctx, `
					INSERT INTO matches (id, tournament_id, stage, round, position, player1_id, player2_id, status)
					VALUES (?, ?, 'knockout', ?, ?, ?, ?, 'pending')
				`, newID(), tournamentID, firstRound, j, *pA, *pB); err != nil {
					return err
				}
				continue
			}

			winner := pA
			if winner == nil {
				winner = pB
			}
			col := "player1_id"
			if j%2 == 1 {
				col = "player2_id"
			}
			if _, err := tx.ExecContext(ctx,
				fmt.Sprintf(`UPDATE matches SET %s = ? WHERE id = ?`, col),
				*winner, parentMatch[firstRound-1][j/2]); err != nil {
				return err
			}
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE tournaments SET status = 'knockout' WHERE id = ?
		`, tournamentID); err != nil {
			return err
		}
		return nil
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "advance failed: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetBracket(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			m.id, m.stage, m.group_id, m.round, m.position,
			m.player1_id, p1.name, m.player2_id, p2.name,
			m.winner_id, m.player1_score, m.player2_score, m.status
		FROM matches m
		LEFT JOIN competitors p1 ON p1.id = m.player1_id
		LEFT JOIN competitors p2 ON p2.id = m.player2_id
		WHERE m.stage = 'knockout'
		  AND m.tournament_id = (SELECT id FROM tournaments ORDER BY created_at DESC LIMIT 1)
		ORDER BY m.round DESC, m.position
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
