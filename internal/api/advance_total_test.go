package api

import (
	"context"
	"fmt"
	"math/bits"
	"net/http"
	"testing"
)

// TestAdvanceTotalBestRunnersUp checks the per-bracket total: a base number
// qualify from every group and the best of the next-place finishers top the
// bracket up to the target. Run at two sizes, because the totals a large field
// needs (32, 64) are as valid as the small ones.
func TestAdvanceTotalBestRunnersUp(t *testing.T) {
	cases := []struct {
		name         string
		eventID      string
		numGroups    int
		advanceTotal int
	}{
		// 40 players in 6 groups (four of 7, two of 6): the top 2 of every group
		// make 12, and the 4 best third-placed finishers fill a round of 16.
		{"typical field, 16-player bracket", "demo-summer-open", 6, 16},
		// 84 players in 13 groups (six of 7, seven of 6): the top two of every
		// group make 26, and the six best third-placed finishers fill a
		// 32-player bracket.
		{"large field, 32-player bracket", "demo-grand-open", 13, 32},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer(t)
			if rec := do(t, s, "POST", "/api/tournament/import", `{"api_key":"demo","event_id":"`+tc.eventID+`"}`); rec.Code != http.StatusOK {
				t.Fatalf("import: got %d: %s", rec.Code, rec.Body)
			}
			putSettings(t, s, fmt.Sprintf(`{"num_groups":%d}`, tc.numGroups))
			if rec := do(t, s, "POST", "/api/tournament/draw", ""); rec.Code != http.StatusNoContent {
				t.Fatalf("draw: got %d: %s", rec.Code, rec.Body)
			}
			// Play every group game (player 1 wins) so standings are decided.
			for _, m := range listMatches(t, s) {
				if code := patchMatch(t, s, m.ID, *m.Player1ID, 2); code != http.StatusOK {
					t.Fatalf("complete group match: got %d", code)
				}
			}

			putSettings(t, s, fmt.Sprintf(`{"advance_total":%d,"single_bracket":true}`, tc.advanceTotal))
			if rec := do(t, s, "POST", "/api/tournament/advance", ""); rec.Code != http.StatusNoContent {
				t.Fatalf("advance: got %d: %s", rec.Code, rec.Body)
			}

			// The qualifiers fill the bracket exactly: total-1 matches in all,
			// total/2 of them in the first round with no empty (bye) slots.
			wantMatches := tc.advanceTotal - 1
			wantFirstRound := tc.advanceTotal / 2
			firstRoundNo := bits.Len(uint(tc.advanceTotal)) - 1

			bracket := listBracket(t, s)
			if len(bracket) != wantMatches {
				t.Fatalf("knockout matches = %d, want %d for %d qualifiers", len(bracket), wantMatches, tc.advanceTotal)
			}
			firstRound := 0
			for _, m := range bracket {
				if m.Round != nil && *m.Round == firstRoundNo {
					firstRound++
					if m.Player1ID == nil || m.Player2ID == nil {
						t.Fatalf("first-round match has an empty slot (unexpected bye): %+v", m)
					}
				}
				if m.Bracket != nil && *m.Bracket != 1 {
					t.Fatalf("expected a single bracket, found match in bracket %d", *m.Bracket)
				}
			}
			if firstRound != wantFirstRound {
				t.Fatalf("first-round matches = %d, want %d (%d qualifiers)", firstRound, wantFirstRound, tc.advanceTotal)
			}
		})
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
