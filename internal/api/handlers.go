package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	mathrand "math/rand"
	"net/http"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"

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

	// Local edits are authoritative: remember renamed and removed competitors
	// by ticket ID so refreshing the contestant list (e.g. to pick up late
	// ticket sales) can't undo the organiser's changes.
	type localEdit struct {
		name    string
		edited  bool
		removed bool
	}
	edits := map[string]localEdit{}
	editRows, err := s.db.QueryContext(r.Context(), `
		SELECT ticket_tailor_id, name, name_edited, removed
		FROM competitors
		WHERE ticket_tailor_id IS NOT NULL AND (name_edited = 1 OR removed = 1)
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "import failed: "+err.Error())
		return
	}
	for editRows.Next() {
		var tid string
		var e localEdit
		if err := editRows.Scan(&tid, &e.name, &e.edited, &e.removed); err != nil {
			editRows.Close()
			writeError(w, http.StatusInternalServerError, "import failed: "+err.Error())
			return
		}
		edits[tid] = e
	}
	editRows.Close()

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
		// Duplicate ticket names get a numeric suffix ("Alice Mortimer 2") so
		// each ticket is a distinguishable competitor before any renaming.
		displayNames := tickettailor.DisplayNames(tickets)
		for i, t := range tickets {
			name, edited, removed := displayNames[i], false, false
			if e, ok := edits[t.ID]; ok {
				edited, removed = e.edited, e.removed
				if e.edited {
					name = e.name
				}
			}
			if _, err := tx.ExecContext(r.Context(),
				`INSERT INTO competitors (id, tournament_id, name, email, ticket_tailor_id, order_id, name_edited, removed)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				newID(), tournamentID, name, t.Email, t.ID, t.OrderID, edited, removed,
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

// defaultMinGroupGames is the target group-stage games per player when the
// organiser hasn't configured one: groups of five, four games each.
const defaultMinGroupGames = 4

// maxGroups bounds the fixed group count the organiser can set. It is only a
// sanity guard on a stored setting — groups need at least four players, so the
// draw is what rejects a count the entrant list is too small to fill.
const maxGroups = 200

// loadSettings returns the persisted organiser preferences, with any unset or
// out-of-range value falling back to its default.
func (s *Server) loadSettings(ctx context.Context) Settings {
	st := Settings{
		MinGroupGames: defaultMinGroupGames,
		NumGroups:     0, // automatic
		AdvanceTotal:  defaultAdvanceTotal,
		SingleBracket: false,
	}
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return st
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if rows.Scan(&k, &v) != nil {
			continue
		}
		switch k {
		case "min_group_games":
			if n, err := strconv.Atoi(v); err == nil && n >= 3 && n <= 10 {
				st.MinGroupGames = n
			}
		case "num_groups":
			if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= maxGroups {
				st.NumGroups = n
			}
		case "advance_total":
			if n, err := strconv.Atoi(v); err == nil && validAdvanceTotal(n) {
				st.AdvanceTotal = n
			}
		case "single_bracket":
			st.SingleBracket = v == "1"
		}
	}
	return st
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.loadSettings(r.Context()))
}

