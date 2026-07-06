package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	mathrand "math/rand"
	"net/http"
	"os"
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

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, Config{Demo: os.Getenv("DEMO_MODE") == "1"})
}

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

// handleListTicketTailorEvents lists the events on the Ticket Tailor account
// so the frontend can offer a picker. POST because the API key travels in the
// request body rather than the URL.
func (s *Server) handleListTicketTailorEvents(w http.ResponseWriter, r *http.Request) {
	var body struct {
		APIKey string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.APIKey == "" {
		writeError(w, http.StatusBadRequest, "api_key is required")
		return
	}

	events := tickettailor.DemoEvents
	if body.APIKey != tickettailor.DemoKey {
		var err error
		events, err = tickettailor.New(body.APIKey).ListEvents()
		if err != nil {
			writeError(w, http.StatusBadGateway, "could not fetch events: "+err.Error())
			return
		}
	}

	out := make([]TicketTailorEvent, 0, len(events))
	for _, e := range events {
		out = append(out, TicketTailorEvent{ID: e.ID, Name: e.Name})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	var body struct {
		APIKey  string `json:"api_key"`
		EventID string `json:"event_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.APIKey == "" || body.EventID == "" {
		writeError(w, http.StatusBadRequest, "api_key and event_id are required")
		return
	}

	var (
		event   *tickettailor.Event
		tickets []tickettailor.IssuedTicket
	)
	if body.APIKey == tickettailor.DemoKey {
		var err error
		event, tickets, err = tickettailor.DemoEvent(body.EventID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	} else {
		client := tickettailor.New(body.APIKey)

		var err error
		event, err = client.GetEvent(body.EventID)
		if err != nil {
			writeError(w, http.StatusBadGateway, "could not find event: "+err.Error())
			return
		}

		tickets, err = client.TicketsForEvent(event.ID)
		if err != nil {
			writeError(w, http.StatusBadGateway, "could not fetch tickets: "+err.Error())
			return
		}
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
			m.id, m.stage, m.group_id, m.bracket, m.round, m.position,
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
			&m.ID, &m.Stage, &m.GroupID, &m.Bracket, &m.Round, &m.Position,
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
	var bracket, round, position *int
	if err := s.db.QueryRowContext(ctx, `
		SELECT player1_id, player2_id, stage, bracket, round, position, status, tournament_id FROM matches WHERE id = ?
	`, id).Scan(&p1ID, &p2ID, &stage, &bracket, &round, &position, &status, &tournamentID); err != nil {
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

		// Knockout matches feed their winner into the next round of their own
		// bracket. Once every bracket's final is played, the tournament is
		// complete.
		if stage == "knockout" && round != nil && position != nil {
			if *round > 1 {
				col := "player1_id"
				if *position%2 == 1 {
					col = "player2_id"
				}
				if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
					UPDATE matches SET %s = ?
					WHERE tournament_id = ? AND stage = 'knockout' AND bracket IS ? AND round = ? AND position = ?
				`, col), body.WinnerID, tournamentID, bracket, *round-1, *position/2); err != nil {
					return err
				}
			} else {
				var unfinishedFinals int
				if err := tx.QueryRowContext(ctx, `
					SELECT COUNT(*) FROM matches
					WHERE tournament_id = ? AND stage = 'knockout' AND round = 1 AND status != 'complete'
				`, tournamentID).Scan(&unfinishedFinals); err != nil {
					return err
				}
				if unfinishedFinals == 0 {
					if _, err := tx.ExecContext(ctx, `UPDATE tournaments SET status = 'complete' WHERE id = ?`, tournamentID); err != nil {
						return err
					}
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
			m.id, m.stage, m.group_id, m.bracket, m.round, m.position,
			m.player1_id, p1.name, m.player2_id, p2.name,
			m.winner_id, m.player1_score, m.player2_score, m.status
		FROM matches m
		LEFT JOIN competitors p1 ON p1.id = m.player1_id
		LEFT JOIN competitors p2 ON p2.id = m.player2_id
		WHERE m.id = ?
	`, id).Scan(
		&m.ID, &m.Stage, &m.GroupID, &m.Bracket, &m.Round, &m.Position,
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

	var tournamentID, status string
	if err := s.db.QueryRowContext(ctx, `
		SELECT id, status FROM tournaments ORDER BY created_at DESC LIMIT 1
	`).Scan(&tournamentID, &status); err != nil {
		writeError(w, http.StatusBadRequest, "no tournament")
		return
	}
	if status != string(TournamentStatusSetup) && status != string(TournamentStatusGroupStage) {
		writeError(w, http.StatusBadRequest, "draw is only available before the knockout stage")
		return
	}

	// Redraw: allowed until the first result is recorded, then the draw is
	// locked in. Wipes the previous groups and their matches.
	if status == string(TournamentStatusGroupStage) {
		var started int
		if err := s.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM matches WHERE tournament_id = ? AND status != 'pending'
		`, tournamentID).Scan(&started); err != nil {
			writeError(w, http.StatusInternalServerError, "query failed")
			return
		}
		if started > 0 {
			writeError(w, http.StatusBadRequest, "cannot redraw after a match has been played")
			return
		}
		// Order matters: competitors reference groups (FK), so unlink them
		// before deleting the groups.
		for _, stmt := range []string{
			`DELETE FROM matches WHERE tournament_id = ?`,
			`UPDATE competitors SET group_id = NULL WHERE tournament_id = ?`,
			`DELETE FROM groups WHERE tournament_id = ?`,
		} {
			if _, err := s.db.ExecContext(ctx, stmt, tournamentID); err != nil {
				writeError(w, http.StatusInternalServerError, "clear previous draw failed")
				return
			}
		}
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

// defaultAdvanceCount is how many competitors advance from each group to the
// knockout when the request doesn't specify a count. With the default two
// brackets, the top half go to the main bracket and the rest to the consolation.
const defaultAdvanceCount = 4

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

// qualifier is a competitor advancing to the knockout, with the group-stage
// record used for seeding and their group for clash avoidance.
type qualifier struct {
	id      string
	groupID string
	points  int
	won     int
}

// seedBand groups 0-based seed indexes into the bands within which players are
// interchangeable when avoiding same-group clashes: {1}, {2}, {3-4}, {5-8}, …
// A seed's band determines the earliest round it can meet a higher band, so
// permuting within a band keeps the score-based seeding structurally intact.
func seedBand(seedIdx int) int {
	band := 0
	for threshold := 1; seedIdx >= threshold; threshold <<= 1 {
		band++
	}
	return band
}

// clashCost scores a bracket layout by how early same-group players meet.
// slots[i] is a qualifier index (or -1 for a bye); consecutive pairs are
// first-round matches, consecutive quads are round-of-4 subtrees, and so on.
// Earlier possible meetings are weighted far more heavily than later ones, so
// minimizing the cost pushes same-group meetings as late as possible.
func clashCost(slots []int, quals []qualifier) int {
	cost := 0
	weight := 1 << 24
	for sub := 2; sub <= len(slots)/2; sub *= 2 {
		for start := 0; start < len(slots); start += sub {
			counts := map[string]int{}
			for i := start; i < start+sub; i++ {
				if slots[i] >= 0 {
					counts[quals[slots[i]].groupID]++
				}
			}
			for _, c := range counts {
				if c > 1 {
					cost += (c - 1) * weight
				}
			}
		}
		weight >>= 6
	}
	return cost
}

// avoidGroupClashes permutes players between slots of the same seed band until
// no such swap delays a same-group meeting further. Bye slots stay fixed so the
// top seeds keep their byes.
func avoidGroupClashes(slots []int, quals []qualifier) {
	for improved := true; improved; {
		improved = false
		best := clashCost(slots, quals)
		for i := 0; i < len(slots); i++ {
			for j := i + 1; j < len(slots); j++ {
				a, b := slots[i], slots[j]
				if a < 0 || b < 0 || seedBand(a) != seedBand(b) {
					continue
				}
				slots[i], slots[j] = b, a
				if c := clashCost(slots, quals); c < best {
					best = c
					improved = true
				} else {
					slots[i], slots[j] = a, b
				}
			}
		}
	}
}

// createKnockoutBracket seeds quals by points (then wins) into a new
// single-elimination bracket, keeping same-group players apart for as long as
// possible, and inserts all its matches tagged with the given bracket number.
func (s *Server) createKnockoutBracket(ctx context.Context, tournamentID string, bracket int, quals []qualifier) error {
	sort.SliceStable(quals, func(i, j int) bool {
		if quals[i].points != quals[j].points {
			return quals[i].points > quals[j].points
		}
		return quals[i].won > quals[j].won
	})

	size := nextPow2(len(quals))
	firstRound := 0
	for (1 << firstRound) < size {
		firstRound++
	}

	// Lay seeds out in standard bracket order, then shuffle within seed bands
	// to push same-group meetings as late as possible. A slot holding -1 is a
	// bye: seed numbers beyond the qualifier count.
	order := seedPositions(size)
	slots := make([]int, size)
	for i, seed := range order {
		if seed <= len(quals) {
			slots[i] = seed - 1
		} else {
			slots[i] = -1
		}
	}
	avoidGroupClashes(slots, quals)

	// Pre-create skeleton matches for every round after the first, so the full
	// bracket tree exists with TBD slots that later winners feed into.
	parentMatch := map[int]map[int]string{}
	for round := firstRound - 1; round >= 1; round-- {
		parentMatch[round] = map[int]string{}
		for pos := 0; pos < (1 << (round - 1)); pos++ {
			mid := newID()
			if _, err := s.db.ExecContext(ctx, `
				INSERT INTO matches (id, tournament_id, stage, bracket, round, position, status)
				VALUES (?, ?, 'knockout', ?, ?, ?, 'pending')
			`, mid, tournamentID, bracket, round, pos); err != nil {
				return err
			}
			parentMatch[round][pos] = mid
		}
	}

	// First round: a pair with a bye sends its present player straight into the
	// next round's slot.
	for j := 0; j < size/2; j++ {
		a, b := slots[2*j], slots[2*j+1]

		if a >= 0 && b >= 0 {
			if _, err := s.db.ExecContext(ctx, `
				INSERT INTO matches (id, tournament_id, stage, bracket, round, position, player1_id, player2_id, status)
				VALUES (?, ?, 'knockout', ?, ?, ?, ?, ?, 'pending')
			`, newID(), tournamentID, bracket, firstRound, j, quals[a].id, quals[b].id); err != nil {
				return err
			}
			continue
		}

		winner := a
		if winner < 0 {
			winner = b
		}
		col := "player1_id"
		if j%2 == 1 {
			col = "player2_id"
		}
		if _, err := s.db.ExecContext(ctx,
			fmt.Sprintf(`UPDATE matches SET %s = ? WHERE id = ?`, col),
			quals[winner].id, parentMatch[firstRound-1][j/2]); err != nil {
			return err
		}
	}

	return nil
}

func (s *Server) handleAdvance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var body struct {
		AdvanceTotal  int  `json:"advance_total"`
		SingleBracket bool `json:"single_bracket"`
	}
	json.NewDecoder(r.Body).Decode(&body)

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

	// advance_total is the total number of players progressing across all
	// groups; each group contributes an equal share of its top finishers.
	perGroup := defaultAdvanceCount
	if body.AdvanceTotal > 0 {
		if body.AdvanceTotal%len(groupIDs) != 0 {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("%d players cannot be split evenly across %d groups", body.AdvanceTotal, len(groupIDs)))
			return
		}
		perGroup = body.AdvanceTotal / len(groupIDs)
	}
	// A second bracket needs at least one place per group feeding it.
	numBrackets := 2
	if body.SingleBracket || perGroup < 2 {
		numBrackets = 1
	}

	// Split each group's qualifiers between brackets: the top half of the
	// advancing places go to the main bracket, the rest to the consolation.
	mainPlaces := perGroup
	if numBrackets == 2 {
		mainPlaces = (perGroup + 1) / 2
	}
	brackets := make([][]qualifier, numBrackets)
	for _, gid := range groupIDs {
		standings, err := s.groupStandings(ctx, gid)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "standings failed")
			return
		}
		for place := 0; place < perGroup && place < len(standings); place++ {
			b := 0
			if place >= mainPlaces {
				b = 1
			}
			brackets[b] = append(brackets[b], qualifier{standings[place].ID, gid, standings[place].Points, standings[place].Won})
		}
	}

	for bi, quals := range brackets {
		if len(quals) < 2 {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("not enough qualifiers to form bracket %d", bi+1))
			return
		}
		if err := s.createKnockoutBracket(ctx, tournamentID, bi+1, quals); err != nil {
			writeError(w, http.StatusInternalServerError, "create bracket failed")
			return
		}
	}

	if _, err := s.db.ExecContext(ctx, `
		UPDATE tournaments SET status = 'knockout' WHERE id = ?
	`, tournamentID); err != nil {
		writeError(w, http.StatusInternalServerError, "update status failed")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetBracket(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			m.id, m.stage, m.group_id, m.bracket, m.round, m.position,
			m.player1_id, p1.name, m.player2_id, p2.name,
			m.winner_id, m.player1_score, m.player2_score, m.status
		FROM matches m
		LEFT JOIN competitors p1 ON p1.id = m.player1_id
		LEFT JOIN competitors p2 ON p2.id = m.player2_id
		WHERE m.stage = 'knockout'
		  AND m.tournament_id = (SELECT id FROM tournaments ORDER BY created_at DESC LIMIT 1)
		ORDER BY m.bracket, m.round DESC, m.position
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
			&m.ID, &m.Stage, &m.GroupID, &m.Bracket, &m.Round, &m.Position,
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
