package api

import (
	"net/http"
	"testing"
)

// groupOf returns the group a competitor is in, by name, plus that group's ID.
func groupOf(t *testing.T, s *Server, name string) (string, string) {
	t.Helper()
	for _, g := range listGroups(t, s) {
		for _, c := range g.Competitors {
			if c.Name == name {
				return g.Name, g.ID
			}
		}
	}
	t.Fatalf("competitor %q is not in any group", name)
	return "", ""
}

// competitorID finds a competitor by name in the current tournament.
func competitorID(t *testing.T, s *Server, name string) string {
	t.Helper()
	for _, c := range listCompetitors(t, s) {
		if c.Name == name {
			return c.ID
		}
	}
	t.Fatalf("no competitor named %q", name)
	return ""
}

// opponentsOf returns the names a competitor has group fixtures against.
func opponentsOf(t *testing.T, s *Server, id string) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	for _, m := range listMatches(t, s) {
		if m.Stage != MatchStageGroup {
			continue
		}
		if m.Player1ID != nil && *m.Player1ID == id {
			names[*m.Player2Name] = true
		}
		if m.Player2ID != nil && *m.Player2ID == id {
			names[*m.Player1Name] = true
		}
	}
	return names
}

// setupDrawnTournament imports the small demo event and draws 4 groups of 4.
func setupDrawnTournament(t *testing.T) *Server {
	t.Helper()
	s := newTestServer(t)
	importDemo(t, s) // 16 players
	putSettings(t, s, `{"num_groups":4}`)
	if rec := do(t, s, "POST", "/api/tournament/draw", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("draw: got %d: %s", rec.Code, rec.Body)
	}
	return s
}

// TestMoveCompetitorBetweenGroups: a move after the draw takes the player's
// fixtures with it — the old group's are gone, and they now play everyone in
// the group they joined.
func TestMoveCompetitorBetweenGroups(t *testing.T) {
	s := setupDrawnTournament(t)

	// Pick someone and a group that isn't theirs.
	player := listCompetitors(t, s)[0]
	fromName, _ := groupOf(t, s, player.Name)
	var target Group
	for _, g := range listGroups(t, s) {
		if g.Name != fromName {
			target = g
			break
		}
	}

	wantOpponents := map[string]bool{}
	for _, c := range target.Competitors {
		wantOpponents[c.Name] = true
	}

	if rec := do(t, s, "POST", "/api/competitors/"+player.ID+"/group", `{"group_id":"`+target.ID+`"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("move: got %d: %s", rec.Code, rec.Body)
	}

	gotName, _ := groupOf(t, s, player.Name)
	if gotName != target.Name {
		t.Fatalf("%s is in %s after the move, want %s", player.Name, gotName, target.Name)
	}

	// The group they left must no longer list them, and must have shrunk.
	for _, g := range listGroups(t, s) {
		if g.Name != fromName {
			continue
		}
		if len(g.Competitors) != 3 {
			t.Errorf("%s has %d competitors after losing one, want 3", fromName, len(g.Competitors))
		}
		for _, c := range g.Competitors {
			if c.ID == player.ID {
				t.Errorf("%s still lists %s", fromName, player.Name)
			}
		}
	}

	// Fixtures: exactly the new group-mates, nobody from the old group.
	got := opponentsOf(t, s, player.ID)
	if len(got) != len(wantOpponents) {
		t.Fatalf("%s has %d fixtures, want %d: %v", player.Name, len(got), len(wantOpponents), got)
	}
	for name := range wantOpponents {
		if !got[name] {
			t.Errorf("%s has no fixture against new group-mate %s", player.Name, name)
		}
	}

	// Every other match in the tournament still has both players in one group.
	groupByPlayer := map[string]string{}
	for _, g := range listGroups(t, s) {
		for _, c := range g.Competitors {
			groupByPlayer[c.ID] = g.ID
		}
	}
	for _, m := range listMatches(t, s) {
		if m.Stage != MatchStageGroup {
			continue
		}
		if groupByPlayer[*m.Player1ID] != groupByPlayer[*m.Player2ID] {
			t.Errorf("match %s pairs players from different groups", m.ID)
		}
	}
}

// TestMoveCompetitorDiscardsTheirResults: moving a player who has already
// played drops those games — they were fixtures in a group they have left —
// so neither they nor their old opponents keep the record.
func TestMoveCompetitorDiscardsTheirResults(t *testing.T) {
	s := setupDrawnTournament(t)

	// Play one match, then move its winner out of that group.
	var played Match
	for _, m := range listMatches(t, s) {
		if m.Stage == MatchStageGroup {
			played = m
			break
		}
	}
	if code := patchMatch(t, s, played.ID, *played.Player1ID, 2); code != http.StatusOK {
		t.Fatalf("record result: got %d", code)
	}

	winnerID, loserName := *played.Player1ID, *played.Player2Name
	fromName, _ := groupOf(t, s, *played.Player1Name)
	var targetID string
	for _, g := range listGroups(t, s) {
		if g.Name != fromName {
			targetID = g.ID
			break
		}
	}

	if rec := do(t, s, "POST", "/api/competitors/"+winnerID+"/group", `{"group_id":"`+targetID+`"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("move: got %d: %s", rec.Code, rec.Body)
	}

	// The loser's record is back to nothing: the game they lost is gone.
	for _, g := range listGroups(t, s) {
		for _, c := range g.Competitors {
			if c.Name == loserName && (c.Played != 0 || c.Lost != 0) {
				t.Errorf("%s still shows played=%d lost=%d after their opponent moved away", loserName, c.Played, c.Lost)
			}
			if c.ID == winnerID && c.Played != 0 {
				t.Errorf("moved player shows played=%d in their new group, want 0", c.Played)
			}
		}
	}
}

// TestMoveCompetitorRejections covers the guards: the group must exist and be
// a different one, the player must exist and still be in the tournament, and
// moves only make sense while the group stage is running.
func TestMoveCompetitorRejections(t *testing.T) {
	s := setupDrawnTournament(t)

	player := listCompetitors(t, s)[0]
	_, currentGroupID := groupOf(t, s, player.Name)
	otherGroupID := ""
	for _, g := range listGroups(t, s) {
		if g.ID != currentGroupID {
			otherGroupID = g.ID
			break
		}
	}

	cases := []struct {
		name, path, body string
		want             int
	}{
		{"no group_id", "/api/competitors/" + player.ID + "/group", `{}`, http.StatusBadRequest},
		{"same group", "/api/competitors/" + player.ID + "/group", `{"group_id":"` + currentGroupID + `"}`, http.StatusBadRequest},
		{"unknown group", "/api/competitors/" + player.ID + "/group", `{"group_id":"nope"}`, http.StatusNotFound},
		{"unknown competitor", "/api/competitors/nope/group", `{"group_id":"` + otherGroupID + `"}`, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if rec := do(t, s, "POST", tc.path, tc.body); rec.Code != tc.want {
				t.Fatalf("got %d, want %d: %s", rec.Code, tc.want, rec.Body)
			}
		})
	}

	// A removed player has to be restored before they can be moved.
	t.Run("removed competitor", func(t *testing.T) {
		if rec := do(t, s, "DELETE", "/api/competitors/"+player.ID, ""); rec.Code != http.StatusNoContent {
			t.Fatalf("remove: got %d", rec.Code)
		}
		rec := do(t, s, "POST", "/api/competitors/"+player.ID+"/group", `{"group_id":"`+otherGroupID+`"}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("got %d, want 400: %s", rec.Code, rec.Body)
		}
		if rec := do(t, s, "POST", "/api/competitors/"+player.ID+"/restore", ""); rec.Code != http.StatusNoContent {
			t.Fatalf("restore: got %d", rec.Code)
		}
	})

	// Before the draw there are no groups to move between; once the knockout is
	// running the group stage is history.
	t.Run("wrong tournament status", func(t *testing.T) {
		s := newTestServer(t)
		importDemo(t, s)
		rec := do(t, s, "POST", "/api/competitors/"+listCompetitors(t, s)[0].ID+"/group", `{"group_id":"any"}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("move during setup: got %d, want 400: %s", rec.Code, rec.Body)
		}
	})
}

