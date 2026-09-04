package api

import (
	"context"
	"encoding/json"
	"fmt"
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

	// 84 players → 16 groups: twelve of 5 and four of 6. Nothing about the draw
	// caps the field — a large one-off draws like any other size.
	importEvent("demo-grand-open")
	if rec := autoDraw(); rec.Code != http.StatusNoContent {
		t.Fatalf("auto draw with 84: got %d: %s", rec.Code, rec.Body)
	}
	if got := groupSizes(); !reflect.DeepEqual(got, []int{5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 6, 6, 6, 6}) {
		t.Fatalf("84 players: group sizes = %v, want twelve groups of 5 and four of 6", got)
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

// TestGroupName covers naming past the 26th group, where single letters run
// out and the names go double-lettered.
func TestGroupName(t *testing.T) {
	for i, want := range map[int]string{0: "Group A", 25: "Group Z", 26: "Group AA", 27: "Group AB", 51: "Group AZ", 52: "Group BA"} {
		if got := groupName(i); got != want {
			t.Errorf("groupName(%d) = %q, want %q", i, got, want)
		}
	}
}

// TestListGroupsOrdersPastZ: with more than 26 groups the names are no longer
// all one letter, so listings order by name length before name — otherwise
// "Group AA" would sort in front of "Group B".
func TestListGroupsOrdersPastZ(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()

	tID := newID()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO tournaments (id, name, status) VALUES (?, 'T', 'group_stage')`, tID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 28; i++ {
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO groups (id, tournament_id, name) VALUES (?, ?, ?)`, newID(), tID, groupName(i)); err != nil {
			t.Fatal(err)
		}
	}

	var names []string
	for _, g := range listGroups(t, s) {
		names = append(names, g.Name)
	}
	if len(names) != 28 {
		t.Fatalf("got %d groups, want 28", len(names))
	}
	if names[0] != "Group A" || names[25] != "Group Z" || names[26] != "Group AA" || names[27] != "Group AB" {
		t.Fatalf("group order = %v, want Group A … Group Z, Group AA, Group AB", names)
	}
}

// TestLargeFieldImportAndDraw is the guard on tournament size: a one-off event
// twice the usual 40 imports in full and every competitor lands in a group.
func TestLargeFieldImportAndDraw(t *testing.T) {
	s := newTestServer(t)

	rec := do(t, s, "POST", "/api/tournament/import", `{"api_key":"demo","event_id":"demo-grand-open"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("import: got %d: %s", rec.Code, rec.Body)
	}
	var result struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode import result: %v", err)
	}
	if result.Count < 80 {
		t.Fatalf("imported %d competitors, want at least 80", result.Count)
	}
	if got := len(listCompetitors(t, s)); got != result.Count {
		t.Fatalf("listed %d competitors, want the %d imported", got, result.Count)
	}

	// 20 groups is a group count no 40-player field could ever have asked for.
	putSettings(t, s, `{"num_groups":20}`)
	if rec := do(t, s, "POST", "/api/tournament/draw", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("draw into 20 groups: got %d: %s", rec.Code, rec.Body)
	}

	groups := listGroups(t, s)
	if len(groups) != 20 {
		t.Fatalf("got %d groups, want 20", len(groups))
	}
	drawn := 0
	for _, g := range groups {
		if len(g.Competitors) < 4 {
			t.Errorf("%s has %d competitors, below the minimum of 4", g.Name, len(g.Competitors))
		}
		drawn += len(g.Competitors)
	}
	if drawn != result.Count {
		t.Fatalf("%d competitors landed in groups, want all %d", drawn, result.Count)
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

// TestDrawSpreadsOversizedOrder: a 3-ticket order cannot be kept fully apart
// across 2 groups, but that is no reason to refuse the draw — it goes ahead
// with the order spread as thinly as 2 groups allow (2 and 1, never 3 and 0).
func TestDrawSpreadsOversizedOrder(t *testing.T) {
	s := newTestServer(t)
	importDemo(t, s)

	putSettings(t, s, `{"num_groups":2}`)
	for i := 0; i < 10; i++ {
		if rec := do(t, s, "POST", "/api/tournament/draw", ""); rec.Code != http.StatusNoContent {
			t.Fatalf("draw %d: got %d, want 204: %s", i, rec.Code, rec.Body)
		}
		for _, g := range listGroups(t, s) {
			alices := 0
			for _, c := range g.Competitors {
				if strings.HasPrefix(c.Name, "Alice Mortimer") {
					alices++
				}
			}
			if alices > 2 {
				t.Fatalf("draw %d: %d of the 3-ticket order landed in %s, want no more than 2", i, alices, g.Name)
			}
		}
	}
}

// TestDrawWithBlockBooking covers the shape a corporate or venue block booking
// takes: one order holding half the field. No group count can keep 20 tickets
// apart in a 40-player draw, so the draw must not chase it — it sizes groups
// from the games-per-player target as usual and spreads the block evenly,
// rather than refusing to run at all.
func TestDrawWithBlockBooking(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()

	tID := newID()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO tournaments (id, name, status) VALUES (?, 'Block Booking', 'setup')`, tID); err != nil {
		t.Fatal(err)
	}
	const blockSize = 20
	blockIDs := map[string]bool{}
	for i := 0; i < 40; i++ {
		// The first 20 share one order; the rest bought individually.
		orderID := fmt.Sprintf("or_solo_%02d", i)
		if i < blockSize {
			orderID = "or_block"
		}
		cID := newID()
		if i < blockSize {
			blockIDs[cID] = true
		}
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO competitors (id, tournament_id, name, order_id) VALUES (?, ?, ?, ?)`,
			cID, tID, fmt.Sprintf("Player %02d", i+1), orderID); err != nil {
			t.Fatal(err)
		}
	}

	// Automatic sizing, 4 games per player: 40 players → 8 groups of 5.
	putSettings(t, s, `{"min_group_games":4,"num_groups":0}`)
	if rec := do(t, s, "POST", "/api/tournament/draw", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("draw with a 20-ticket block booking: got %d, want 204: %s", rec.Code, rec.Body)
	}

	groups := listGroups(t, s)
	if len(groups) != 8 {
		t.Fatalf("got %d groups, want 8 — the block booking must not change the sizing", len(groups))
	}
	// 20 tickets over 8 groups is 2 or 3 per group: evenly spread, not clumped.
	for _, g := range groups {
		if len(g.Competitors) != 5 {
			t.Errorf("%s has %d competitors, want 5", g.Name, len(g.Competitors))
		}
		fromBlock := 0
		for _, c := range g.Competitors {
			if blockIDs[c.ID] {
				fromBlock++
			}
		}
		if fromBlock > 3 {
			t.Errorf("%s holds %d of the block booking, want no more than 3", g.Name, fromBlock)
		}
	}
}