// handleUpdateSettings merges the request over the current settings, so
// partial updates ({"min_group_games": 5}) leave the other values untouched.
func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st := s.loadSettings(ctx)
	if err := json.NewDecoder(r.Body).Decode(&st); err != nil {
		writeError(w, http.StatusBadRequest, "invalid settings")
		return
	}
	switch {
	case st.MinGroupGames < 3 || st.MinGroupGames > 10:
		writeError(w, http.StatusBadRequest, "min_group_games must be between 3 and 10")
		return
	case st.NumGroups < 0 || st.NumGroups > maxGroups:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("num_groups must be between 1 and %d, or 0 for automatic", maxGroups))
		return
	case !validAdvanceTotal(st.AdvanceTotal):
		writeError(w, http.StatusBadRequest, fmt.Sprintf("advance_total must be one of %s", advanceTotalsLabel()))
		return
	}

	singleBracket := "0"
	if st.SingleBracket {
		singleBracket = "1"
	}
	if err := s.withTx(ctx, func(tx *sql.Tx) error {
		for k, v := range map[string]string{
			"min_group_games": strconv.Itoa(st.MinGroupGames),
			"num_groups":      strconv.Itoa(st.NumGroups),
			"advance_total":   strconv.Itoa(st.AdvanceTotal),
			"single_bracket":  singleBracket,
		} {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO settings (key, value) VALUES (?, ?)
				ON CONFLICT(key) DO UPDATE SET value = excluded.value
			`, k, v); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "save failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleClearTournament resets the tournament to its freshly-imported state:
// the draw, all results and the knockout are wiped, but the tournament and its
// competitors (including local renames and removals) are kept, so the draw can
// be run again without re-importing.
func (s *Server) handleClearTournament(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.withTx(ctx, func(tx *sql.Tx) error {
		for _, stmt := range []string{
			`DELETE FROM matches`,
			`UPDATE competitors SET group_id = NULL, seed = NULL`,
			`DELETE FROM groups`,
			`UPDATE tournaments SET status = 'setup'`,
		} {
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "reset failed: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAddCompetitor(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not yet implemented")
}

func (s *Server) handleDeleteCompetitor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")

	var tournamentID string
	if err := s.db.QueryRowContext(ctx, `SELECT tournament_id FROM competitors WHERE id = ?`, id).Scan(&tournamentID); err != nil {
		writeError(w, http.StatusNotFound, "competitor not found")
		return
	}

	if err := s.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE competitors SET removed = 1 WHERE id = ?`, id); err != nil {
			return err
		}
		// If they were mid-knockout, their opponent walks over so the bracket
		// can still reach a final.
		return resolveKnockoutWalkovers(ctx, tx, tournamentID)
	}); err != nil {
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

// handleRenameCompetitor sets a competitor's display name, e.g. when their
// ticket was bought under someone else's name. The name_edited flag marks the
// local name as authoritative so a Ticket Tailor re-import won't overwrite it.
func (s *Server) handleRenameCompetitor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")

	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	res, err := s.db.ExecContext(ctx, `
		UPDATE competitors SET name = ?, name_edited = 1 WHERE id = ?
	`, name, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "rename failed")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "competitor not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleMoveCompetitor moves a competitor into another group after the draw —
// a late arrival, a no-show leaving a group short, a hand-rebalanced group.
//
// A group is a round robin, so fixtures follow the player: their old group's
// matches are deleted and a pending match against each new group-mate replaces
// them. Any result they had already recorded is deleted too — those were games
// in a group they have left, and keeping them would credit old opponents for a
// game that no longer exists. Callers confirm first when the player has played
// (their standings row carries the count).
//
// Order-mate separation is not re-imposed: moving one named player by hand is a
// deliberate choice, and refusing it would leave the organiser stuck.
func (s *Server) handleMoveCompetitor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")

	var body struct {
		GroupID string `json:"group_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.GroupID == "" {
		writeError(w, http.StatusBadRequest, "group_id is required")
		return
	}

	var tournamentID, status string
	if err := s.db.QueryRowContext(ctx, `
		SELECT id, status FROM tournaments ORDER BY created_at DESC LIMIT 1
	`).Scan(&tournamentID, &status); err != nil {
		writeError(w, http.StatusBadRequest, "no tournament")
		return
	}
	if status != string(TournamentStatusGroupStage) {
		writeError(w, http.StatusBadRequest, "players can only be moved between groups during the group stage")
		return
	}

	var currentGroup *string
	var removed bool
	if err := s.db.QueryRowContext(ctx, `
		SELECT group_id, removed FROM competitors WHERE id = ? AND tournament_id = ?
	`, id, tournamentID).Scan(&currentGroup, &removed); err != nil {
		writeError(w, http.StatusNotFound, "competitor not found")
		return
	}
	if removed {
		writeError(w, http.StatusBadRequest, "this player has been removed from the tournament — restore them first")
		return
	}
	if currentGroup != nil && *currentGroup == body.GroupID {
		writeError(w, http.StatusBadRequest, "player is already in that group")
		return
	}

	var groupExists int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM groups WHERE id = ? AND tournament_id = ?
	`, body.GroupID, tournamentID).Scan(&groupExists); err != nil || groupExists == 0 {
		writeError(w, http.StatusNotFound, "group not found")
		return
	}

	if err := s.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM matches
			WHERE tournament_id = ? AND stage = 'group' AND (player1_id = ? OR player2_id = ?)
		`, tournamentID, id, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE competitors SET group_id = ? WHERE id = ?
		`, body.GroupID, id); err != nil {
			return err
		}

		// Collect the new group-mates before inserting: the rows must be closed
		// before the transaction's connection is used for the writes.
		rows, err := tx.QueryContext(ctx, `
			SELECT id FROM competitors WHERE group_id = ? AND removed = 0 AND id != ?
		`, body.GroupID, id)
		if err != nil {
			return err
		}
		var opponents []string
		for rows.Next() {
			var oID string
			if err := rows.Scan(&oID); err != nil {
				rows.Close()
				return err
			}
			opponents = append(opponents, oID)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		for _, oID := range opponents {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO matches (id, tournament_id, stage, group_id, player1_id, player2_id, status)
				VALUES (?, ?, 'group', ?, ?, ?, 'pending')
			`, newID(), tournamentID, body.GroupID, id, oID); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "move failed: "+err.Error())
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

// rankAbove reports whether a should be ranked above b — the one place the
// ranking order is defined, used for standings, qualifying and seeding alike:
//
//  1. most points;
//  2. head-to-head, if one of them beat the other;
//  3. most wins.
//
// Players it separates on none of the three are genuinely level, and where that
// decides who progresses the organiser is asked to settle it (see cutForced).
func rankAbove(a, b CompetitorStanding, beat map[string]map[string]bool) bool {
	if a.Points != b.Points {
		return a.Points > b.Points
	}
	if beat[a.ID][b.ID] {
		return true
	}
	if beat[b.ID][a.ID] {
		return false
	}
	return a.Won > b.Won
}

// cutForced reports whether the ranking rules alone decide a tied block's split
// at index cut: every player above the line must rank strictly above every
// player below it. Ordering head-to-head above wins means the rules can
// contradict themselves (A beat B, B beat C, C beat A, all on equal wins), so a
// pair only counts as decided when the comparison holds one way and not the
// other. Checking across the line rather than within it keeps the question to
// the one that matters — who progresses — so players left unordered on the same
// side of the cut are no reason to trouble the organiser.
func cutForced(block []CompetitorStanding, cut int, beat map[string]map[string]bool) bool {
	for _, a := range block[:cut] {
		for _, b := range block[cut:] {
			if !rankAbove(a, b, beat) || rankAbove(b, a, beat) {
				return false
			}
		}
	}
	return true
}

// groupStandings tallies completed matches for a group and returns its
// competitors ranked best-first by rankAbove. Removed competitors are excluded.
func (s *Server) groupStandings(ctx context.Context, groupID string) ([]CompetitorStanding, error) {
	cRows, err := s.db.QueryContext(ctx, `
		SELECT id, name FROM competitors WHERE group_id = ? AND removed = 0 ORDER BY name
	`, groupID)
	if err != nil {
		return nil, err
	}
	standings := map[string]*CompetitorStanding{}
	beat := map[string]map[string]bool{} // beat[a][b] == a defeated b, the tie-breaker
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
		if winnerID != nil {
			loser := p1ID
			if *winnerID == p1ID {
				loser = p2ID
			}
			if beat[*winnerID] == nil {
				beat[*winnerID] = map[string]bool{}
			}
			beat[*winnerID][loser] = true
		}
	}
	mRows.Close()

	result := make([]CompetitorStanding, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		result = append(result, *standings[id])
	}
	sort.SliceStable(result, func(i, j int) bool { return rankAbove(result[i], result[j], beat) })
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
		SELECT id, name FROM groups WHERE tournament_id = ? ORDER BY LENGTH(name), name
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
			m.id, m.stage, m.group_id, g.name, m.bracket, m.round, m.position,
			m.player1_id, p1.name, m.player2_id, p2.name,
			m.winner_id, m.player1_score, m.player2_score, m.status
		FROM matches m
		LEFT JOIN competitors p1 ON p1.id = m.player1_id
		LEFT JOIN competitors p2 ON p2.id = m.player2_id
		LEFT JOIN groups g ON g.id = m.group_id
		WHERE m.tournament_id = (SELECT id FROM tournaments ORDER BY created_at DESC LIMIT 1)
		  AND m.player1_id IS NOT NULL AND m.player2_id IS NOT NULL
		  AND p1.removed = 0 AND p2.removed = 0
		ORDER BY m.stage, LENGTH(g.name), g.name, m.round, m.position
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
			&m.ID, &m.Stage, &m.GroupID, &m.GroupName, &m.Bracket, &m.Round, &m.Position,
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

// resolveKnockoutWalkovers advances any player whose knockout opponent has been
// removed. The removed competitor counts as absent at whatever slot they hold:
// the opponent wins by walkover (no score) and moves into the next round,
// cascading so the bracket can still reach a final. Matches whose opponent is
// still TBD are left alone — they resolve once the feeder fills the slot and
// this runs again.
func resolveKnockoutWalkovers(ctx context.Context, tx *sql.Tx, tournamentID string) error {
	for {
		var id string
		var bracket *int
		var round, position int
		var winnerID string
		err := tx.QueryRowContext(ctx, `
			SELECT m.id, m.bracket, m.round, m.position,
			       CASE WHEN c1.removed = 1 THEN m.player2_id ELSE m.player1_id END
			FROM matches m
			JOIN competitors c1 ON c1.id = m.player1_id
			JOIN competitors c2 ON c2.id = m.player2_id
			WHERE m.tournament_id = ? AND m.stage = 'knockout' AND m.status != 'complete'
			  AND c1.removed <> c2.removed
			LIMIT 1
		`, tournamentID).Scan(&id, &bracket, &round, &position, &winnerID)
		if err == sql.ErrNoRows {
			break
		}
		if err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE matches SET winner_id = ?, status = 'complete' WHERE id = ?
		`, winnerID, id); err != nil {
			return err
		}
		if round > 1 {
			col := "player1_id"
			if position%2 == 1 {
				col = "player2_id"
			}
			if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
				UPDATE matches SET %s = ?
				WHERE tournament_id = ? AND stage = 'knockout' AND bracket IS ? AND round = ? AND position = ?
			`, col), winnerID, tournamentID, bracket, round-1, position/2); err != nil {
				return err
			}
		}
	}

	// If the walkovers decided every final, the tournament is over.
	var unfinishedFinals int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM matches
		WHERE tournament_id = ? AND stage = 'knockout' AND round = 1 AND status != 'complete'
	`, tournamentID).Scan(&unfinishedFinals); err != nil {
		return err
	}
	if unfinishedFinals == 0 {
		if _, err := tx.ExecContext(ctx, `
			UPDATE tournaments SET status = 'complete' WHERE id = ? AND status = 'knockout'
		`, tournamentID); err != nil {
			return err
		}
	}
	return nil
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
	// Completed matches can be corrected, but only while the correction cannot
	// corrupt anything downstream:
	//   - group results feed the knockout seeding, so they lock once the
	//     tournament has advanced past the group stage;
	//   - a knockout result locks once the next round's match has been played,
	//     because its winner has already competed there.
	if status == "complete" {
		if stage == "group" {
			var tStatus string
			if err := s.db.QueryRowContext(ctx, `
				SELECT status FROM tournaments WHERE id = ?
			`, tournamentID).Scan(&tStatus); err != nil {
				writeError(w, http.StatusInternalServerError, "query failed")
				return
			}
			if tStatus != string(TournamentStatusGroupStage) {
				writeError(w, http.StatusConflict, "group results are locked once the knockout stage has started")
				return
			}
		} else if round != nil && position != nil && *round > 1 {
			var parentStatus string
			err := s.db.QueryRowContext(ctx, `
				SELECT status FROM matches
				WHERE tournament_id = ? AND stage = 'knockout' AND bracket IS ? AND round = ? AND position = ?
			`, tournamentID, bracket, *round-1, *position/2).Scan(&parentStatus)
			if err == nil && parentStatus == "complete" {
				writeError(w, http.StatusConflict, "this result is locked: the next round has already been played")
				return
			}
		}
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
			// The winner may have just landed opposite a removed player; let any
			// resulting walkover cascade forward.
			return resolveKnockoutWalkovers(ctx, tx, tournamentID)
		}
		return nil
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}

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

