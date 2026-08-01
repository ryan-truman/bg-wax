package api

import (
	"context"
	"net/http"
	"testing"
)

// TestAdvanceTotalBestRunnersUp checks the per-bracket total: a target of 16
// from 6 groups qualifies the top 2 of every group (12) and tops up with the
// 4 best next-place finishers, landing a clean 16-player round of 16.
func TestAdvanceTotalBestRunnersUp(t *testing.T) {
	s := newTestServer(t)
	if rec := do(t, s, "POST", "/api/tournament/import", `{"api_key":"demo","event_id":"demo-summer-open"}`); rec.Code != http.StatusOK {
		t.Fatalf("import: got %d: %s", rec.Code, rec.Body)
	}
	putSettings(t, s, `{"num_groups":6}`)
	if rec := do(t, s, "POST", "/api/tournament/draw", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("draw: got %d: %s", rec.Code, rec.Body)
	}
	// Play every group game (player 1 wins) so standings are decided.
	for _, m := range listMatches(t, s) {
		if code := patchMatch(t, s, m.ID, *m.Player1ID, 2); code != http.StatusOK {
			t.Fatalf("complete group match: got %d", code)
		}
	}

	putSettings(t, s, `{"advance_total":16,"single_bracket":true}`)
	if rec := do(t, s, "POST", "/api/tournament/advance", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("advance: got %d: %s", rec.Code, rec.Body)
	}

	// 16 qualifiers fill an exact round of 16: 8 first-round matches, every slot
	// filled (no byes), and 8+4+2+1 = 15 knockout matches in a single bracket.
	bracket := listBracket(t, s)
	if len(bracket) != 15 {
		t.Fatalf("knockout matches = %d, want 15 for 16 qualifiers", len(bracket))
	}
	firstRound := 0
	for _, m := range bracket {
		if m.Round != nil && *m.Round == 4 {
			firstRound++
			if m.Player1ID == nil || m.Player2ID == nil {
				t.Fatalf("round-of-16 match has an empty slot (unexpected bye): %+v", m)
			}
		}
		if m.Bracket != nil && *m.Bracket != 1 {
			t.Fatalf("expected a single bracket, found match in bracket %d", *m.Bracket)
		}
	}
	if firstRound != 8 {
		t.Fatalf("first-round matches = %d, want 8 (16 qualifiers)", firstRound)
	}
}

// TestAdvanceFailureLeavesNoPartialBracket: when the top bracket would swallow
// every qualifier and leave the second bracket short, the advance is refused
// before anything is written — no half-seeded knockout, status unchanged.
func TestAdvanceFailureLeavesNoPartialBracket(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()

	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("exec %q: %v", query, err)
		}
	}
	tID := newID()
	exec(`INSERT INTO tournaments (id, name, status) VALUES (?, 'T', 'group_stage')`, tID)

	// Two groups of two with decided matches: four players in total.
	for _, name := range []string{"A", "B"} {
		gid := newID()
		exec(`INSERT INTO groups (id, tournament_id, name) VALUES (?, ?, ?)`, gid, tID, "Group "+name)
		winner, loser := newID(), newID()
		exec(`INSERT INTO competitors (id, tournament_id, name, group_id) VALUES (?, ?, ?, ?)`, winner, tID, name+" Winner", gid)
		exec(`INSERT INTO competitors (id, tournament_id, name, group_id) VALUES (?, ?, ?, ?)`, loser, tID, name+" Runner", gid)
		exec(`INSERT INTO matches (id, tournament_id, stage, group_id, player1_id, player2_id, winner_id, player1_score, player2_score, status)
			VALUES (?, ?, 'group', ?, ?, ?, ?, 2, 0, 'complete')`, newID(), tID, gid, winner, loser, winner)
	}

	// Four per bracket over two brackets: the first takes all four players,
	// leaving nobody for the second.
	putSettings(t, s, `{"advance_total":4,"single_bracket":false}`)
	if rec := do(t, s, "POST", "/api/tournament/advance", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("advance: got %d, want 400 when the second bracket cannot be formed: %s", rec.Code, rec.Body)
	}

	if got := len(listBracket(t, s)); got != 0 {
		t.Fatalf("knockout matches = %d, want 0 — a failed advance must write nothing", got)
	}
	if got := tournamentStatus(t, s); got != "group_stage" {
		t.Fatalf("status = %q, want group_stage after a failed advance", got)
	}
}
