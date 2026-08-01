package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// tieFixture builds a two-group tournament where Group A's three players are in
// a head-to-head cycle on equal points. With one place per group, the ranking
// rules cannot say which of the three progresses.
func tieFixture(t *testing.T) (s *Server, ids map[string]string) {
	t.Helper()
	s = newTestServer(t)
	ctx := context.Background()

	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("exec %q: %v", query, err)
		}
	}
	tID := newID()
	exec(`INSERT INTO tournaments (id, name, status) VALUES (?, 'T', 'group_stage')`, tID)

	gA, gB := newID(), newID()
	exec(`INSERT INTO groups (id, tournament_id, name) VALUES (?, ?, 'Group A')`, gA, tID)
	exec(`INSERT INTO groups (id, tournament_id, name) VALUES (?, ?, 'Group B')`, gB, tID)

	ids = map[string]string{}
	add := func(name, gid string) {
		ids[name] = newID()
		exec(`INSERT INTO competitors (id, tournament_id, name, group_id) VALUES (?, ?, ?, ?)`, ids[name], tID, name, gid)
	}
	for _, n := range []string{"Alice", "Ben", "Cara"} {
		add(n, gA)
	}
	for _, n := range []string{"Dan", "Eve"} {
		add(n, gB)
	}

	match := func(gid, p1, p2, winner string) {
		exec(`INSERT INTO matches (id, tournament_id, stage, group_id, player1_id, player2_id, winner_id, player1_score, player2_score, status)
			VALUES (?, ?, 'group', ?, ?, ?, ?, 2, 0, 'complete')`,
			newID(), tID, gid, p1, p2, winner)
	}
	// A beats B, B beats C, C beats A — everyone on one win and two points.
	match(gA, ids["Alice"], ids["Ben"], ids["Alice"])
	match(gA, ids["Ben"], ids["Cara"], ids["Ben"])
	match(gA, ids["Cara"], ids["Alice"], ids["Cara"])
	match(gB, ids["Dan"], ids["Eve"], ids["Dan"])

	// One bracket of two: the top finisher from each group.
	putSettings(t, s, `{"advance_total":2,"single_bracket":true}`)
	return s, ids
}