// groupName names the i-th group (0-based) "Group A" … "Group Z", then
// "Group AA", "Group AB", … so a draw of any size can name its groups. Listings
// order by name length before name, so "Group Z" still precedes "Group AA".
func groupName(i int) string {
	letters := ""
	for i >= 0 {
		letters = string(rune('A'+i%26)) + letters
		i = i/26 - 1
	}
	return "Group " + letters
}

// handleDraw runs the group draw. It takes no request parameters: sizing comes
// entirely from the persisted settings — a fixed count if the organiser set one,
// otherwise automatic sizing from the games-per-player target (groups of
// target+1, any remainder spilling into bigger groups, since more games beats
// fewer).
func (s *Server) handleDraw(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

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
		SELECT id, order_id FROM competitors WHERE tournament_id = ? AND removed = 0
	`, tournamentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	// Cluster competitors by purchase order: tickets bought together (friends,
	// couples) must not land in the same group. Competitors without an order
	// stand alone.
	clusters := map[string][]string{}
	var clusterKeys []string
	total := 0
	for rows.Next() {
		var id string
		var orderID *string
		if err := rows.Scan(&id, &orderID); err != nil {
			rows.Close()
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		key := "solo:" + id
		if orderID != nil && *orderID != "" {
			key = "order:" + *orderID
		}
		if _, ok := clusters[key]; !ok {
			clusterKeys = append(clusterKeys, key)
		}
		clusters[key] = append(clusters[key], id)
		total++
	}
	rows.Close()

	if total < 4 {
		writeError(w, http.StatusBadRequest, "at least 4 competitors are required for a draw")
		return
	}
	maxCluster := 0
	for _, key := range clusterKeys {
		if n := len(clusters[key]); n > maxCluster {
			maxCluster = n
		}
	}

	st := s.loadSettings(ctx)
	numGroups := st.NumGroups
	if numGroups == 0 {
		// Automatic sizing from the games-per-player target. A fixed count
		// gets no adjustment — the checks below reject it if it can't work.
		numGroups = total / (st.MinGroupGames + 1)
		if numGroups < 1 {
			numGroups = 1
		}
		// A multi-ticket order needs at least as many groups as it has tickets
		// to be kept apart, which is worth extra groups — but never groups
		// below the four-player minimum. A bigger order is a block booking (a
		// company, a venue buying for invited players) that no group count can
		// separate, so the games-per-player target keeps the sizing instead.
		if maxCluster > numGroups && maxCluster <= total/4 {
			numGroups = maxCluster
		}
	}

	// Groups of four are the hard minimum (three games each).
	if total/numGroups < 4 {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("groups must have at least 4 players — %d competitors can make at most %d groups", total, total/4))
		return
	}

	// Shuffle the cluster order and each cluster's members, then flatten with
	// order-mates consecutive and deal round-robin across the groups. Consecutive
	// positions land in distinct groups, so an order spreads as thinly as the
	// group count allows — never sharing a group while it has no more tickets
	// than there are groups, evenly split beyond that. The deal keeps group sizes
	// within one of each other, any remainder landing in the earlier groups.
	mathrand.Shuffle(len(clusterKeys), func(i, j int) {
		clusterKeys[i], clusterKeys[j] = clusterKeys[j], clusterKeys[i]
	})
	ordered := make([]string, 0, total)
	for _, key := range clusterKeys {
		members := clusters[key]
		mathrand.Shuffle(len(members), func(i, j int) {
			members[i], members[j] = members[j], members[i]
		})
		ordered = append(ordered, members...)
	}
	groupMembers := make([][]string, numGroups)
	for i, id := range ordered {
		groupMembers[i%numGroups] = append(groupMembers[i%numGroups], id)
	}

	if err := s.withTx(ctx, func(tx *sql.Tx) error {
		for i := 0; i < numGroups; i++ {
			groupID := newID()
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO groups (id, tournament_id, name) VALUES (?, ?, ?)
			`, groupID, tournamentID, groupName(i)); err != nil {
				return err
			}

			members := groupMembers[i]

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

