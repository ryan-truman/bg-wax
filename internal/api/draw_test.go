package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func listGroups(t *testing.T, s *Server) []Group {
	t.Helper()
	rec := do(t, s, "GET", "/api/groups", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list groups: got %d", rec.Code)
	}
	var gs []Group
	if err := json.Unmarshal(rec.Body.Bytes(), &gs); err != nil {
		t.Fatalf("decode groups: %v", err)
	}
	return gs
}

// TestDrawKeepsOrderMatesApart verifies that tickets purchased in one order
// never share a group. The demo Winter Classic has two multi-ticket orders:
// Alice Mortimer × 3 (all under the buyer's name) and Ben Okoro + Carla Reyes.
// The draw is random, so redraw repeatedly — the guarantee must hold every
// time, not just on a lucky shuffle.
func TestDrawKeepsOrderMatesApart(t *testing.T) {
	s := newTestServer(t)
	importDemo(t, s)
	putSettings(t, s, `{"num_groups":4}`)

	for i := 0; i < 10; i++ {
		if rec := do(t, s, "POST", "/api/tournament/draw", ""); rec.Code != http.StatusNoContent {
			t.Fatalf("draw %d: got %d: %s", i, rec.Code, rec.Body)
		}

		// Import numbers duplicate names, so Alice's tickets appear as
		// "Alice Mortimer", "Alice Mortimer 2" and "Alice Mortimer 3".
		alices := 0
		for _, g := range listGroups(t, s) {
			names := map[string]int{}
			groupAlices := 0
			for _, c := range g.Competitors {
				names[c.Name]++
				if strings.HasPrefix(c.Name, "Alice Mortimer") {
					groupAlices++
				}
			}
			if groupAlices > 1 {
				t.Fatalf("draw %d: %d tickets from Alice Mortimer's order in %s", i, groupAlices, g.Name)
			}
			alices += groupAlices
			if names["Ben Okoro"] > 0 && names["Carla Reyes"] > 0 {
				t.Fatalf("draw %d: Ben Okoro and Carla Reyes (same order) share %s", i, g.Name)
			}
		}
		if alices != 3 {
			t.Fatalf("draw %d: expected 3 Alice Mortimer tickets across the groups, found %d", i, alices)
		}
	}
}

// TestAutoDraw verifies automatic group sizing (draw with no num_groups):
// aim for groups of five so everyone gets at least four games, and when the
// numbers don't divide evenly, spill the remainder into groups of six rather
// than making smaller groups.
func TestAutoDraw(t *testing.T) {
	s := newTestServer(t)

	groupSizes := func() []int {
		t.Helper()
		var sizes []int
		for _, g := range listGroups(t, s) {
			sizes = append(sizes, len(g.Competitors))
		}
		sort.Ints(sizes)
		return sizes
	}
	importEvent := func(id string) {
		t.Helper()
		rec := do(t, s, "POST", "/api/tournament/import", `{"api_key":"demo","event_id":"`+id+`"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("import %s: got %d: %s", id, rec.Code, rec.Body)
		}
	}
	autoDraw := func() *httptest.ResponseRecorder {
		t.Helper()
		return do(t, s, "POST", "/api/tournament/draw", "")
	}

	// 16 players → 3 groups: two of 5 and one of 6.
	importEvent("demo-winter-classic")
	if rec := autoDraw(); rec.Code != http.StatusNoContent {
		t.Fatalf("auto draw with 16: got %d: %s", rec.Code, rec.Body)
	}
	if got := groupSizes(); !reflect.DeepEqual(got, []int{5, 5, 6}) {
		t.Fatalf("16 players: group sizes = %v, want two groups of 5 and one of 6", got)
	}

	// 40 players → 8 groups of 5.
	importEvent("demo-summer-open")
	if rec := autoDraw(); rec.Code != http.StatusNoContent {
		t.Fatalf("auto draw with 40: got %d: %s", rec.Code, rec.Body)
	}
	if got := groupSizes(); !reflect.DeepEqual(got, []int{5, 5, 5, 5, 5, 5, 5, 5}) {
		t.Fatalf("40 players: group sizes = %v, want eight groups of 5", got)
	}

	// 38 players → 7 groups: four of 5 and three of 6. (Removal marks the
	// competitor removed rather than deleting, so this stays last — the flag
	// would carry over into any later re-import.)
	importEvent("demo-summer-open")
	for _, c := range listCompetitors(t, s)[:2] {
		if rec := do(t, s, "DELETE", "/api/competitors/"+c.ID, ""); rec.Code != http.StatusNoContent {
			t.Fatalf("remove competitor: got %d", rec.Code)
		}
	}
	if rec := autoDraw(); rec.Code != http.StatusNoContent {
		t.Fatalf("auto draw with 38: got %d: %s", rec.Code, rec.Body)
	}
	if got := groupSizes(); !reflect.DeepEqual(got, []int{5, 5, 5, 5, 6, 6, 6}) {
		t.Fatalf("38 players: group sizes = %v, want four groups of 5 and three of 6", got)
	}
}

// TestDrawRejectsGroupsBelowMinimum: a manual group count that would push any
// group below the hard minimum of 4 players is refused.
func TestDrawRejectsGroupsBelowMinimum(t *testing.T) {
	s := newTestServer(t)
	importDemo(t, s) // 16 players

	// 5 groups of 16 → groups of 3, below the minimum.
	putSettings(t, s, `{"num_groups":5}`)
	rec := do(t, s, "POST", "/api/tournament/draw", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "at least 4 players") {
		t.Fatalf("unexpected error message: %s", rec.Body)
	}
}

// TestDrawRejectsImpossibleOrderSplit: a 3-ticket order cannot be kept apart
// across 2 groups, so that draw must be refused outright.
func TestDrawRejectsImpossibleOrderSplit(t *testing.T) {
	s := newTestServer(t)
	importDemo(t, s)

	putSettings(t, s, `{"num_groups":2}`)
	rec := do(t, s, "POST", "/api/tournament/draw", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400: %s", rec.Code, rec.Body)
	}
}
