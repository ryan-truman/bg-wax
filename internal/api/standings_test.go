package api

import (
	"context"
	"testing"
)

// TestGroupStandingsHeadToHead verifies the ranking priority: points, then
// head-to-head. Two players finish level on points (2); the one who beat the
// other must rank higher — even though the head-to-head winner sorts LAST
// alphabetically, so the test can only pass if head-to-head is applied rather
// than the stable name order falling through.
func TestGroupStandingsHeadToHead(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()

	tID, gID := newID(), newID()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("exec %q: %v", query, err)
		}
	}
	exec(`INSERT INTO tournaments (id, name, status) VALUES (?, 'T', 'group_stage')`, tID)
	exec(`INSERT INTO groups (id, tournament_id, name) VALUES (?, ?, 'Group A')`, gID, tID)

	// Names chosen so alphabetical order is Amy < Cara < Zoe.
	zoe, amy, cara := newID(), newID(), newID()
	exec(`INSERT INTO competitors (id, tournament_id, name, group_id) VALUES (?, ?, 'Zoe', ?)`, zoe, tID, gID)
	exec(`INSERT INTO competitors (id, tournament_id, name, group_id) VALUES (?, ?, 'Amy', ?)`, amy, tID, gID)
	exec(`INSERT INTO competitors (id, tournament_id, name, group_id) VALUES (?, ?, 'Cara', ?)`, cara, tID, gID)

	match := func(p1, p2, winner string, s1, s2 int) {
		exec(`INSERT INTO matches (id, tournament_id, stage, group_id, player1_id, player2_id, winner_id, player1_score, player2_score, status)
			VALUES (?, ?, 'group', ?, ?, ?, ?, ?, ?, 'complete')`,
			newID(), tID, gID, p1, p2, winner, s1, s2)
	}
	match(zoe, amy, zoe, 2, 0)  // Zoe beats Amy — the head-to-head result
	match(amy, cara, amy, 2, 0) // Amy beats Cara, leaving Zoe and Amy tied on 2 pts

	standings, err := s.groupStandings(ctx, gID)
	if err != nil {
		t.Fatalf("groupStandings: %v", err)
	}
	if len(standings) != 3 {
		t.Fatalf("standings = %d, want 3", len(standings))
	}
	if standings[0].Points != standings[1].Points {
		t.Fatalf("expected top two level on points, got %+v and %+v", standings[0], standings[1])
	}
	if standings[0].Name != "Zoe" {
		t.Fatalf("head-to-head winner should rank first, got %q (order: %q, %q, %q)",
			standings[0].Name, standings[0].Name, standings[1].Name, standings[2].Name)
	}
	if standings[2].Name != "Cara" {
		t.Fatalf("winless player should rank last, got %q", standings[2].Name)
	}
}

// TestGroupStandingsWinsBreakTie verifies the third differentiator: two players
// level on points who never met are split by wins. Zoe reached her points over
// more, smaller wins than Amy, so she ranks higher — and since Amy sorts first
// alphabetically, the test only passes if wins is actually applied.
func TestGroupStandingsWinsBreakTie(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()

	tID, gID := newID(), newID()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("exec %q: %v", query, err)
		}
	}
	exec(`INSERT INTO tournaments (id, name, status) VALUES (?, 'T', 'group_stage')`, tID)
	exec(`INSERT INTO groups (id, tournament_id, name) VALUES (?, ?, 'Group A')`, gID, tID)

	ids := map[string]string{}
	for _, name := range []string{"Amy", "Zoe", "Cara", "Dave"} {
		ids[name] = newID()
		exec(`INSERT INTO competitors (id, tournament_id, name, group_id) VALUES (?, ?, ?, ?)`, ids[name], tID, name, gID)
	}
	match := func(p1, p2, winner string, s1 int) {
		exec(`INSERT INTO matches (id, tournament_id, stage, group_id, player1_id, player2_id, winner_id, player1_score, player2_score, status)
			VALUES (?, ?, 'group', ?, ?, ?, ?, ?, 0, 'complete')`, newID(), tID, gID, p1, p2, winner, s1)
	}
	// Zoe: two one-point wins. Amy: a single two-point win. Level on points, and
	// they never played each other, so only wins can separate them.
	match(ids["Zoe"], ids["Cara"], ids["Zoe"], 1)
	match(ids["Zoe"], ids["Dave"], ids["Zoe"], 1)
	match(ids["Amy"], ids["Cara"], ids["Amy"], 2)

	standings, err := s.groupStandings(ctx, gID)
	if err != nil {
		t.Fatalf("groupStandings: %v", err)
	}
	if standings[0].Points != standings[1].Points {
		t.Fatalf("expected top two level on points, got %+v and %+v", standings[0], standings[1])
	}
	if standings[0].Name != "Zoe" {
		t.Fatalf("more wins should rank higher, got %q first (%+v)", standings[0].Name, standings[:2])
	}
}