// advanceTotals are the qualifier counts a knockout bracket can be built from.
// Only powers of two, so a bracket fills exactly and no one gets a bye. 8 and 16
// are the usual choices; 2 and 4 cover small tournaments, 32 and 64 large ones.
// A total the entrant list cannot fill takes everyone available, leaving byes.
var advanceTotals = []int{2, 4, 8, 16, 32, 64}

const defaultAdvanceTotal = 16

func validAdvanceTotal(n int) bool {
	return slices.Contains(advanceTotals, n)
}

// advanceTotalsLabel lists the valid totals for an error message, e.g. "2, 4, 8, 16".
func advanceTotalsLabel() string {
	parts := make([]string, len(advanceTotals))
	for i, n := range advanceTotals {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ", ")
}

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
// first-round matches, consecutive quads round-of-4 subtrees, and so on. Earlier
// meetings weigh far more, so minimizing the cost pushes them as late as possible.
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

// createKnockoutBracket seeds quals by the usual ranking order (see rankAbove),
// falling back to any order the organiser chose to settle a tie, into a new
// single-elimination bracket. Same-group players are kept apart for as long as
// possible, and every match is tagged with the given bracket number.
func createKnockoutBracket(ctx context.Context, tx *sql.Tx, tournamentID string, bracket int, quals []qualifier, beat, manual map[string]map[string]bool) error {
	sort.SliceStable(quals, func(i, j int) bool {
		a, b := quals[i], quals[j]
		as := CompetitorStanding{ID: a.id, Won: a.won, Points: a.points}
		bs := CompetitorStanding{ID: b.id, Won: b.won, Points: b.points}
		if rankAbove(as, bs, beat) {
			return true
		}
		if rankAbove(bs, as, beat) {
			return false
		}
		return manual[a.id][b.id]
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
			if _, err := tx.ExecContext(ctx, `
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
			if _, err := tx.ExecContext(ctx, `
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
		if _, err := tx.ExecContext(ctx,
			fmt.Sprintf(`UPDATE matches SET %s = ? WHERE id = ?`, col),
			quals[winner].id, parentMatch[firstRound-1][j/2]); err != nil {
			return err
		}
	}

	return nil
}

// headToHead returns beat[a][b] == true when competitor a defeated competitor b
// in a completed group-stage match. Used as the final ranking tie-breaker when
// two players are level on wins and points.
func (s *Server) headToHead(ctx context.Context, tournamentID string) (map[string]map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT winner_id, player1_id, player2_id FROM matches
		WHERE tournament_id = ? AND stage = 'group' AND status = 'complete' AND winner_id IS NOT NULL
	`, tournamentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	beat := map[string]map[string]bool{}
	for rows.Next() {
		var winner, p1, p2 string
		if err := rows.Scan(&winner, &p1, &p2); err != nil {
			return nil, err
		}
		loser := p1
		if winner == p1 {
			loser = p2
		}
		if beat[winner] == nil {
			beat[winner] = map[string]bool{}
		}
		beat[winner][loser] = true
	}
	return beat, rows.Err()
}

// tierPicker selects each knockout bracket's qualifiers from the group
// standings and reports the ties it cannot settle on its own.
//
// A base number qualify from every group (target ÷ groups), then the best of the
// next-place finishers fill the remainder (the "best runners-up" rule). `taken`
// records how many places each group has already given to earlier brackets, so
// calling pick again peels off the next tier down.
//
// At each cut — the boundary between players taken and those left behind —
// players level on points that head-to-head cannot separate would be split
// arbitrarily. Those cuts go into `ties` for the organiser to settle; orderings
// they have already given arrive in `resolved`, keyed by tie ID.
type tierPicker struct {
	standings  map[string][]CompetitorStanding
	groupNames map[string]string
	groupIDs   []string
	beat       map[string]map[string]bool
	resolved   map[string][]string
	taken      map[string]int
	ties       []TieBreak
	// manual records the precedence the organiser chose, in the same shape as
	// beat, so seeding keeps their order rather than re-sorting it away.
	manual map[string]map[string]bool
	// numBrackets is needed to tell a tie whether the players who miss out drop
	// into a lower bracket or out of the tournament altogether.
	numBrackets int
}

func newTierPicker(standings map[string][]CompetitorStanding, groupNames map[string]string, groupIDs []string, beat map[string]map[string]bool, resolved map[string][]string, numBrackets int) *tierPicker {
	return &tierPicker{
		standings:   standings,
		groupNames:  groupNames,
		groupIDs:    groupIDs,
		beat:        beat,
		resolved:    resolved,
		taken:       map[string]int{},
		manual:      map[string]map[string]bool{},
		numBrackets: numBrackets,
	}
}

// below is the bracket that players missing this cut fall to, or 0 when this is
// the lowest bracket and missing out means not progressing at all.
func (p *tierPicker) below(bracket int) int {
	if bracket < p.numBrackets {
		return bracket + 1
	}
	return 0
}

func (p *tierPicker) pick(target, bracket int) []qualifier {
	base := target / len(p.groupIDs)
	rem := target % len(p.groupIDs)

	var picks []qualifier
	take := func(gid string) {
		s := p.standings[gid][p.taken[gid]]
		picks = append(picks, qualifier{s.ID, gid, s.Points, s.Won})
		p.taken[gid]++
	}

	// The guaranteed slice from every group. Whoever just misses the cut becomes
	// their group's candidate for the runners-up pool, so when there are leftover
	// places to fill they are still in the running for this bracket.
	for _, gid := range p.groupIDs {
		start := p.taken[gid]
		p.checkGroupCut(gid, start, start+base, bracket, rem > 0)
		for i := 0; i < base && p.taken[gid] < len(p.standings[gid]); i++ {
			take(gid)
		}
	}

	// The best of each group's next-place finishers fill the leftover slots.
	if rem > 0 {
		pool := make([]string, 0, len(p.groupIDs))
		for _, gid := range p.groupIDs {
			if p.taken[gid] < len(p.standings[gid]) {
				pool = append(pool, gid)
			}
		}
		sort.SliceStable(pool, func(i, j int) bool {
			return rankAbove(p.next(pool[i]), p.next(pool[j]), p.beat)
		})
		p.checkPoolCut(pool, rem, bracket)
		for i := 0; i < rem && i < len(pool); i++ {
			take(pool[i])
		}
	}
	return picks
}

// next is the group's best finisher not yet taken — its candidate for the
// runners-up pool.
func (p *tierPicker) next(gid string) CompetitorStanding {
	return p.standings[gid][p.taken[gid]]
}

// checkGroupCut examines the boundary at index cut in one group's standings:
// players before it take a place in this bracket, players after it do not. If
// the players either side are level on points and head-to-head cannot separate
// them, the cut is arbitrary — apply the organiser's ordering if they have
// given one, otherwise record the tie for them to settle.
func (p *tierPicker) checkGroupCut(gid string, start, cut, bracket int, hasPool bool) {
	list := p.standings[gid]
	if cut <= start || cut >= len(list) || list[cut-1].Points != list[cut].Points {
		return
	}
	pts := list[cut].Points
	lo, hi := cut-1, cut+1
	for lo > start && list[lo-1].Points == pts {
		lo--
	}
	for hi < len(list) && list[hi].Points == pts {
		hi++
	}

	block := list[lo:hi]
	if cutForced(block, cut-lo, p.beat) {
		return
	}

	id := fmt.Sprintf("group:%s:%d", gid, lo)
	if p.applyOrder(block, p.resolved[id]) {
		return
	}
	tie := TieBreak{
		ID:          id,
		Scope:       "group",
		GroupName:   p.groupNames[gid],
		Bracket:     bracket,
		Place:       lo + 1,
		Slots:       cut - lo,
		Points:      pts,
		DropsToPool: hasPool,
		Competitors: p.candidates(block, gid),
	}
	if !hasPool {
		tie.DropsToBracket = p.below(bracket)
	}
	p.ties = append(p.ties, tie)
}

// checkPoolCut examines the cut in the runners-up pool, where the leftover
// places go to the best of the groups' next-place finishers. Those candidates
// each come from a different group, so they have never played each other and
// being level on points always needs the organiser to decide.
func (p *tierPicker) checkPoolCut(pool []string, rem, bracket int) {
	if rem >= len(pool) || p.next(pool[rem-1]).Points != p.next(pool[rem]).Points {
		return
	}
	pts := p.next(pool[rem]).Points
	lo, hi := rem-1, rem+1
	for lo > 0 && p.next(pool[lo-1]).Points == pts {
		lo--
	}
	for hi < len(pool) && p.next(pool[hi]).Points == pts {
		hi++
	}

	block := pool[lo:hi]
	standings := make([]CompetitorStanding, len(block))
	for i, gid := range block {
		standings[i] = p.next(gid)
	}
	// These candidates come from different groups so have never met, but wins
	// can still separate them.
	if cutForced(standings, rem-lo, p.beat) {
		return
	}

	id := fmt.Sprintf("pool:%d", bracket)
	if order, ok := p.resolved[id]; ok && p.applyOrder(standings, order) {
		// Re-order the pool itself to match the standings we just sorted.
		byID := make(map[string]string, len(block))
		for _, gid := range block {
			byID[p.next(gid).ID] = gid
		}
		for i, st := range standings {
			block[i] = byID[st.ID]
		}
		return
	}

	candidates := make([]TieCandidate, len(block))
	for i, gid := range block {
		candidates[i] = p.candidate(p.next(gid), gid)
	}
	p.ties = append(p.ties, TieBreak{
		ID:             id,
		Scope:          "pool",
		Bracket:        bracket,
		Place:          lo + 1,
		Slots:          rem - lo,
		Points:         pts,
		DropsToBracket: p.below(bracket),
		Competitors:    candidates,
	})
}

// applyOrder re-orders a tied block to the sequence the organiser chose and
// records that precedence for seeding. It reports false — leaving the block
// untouched — unless the given order is exactly the players in the block, so a
// stale answer from an earlier round of questions is ignored rather than
// half-applied.
func (p *tierPicker) applyOrder(block []CompetitorStanding, order []string) bool {
	if len(order) != len(block) {
		return false
	}
	rank := make(map[string]int, len(order))
	for i, id := range order {
		rank[id] = i
	}
	for _, st := range block {
		if _, ok := rank[st.ID]; !ok {
			return false
		}
	}
	sort.SliceStable(block, func(i, j int) bool { return rank[block[i].ID] < rank[block[j].ID] })
	for i, a := range order {
		for _, b := range order[i+1:] {
			if p.manual[a] == nil {
				p.manual[a] = map[string]bool{}
			}
			p.manual[a][b] = true
		}
	}
	return true
}

func (p *tierPicker) candidate(st CompetitorStanding, gid string) TieCandidate {
	return TieCandidate{
		ID:        st.ID,
		Name:      st.Name,
		GroupName: p.groupNames[gid],
		Played:    st.Played,
		Won:       st.Won,
		Points:    st.Points,
	}
}

func (p *tierPicker) candidates(block []CompetitorStanding, gid string) []TieCandidate {
	out := make([]TieCandidate, len(block))
	for i, st := range block {
		out[i] = p.candidate(st, gid)
	}
	return out
}

// handleAdvance closes the group stage and seeds the knockout. How many advance
// and whether to split into two brackets come from the persisted settings; the
// only accepted body is `tie_breaks`, mapping a tie ID to the finishing order
// the organiser chose.
//
// When a tie decides who progresses and the rules cannot settle it, no bracket
// is created: the handler replies 409 with the ties to put to the organiser,
// and the request is repeated once they have answered.
func (s *Server) handleAdvance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var body struct {
		TieBreaks map[string][]string `json:"tie_breaks"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var tournamentID string
	if err := s.db.QueryRowContext(ctx, `
		SELECT id FROM tournaments WHERE status = 'group_stage' ORDER BY created_at DESC LIMIT 1
	`).Scan(&tournamentID); err != nil {
		writeError(w, http.StatusBadRequest, "no tournament in group stage")
		return
	}

	groupRows, err := s.db.QueryContext(ctx, `
		SELECT id, name FROM groups WHERE tournament_id = ? ORDER BY LENGTH(name), name
	`, tournamentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	var groupIDs []string
	groupNames := map[string]string{}
	for groupRows.Next() {
		var gid, gname string
		if err := groupRows.Scan(&gid, &gname); err != nil {
			groupRows.Close()
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		groupIDs = append(groupIDs, gid)
		groupNames[gid] = gname
	}
	groupRows.Close()

	if len(groupIDs) == 0 {
		writeError(w, http.StatusBadRequest, "no groups to advance from")
		return
	}

	st := s.loadSettings(ctx)
	numBrackets := 2
	if st.SingleBracket {
		numBrackets = 1
	}

	// Load each group's standings once, then peel off one bracket's worth of
	// qualifiers at a time: the main bracket takes the top tier, the consolation
	// the next.
	standings := map[string][]CompetitorStanding{}
	for _, gid := range groupIDs {
		list, err := s.groupStandings(ctx, gid)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "standings failed")
			return
		}
		standings[gid] = list
	}
	beat, err := s.headToHead(ctx, tournamentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "head-to-head failed")
		return
	}

	picker := newTierPicker(standings, groupNames, groupIDs, beat, body.TieBreaks, numBrackets)
	brackets := make([][]qualifier, numBrackets)
	for bi := range brackets {
		brackets[bi] = picker.pick(st.AdvanceTotal, bi+1)
	}

	// Nothing has been written yet, so an unsettled tie can simply stop here.
	// Answering one tie can shift the cut and expose another, so the organiser
	// may be asked again on the repeat request.
	if len(picker.ties) > 0 {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "a tie decides who progresses and needs to be settled",
			"ties":  picker.ties,
		})
		return
	}

	for bi, quals := range brackets {
		if len(quals) < 2 {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("not enough qualifiers to form bracket %d", bi+1))
			return
		}
	}

	// All brackets and the status flip land together, so a failure part-way
	// can't leave a half-seeded knockout on a tournament still in group stage.
	if err := s.withTx(ctx, func(tx *sql.Tx) error {
		for bi, quals := range brackets {
			if err := createKnockoutBracket(ctx, tx, tournamentID, bi+1, quals, beat, picker.manual); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, `
			UPDATE tournaments SET status = 'knockout' WHERE id = ?
		`, tournamentID)
		return err
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "create bracket failed")
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
