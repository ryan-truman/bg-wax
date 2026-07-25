package api

import (
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