func advanceTies(t *testing.T, s *Server, body string) []TieBreak {
	t.Helper()
	rec := do(t, s, "POST", "/api/tournament/advance", body)
	if rec.Code != http.StatusConflict {
		t.Fatalf("advance: got %d, want 409: %s", rec.Code, rec.Body)
	}
	var resp struct {
		Ties []TieBreak `json:"ties"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode ties: %v", err)
	}
	return resp.Ties
}

// TestAdvanceReportsUnresolvableTie: a cycle at the qualifying cut stops the
// advance and reports the tied players instead of picking one arbitrarily.
func TestAdvanceReportsUnresolvableTie(t *testing.T) {
	s, _ := tieFixture(t)

	ties := advanceTies(t, s, "")
	if len(ties) != 1 {
		t.Fatalf("ties = %d, want 1: %+v", len(ties), ties)
	}
	tie := ties[0]
	if tie.Scope != "group" || tie.GroupName != "Group A" {
		t.Fatalf("tie scope/group = %q/%q, want group/Group A", tie.Scope, tie.GroupName)
	}
	if tie.Slots != 1 {
		t.Fatalf("tie slots = %d, want 1 (one place for the three tied)", tie.Slots)
	}
	if len(tie.Competitors) != 3 {
		t.Fatalf("tied competitors = %d, want 3", len(tie.Competitors))
	}
	for _, c := range tie.Competitors {
		if c.Points != 2 || c.Won != 1 {
			t.Fatalf("%s has %d pts / %d won, want the tied 2/1", c.Name, c.Points, c.Won)
		}
	}
	// Nothing may be written while a tie is outstanding.
	if len(listBracket(t, s)) != 0 {
		t.Fatal("bracket was created despite an unsettled tie")
	}
	if got := tournamentStatus(t, s); got != "group_stage" {
		t.Fatalf("status = %q, want group_stage while a tie is outstanding", got)
	}
}

// TestAdvanceAppliesTieBreak: the order the organiser chooses decides who
// progresses, including picking someone the automatic order had ranked last.
func TestAdvanceAppliesTieBreak(t *testing.T) {
	s, ids := tieFixture(t)
	tie := advanceTies(t, s, "")[0]

	// Cara is nominated to progress ahead of the other two.
	body, err := json.Marshal(map[string]any{
		"tie_breaks": map[string][]string{
			tie.ID: {ids["Cara"], ids["Alice"], ids["Ben"]},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if rec := do(t, s, "POST", "/api/tournament/advance", string(body)); rec.Code != http.StatusNoContent {
		t.Fatalf("advance with tie break: got %d: %s", rec.Code, rec.Body)
	}

	bracket := listBracket(t, s)
	if len(bracket) != 1 {
		t.Fatalf("knockout matches = %d, want 1 final", len(bracket))
	}
	got := map[string]bool{}
	for _, m := range bracket {
		if m.Player1ID != nil {
			got[*m.Player1ID] = true
		}
		if m.Player2ID != nil {
			got[*m.Player2ID] = true
		}
	}
	if !got[ids["Cara"]] {
		t.Fatal("Cara was chosen to progress but is not in the bracket")
	}
	if got[ids["Alice"]] || got[ids["Ben"]] {
		t.Fatal("a player the organiser ranked below Cara progressed")
	}
	if !got[ids["Dan"]] {
		t.Fatal("Group B's winner is missing from the bracket")
	}
}

// TestAdvanceRejectsStaleTieBreak: an answer that does not name exactly the
// tied players is ignored rather than half-applied, so the tie is asked again.
func TestAdvanceRejectsStaleTieBreak(t *testing.T) {
	s, ids := tieFixture(t)
	tie := advanceTies(t, s, "")[0]

	body, err := json.Marshal(map[string]any{
		"tie_breaks": map[string][]string{
			tie.ID: {ids["Cara"], ids["Alice"]}, // Ben missing
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if ties := advanceTies(t, s, string(body)); len(ties) != 1 {
		t.Fatalf("ties = %d, want the tie asked again", len(ties))
	}
}

// TestPoolTieReportsWhereLosersLand: a tie for the leftover "best runners-up"
// places in the top bracket must say that whoever misses out drops into the
// bracket below, rather than leaving the organiser to guess.
func TestPoolTieReportsWhereLosersLand(t *testing.T) {
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

	// Three groups of two. Each group's winner takes the one guaranteed place,
	// leaving the three runners-up — all on nothing, none having met — to fight
	// over a single leftover place.
	for _, name := range []string{"A", "B", "C"} {
		gid := newID()
		exec(`INSERT INTO groups (id, tournament_id, name) VALUES (?, ?, ?)`, gid, tID, "Group "+name)
		winner, loser := newID(), newID()
		exec(`INSERT INTO competitors (id, tournament_id, name, group_id) VALUES (?, ?, ?, ?)`, winner, tID, name+" Winner", gid)
		exec(`INSERT INTO competitors (id, tournament_id, name, group_id) VALUES (?, ?, ?, ?)`, loser, tID, name+" Runner", gid)
		exec(`INSERT INTO matches (id, tournament_id, stage, group_id, player1_id, player2_id, winner_id, player1_score, player2_score, status)
			VALUES (?, ?, 'group', ?, ?, ?, ?, 2, 0, 'complete')`, newID(), tID, gid, winner, loser, winner)
	}
	// Four per bracket over three groups: one guaranteed place each, one left over.
	putSettings(t, s, `{"advance_total":4,"single_bracket":false}`)

	ties := advanceTies(t, s, "")
	if len(ties) != 1 {
		t.Fatalf("ties = %d, want 1: %+v", len(ties), ties)
	}
	tie := ties[0]
	if tie.Scope != "pool" {
		t.Fatalf("scope = %q, want pool", tie.Scope)
	}
	if tie.Bracket != 1 {
		t.Fatalf("bracket = %d, want 1 (the tie is for the top bracket)", tie.Bracket)
	}
	if tie.Slots != 1 || len(tie.Competitors) != 3 {
		t.Fatalf("want 3 players for 1 place, got %d for %d", len(tie.Competitors), tie.Slots)
	}
	if tie.DropsToBracket != 2 {
		t.Fatalf("drops_to_bracket = %d, want 2 — the two who miss out fall to the lower bracket", tie.DropsToBracket)
	}
	if tie.DropsToPool {
		t.Fatal("a pool tie has no further pool to drop into")
	}
	// The candidates come from three different groups, which the dialog shows.
	seen := map[string]bool{}
	for _, c := range tie.Competitors {
		seen[c.GroupName] = true
	}
	if len(seen) != 3 {
		t.Fatalf("candidate groups = %v, want one from each group", seen)
	}
}

// TestWinsSettleCutWithoutAsking: players level on points who never met are
// separated by wins, so the cut is decided by the rules and the organiser is
// not asked. The player with more wins is the one who progresses.
func TestWinsSettleCutWithoutAsking(t *testing.T) {
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

	gA, gB := newID(), newID()
	exec(`INSERT INTO groups (id, tournament_id, name) VALUES (?, ?, 'Group A')`, gA, tID)
	exec(`INSERT INTO groups (id, tournament_id, name) VALUES (?, ?, 'Group B')`, gB, tID)

	ids := map[string]string{}
	add := func(name, gid string) {
		ids[name] = newID()
		exec(`INSERT INTO competitors (id, tournament_id, name, group_id) VALUES (?, ?, ?, ?)`, ids[name], tID, name, gid)
	}
	for _, n := range []string{"Alice", "Bob", "Cara", "Dave"} {
		add(n, gA)
	}
	for _, n := range []string{"Eve", "Frank"} {
		add(n, gB)
	}
	match := func(gid, p1, p2, winner string, s1 int) {
		exec(`INSERT INTO matches (id, tournament_id, stage, group_id, player1_id, player2_id, winner_id, player1_score, player2_score, status)
			VALUES (?, ?, 'group', ?, ?, ?, ?, ?, 0, 'complete')`, newID(), tID, gid, p1, p2, winner, s1)
	}
	// Bob wins twice for a point each; Alice wins once for two. Level on points
	// at the top of Group A, and the two never played each other.
	match(gA, ids["Bob"], ids["Cara"], ids["Bob"], 1)
	match(gA, ids["Bob"], ids["Dave"], ids["Bob"], 1)
	match(gA, ids["Alice"], ids["Cara"], ids["Alice"], 2)
	match(gB, ids["Eve"], ids["Frank"], ids["Eve"], 2)

	// One place per group, so the Alice/Bob cut decides who progresses.
	putSettings(t, s, `{"advance_total":2,"single_bracket":true}`)
	if rec := do(t, s, "POST", "/api/tournament/advance", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("advance: got %d, want wins to settle the cut unasked: %s", rec.Code, rec.Body)
	}

	in := map[string]bool{}
	for _, m := range listBracket(t, s) {
		if m.Player1ID != nil {
			in[*m.Player1ID] = true
		}
		if m.Player2ID != nil {
			in[*m.Player2ID] = true
		}
	}
	if !in[ids["Bob"]] {
		t.Fatal("Bob has more wins on equal points but did not progress")
	}
	if in[ids["Alice"]] {
		t.Fatal("Alice progressed despite fewer wins on equal points")
	}
}

// TestAdvanceIgnoresTieThatChangesNothing: players level on points who all
// qualify anyway are ordered automatically, without troubling the organiser.
func TestAdvanceIgnoresTieThatChangesNothing(t *testing.T) {
	s, _ := tieFixture(t)
	// Four per group takes the whole of Group A, so the cycle no longer
	// straddles a cut — all three progress whatever order they are put in.
	putSettings(t, s, `{"advance_total":8,"single_bracket":true}`)

	if rec := do(t, s, "POST", "/api/tournament/advance", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("advance: got %d, want it to proceed without asking: %s", rec.Code, rec.Body)
	}
}
