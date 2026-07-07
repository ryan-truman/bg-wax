package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func listMatches(t *testing.T, s *Server) []Match {
	t.Helper()
	rec := do(t, s, "GET", "/api/matches", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list matches: got %d", rec.Code)
	}
	var ms []Match
	if err := json.Unmarshal(rec.Body.Bytes(), &ms); err != nil {
		t.Fatalf("decode matches: %v", err)
	}
	return ms
}

func listBracket(t *testing.T, s *Server) []Match {
	t.Helper()
	rec := do(t, s, "GET", "/api/bracket", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list bracket: got %d", rec.Code)
	}
	var ms []Match
	if err := json.Unmarshal(rec.Body.Bytes(), &ms); err != nil {
		t.Fatalf("decode bracket: %v", err)
	}
	return ms
}

func patchMatch(t *testing.T, s *Server, id, winnerID string, points int) int {
	t.Helper()
	rec := do(t, s, "PATCH", "/api/matches/"+id, fmt.Sprintf(`{"winner_id":%q,"points":%d}`, winnerID, points))
	return rec.Code
}

// TestEditCompletedScores walks the whole lifecycle of result corrections:
// group results are editable during the group stage, locked once the knockout
// starts (they decided the seeding); knockout results are editable until the
// next round has been played, and the final is always editable.
func TestEditCompletedScores(t *testing.T) {
	s := newTestServer(t)
	importDemo(t, s)
	putSettings(t, s, `{"num_groups":4}`)
	if rec := do(t, s, "POST", "/api/tournament/draw", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("draw: got %d: %s", rec.Code, rec.Body)
	}

	// Record a group result, then correct it to the other player.
	groupMatch := listMatches(t, s)[0]
	if code := patchMatch(t, s, groupMatch.ID, *groupMatch.Player1ID, 2); code != http.StatusOK {
		t.Fatalf("record group result: got %d", code)
	}
	if code := patchMatch(t, s, groupMatch.ID, *groupMatch.Player2ID, 3); code != http.StatusOK {
		t.Fatalf("correct group result during group stage: got %d, want 200", code)
	}
	for _, m := range listMatches(t, s) {
		if m.ID == groupMatch.ID {
			if m.WinnerID == nil || *m.WinnerID != *groupMatch.Player2ID || m.Player2Score == nil || *m.Player2Score != 3 {
				t.Fatalf("correction not applied: %+v", m)
			}
		}
	}

	// Finish the group stage and advance to a single 4-player knockout.
	for _, m := range listMatches(t, s) {
		if m.Status != MatchStatusComplete {
			if code := patchMatch(t, s, m.ID, *m.Player1ID, 1); code != http.StatusOK {
				t.Fatalf("complete group match: got %d", code)
			}
		}
	}
	putSettings(t, s, `{"advance_per_group":1,"single_bracket":true}`)
	if rec := do(t, s, "POST", "/api/tournament/advance", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("advance: got %d: %s", rec.Code, rec.Body)
	}

	// Group results are now locked.
	if code := patchMatch(t, s, groupMatch.ID, *groupMatch.Player1ID, 1); code != http.StatusConflict {
		t.Fatalf("edit group result after advance: got %d, want 409", code)
	}

	// Record a semi-final, then correct it while the final is still pending.
	var semi Match
	for _, m := range listBracket(t, s) {
		if m.Round != nil && *m.Round == 2 && m.Position != nil && *m.Position == 0 {
			semi = m
		}
	}
	if semi.ID == "" {
		t.Fatal("semi-final not found in bracket")
	}
	if code := patchMatch(t, s, semi.ID, *semi.Player1ID, 2); code != http.StatusOK {
		t.Fatalf("record semi: got %d", code)
	}
	if code := patchMatch(t, s, semi.ID, *semi.Player2ID, 3); code != http.StatusOK {
		t.Fatalf("correct semi before final is played: got %d, want 200", code)
	}
	// The corrected winner must have replaced the original in the final's slot
	// (semi position 0 feeds the final's player1 slot).
	var final Match
	for _, m := range listBracket(t, s) {
		if m.Round != nil && *m.Round == 1 {
			final = m
		}
	}
	if final.Player1ID == nil || *final.Player1ID != *semi.Player2ID {
		t.Fatalf("final slot not re-fed after correction: %+v", final)
	}

	// Play the other semi and the final; the semi result is now locked, but
	// the final itself can still be corrected.
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
	if code := patchMatch(t, s, semi.ID, *semi.Player1ID, 1); code != http.StatusConflict {
		t.Fatalf("edit semi after final played: got %d, want 409", code)
	}
	if code := patchMatch(t, s, final.ID, *final.Player2ID, 3); code != http.StatusOK {
		t.Fatalf("correct the final: got %d, want 200", code)
	}
}
