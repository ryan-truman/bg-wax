package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestKnockoutWalkoverOnRemoval verifies that removing a competitor mid-knockout
// hands their opponent a walkover into the next round, so the bracket can still
// reach — and complete — a final.
func TestKnockoutWalkoverOnRemoval(t *testing.T) {
	s := newTestServer(t)
	importDemo(t, s)
	putSettings(t, s, `{"num_groups":4}`)
	if rec := do(t, s, "POST", "/api/tournament/draw", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("draw: got %d: %s", rec.Code, rec.Body)
	}

	// Finish the group stage, then advance to a single 4-player knockout.
	for _, m := range listMatches(t, s) {
		if m.Status != MatchStatusComplete {
			if code := patchMatch(t, s, m.ID, *m.Player1ID, 1); code != http.StatusOK {
				t.Fatalf("complete group match: got %d", code)
			}
		}
	}
	putSettings(t, s, `{"advance_total":4,"single_bracket":true}`)
	if rec := do(t, s, "POST", "/api/tournament/advance", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("advance: got %d: %s", rec.Code, rec.Body)
	}

	// A 4-player bracket is two semis (round 2) feeding one final (round 1).
	// Remove one semi-finalist and expect their opponent to walk over.
	var semi Match
	for _, m := range listBracket(t, s) {
		if m.Round != nil && *m.Round == 2 && m.Position != nil && *m.Position == 0 {
			semi = m
		}
	}
	if semi.ID == "" || semi.Player1ID == nil || semi.Player2ID == nil {
		t.Fatalf("semi-final not fully populated: %+v", semi)
	}
	removed, survivor := *semi.Player1ID, *semi.Player2ID

	if rec := do(t, s, "DELETE", "/api/competitors/"+removed, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("remove competitor: got %d: %s", rec.Code, rec.Body)
	}

	// The semi is now a completed walkover for the survivor, who has been pushed
	// into the final (semi position 0 feeds the final's player1 slot).
	var final Match
	for _, m := range listBracket(t, s) {
		if m.ID == semi.ID {
			if m.Status != MatchStatusComplete || m.WinnerID == nil || *m.WinnerID != survivor {
				t.Fatalf("walkover not recorded on semi: %+v", m)
			}
		}
		if m.Round != nil && *m.Round == 1 {
			final = m
		}
	}
	if final.Player1ID == nil || *final.Player1ID != survivor {
		t.Fatalf("survivor not advanced into final: %+v", final)
	}

	// Play the other semi and the final; the tournament should complete.
	for _, m := range listBracket(t, s) {
		if m.Round != nil && *m.Round == 2 && m.Status != MatchStatusComplete {
			if code := patchMatch(t, s, m.ID, *m.Player1ID, 1); code != http.StatusOK {
				t.Fatalf("record other semi: got %d", code)
			}
		}
	}
	for _, m := range listBracket(t, s) {
		if m.Round != nil && *m.Round == 1 {
			final = m
		}
	}
	if code := patchMatch(t, s, final.ID, *final.Player1ID, 2); code != http.StatusOK {
		t.Fatalf("record final: got %d", code)
	}
	if got := tournamentStatus(t, s); got != string(TournamentStatusComplete) {
		t.Fatalf("tournament status after final: got %q, want complete", got)
	}
}

func tournamentStatus(t *testing.T, s *Server) string {
	t.Helper()
	rec := do(t, s, "GET", "/api/tournament", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get tournament: got %d", rec.Code)
	}
	var tr Tournament
	if err := json.Unmarshal(rec.Body.Bytes(), &tr); err != nil {
		t.Fatalf("decode tournament: %v", err)
	}
	return string(tr.Status)
}
