package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"backgammon/internal/db"
)

// TestDemoKeyWorkflow drives the Ticket Tailor demo dataset end to end:
// list events with the "demo" API key, then import attendees from one.
// No network involved.
func TestDemoKeyWorkflow(t *testing.T) {
	database, err := db.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	s := NewServer(database, nil)

	req := httptest.NewRequest("POST", "/api/tickettailor/events", strings.NewReader(`{"api_key":"demo"}`))
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("list events: status %d, body %s", w.Code, w.Body)
	}
	var events []TicketTailorEvent
	if err := json.NewDecoder(w.Body).Decode(&events); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 demo events, got %d", len(events))
	}

	req = httptest.NewRequest("POST", "/api/tournament/import", strings.NewReader(`{"api_key":"demo","event_id":"demo-winter-classic"}`))
	w = httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("import: status %d, body %s", w.Code, w.Body)
	}
	var result struct {
		Count      int    `json:"count"`
		Tournament string `json:"tournament"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Count != 16 {
		t.Errorf("expected 16 winter classic attendees, got %d", result.Count)
	}

	// A key other than "demo" with a demo event ID must not hit the branch.
	req = httptest.NewRequest("POST", "/api/tournament/import", strings.NewReader(`{"api_key":"","event_id":"demo-summer-open"}`))
	w = httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400 for missing api_key, got %d", w.Code)
	}
}

// TestRedraw covers the redraw rules: allowed while all matches are pending,
// rejected once a result has been recorded.
func TestRedraw(t *testing.T) {
	database, err := db.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	s := NewServer(database, nil)

	do := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)
		return w
	}

	if w := do("POST", "/api/tournament/import", `{"api_key":"demo","event_id":"demo-winter-classic"}`); w.Code != 200 {
		t.Fatalf("import: status %d, body %s", w.Code, w.Body)
	}
	putSettings(t, s, `{"num_groups":4}`)
	if w := do("POST", "/api/tournament/draw", ""); w.Code != 204 {
		t.Fatalf("initial draw: status %d, body %s", w.Code, w.Body)
	}

	// Switch to automatic sizing and redraw while everything is pending:
	// 16 players → 3 groups (6+5+5), so 15+10+10 = 35 round-robin matches.
	// The old draw's 4 groups of 4 (6 matches each) must be gone.
	putSettings(t, s, `{"num_groups":0}`)
	if w := do("POST", "/api/tournament/draw", ""); w.Code != 204 {
		t.Fatalf("redraw: status %d, body %s", w.Code, w.Body)
	}
	var matches []Match
	w := do("GET", "/api/matches", "")
	if err := json.NewDecoder(w.Body).Decode(&matches); err != nil {
		t.Fatal(err)
	}
	if len(matches) != 35 {
		t.Fatalf("expected 35 matches after redraw, got %d", len(matches))
	}

	// Record one result; redraw must now be rejected.
	m := matches[0]
	if w := do("PATCH", "/api/matches/"+m.ID, `{"winner_id":"`+*m.Player1ID+`","points":2}`); w.Code != 200 {
		t.Fatalf("record result: status %d, body %s", w.Code, w.Body)
	}
	if w := do("POST", "/api/tournament/draw", ""); w.Code != 400 {
		t.Fatalf("expected 400 redrawing after a result, got %d, body %s", w.Code, w.Body)
	}
}
