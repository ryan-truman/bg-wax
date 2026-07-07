package api

import (
	"net/http"
	"strings"
	"testing"
)

// TestResetKeepsCompetitors: resetting the tournament wipes the draw, the
// results and the knockout, but keeps the tournament and its competitors so
// the draw can be run again immediately — no re-import needed.
func TestResetKeepsCompetitors(t *testing.T) {
	s := newTestServer(t)
	importDemo(t, s)
	before := listCompetitors(t, s)

	if rec := do(t, s, "POST", "/api/tournament/draw", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("draw: got %d: %s", rec.Code, rec.Body)
	}
	m := listMatches(t, s)[0]
	if code := patchMatch(t, s, m.ID, *m.Player1ID, 2); code != http.StatusOK {
		t.Fatalf("record result: got %d", code)
	}

	if rec := do(t, s, "POST", "/api/tournament/clear", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("reset: got %d: %s", rec.Code, rec.Body)
	}

	rec := do(t, s, "GET", "/api/tournament", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("tournament gone after reset: got %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"setup"`) {
		t.Fatalf("tournament not back in setup after reset: %s", body)
	}
	if got := len(listCompetitors(t, s)); got != len(before) {
		t.Fatalf("competitors after reset = %d, want %d", got, len(before))
	}
	if got := len(listMatches(t, s)); got != 0 {
		t.Fatalf("matches after reset = %d, want 0", got)
	}
	if got := len(listGroups(t, s)); got != 0 {
		t.Fatalf("groups after reset = %d, want 0", got)
	}

	// The draw must run again without a re-import.
	if rec := do(t, s, "POST", "/api/tournament/draw", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("draw after reset: got %d: %s", rec.Code, rec.Body)
	}
}