// TestMoveCompetitorSurvivesAdvance: a group stage that has been reshuffled by
// hand still advances — the moved player's new fixtures count towards the
// group they joined.
func TestMoveCompetitorSurvivesAdvance(t *testing.T) {
	s := setupDrawnTournament(t)

	player := listCompetitors(t, s)[0]
	fromName, _ := groupOf(t, s, player.Name)
	var targetID, targetName string
	for _, g := range listGroups(t, s) {
		if g.Name != fromName {
			targetID, targetName = g.ID, g.Name
			break
		}
	}
	if rec := do(t, s, "POST", "/api/competitors/"+player.ID+"/group", `{"group_id":"`+targetID+`"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("move: got %d: %s", rec.Code, rec.Body)
	}

	for _, m := range listMatches(t, s) {
		if code := patchMatch(t, s, m.ID, *m.Player1ID, 2); code != http.StatusOK {
			t.Fatalf("complete group match: got %d", code)
		}
	}

	// The moved player's games are counted in their new group's standings.
	for _, g := range listGroups(t, s) {
		if g.Name != targetName {
			continue
		}
		for _, c := range g.Competitors {
			if c.ID == player.ID && c.Played != len(g.Competitors)-1 {
				t.Errorf("moved player played %d games in a group of %d, want %d", c.Played, len(g.Competitors), len(g.Competitors)-1)
			}
		}
	}

	putSettings(t, s, `{"advance_total":4,"single_bracket":true}`)
	if rec := do(t, s, "POST", "/api/tournament/advance", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("advance after a manual move: got %d: %s", rec.Code, rec.Body)
	}
	if got := tournamentStatus(t, s); got != "knockout" {
		t.Fatalf("status = %q, want knockout", got)
	}
}
