package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestMinGroupGamesSetting: the games-per-player target defaults to 4, is
// validated on write, persists (it survives a re-import), and drives the
// automatic draw — it's a minimum, with remainders making groups bigger.
func TestMinGroupGamesSetting(t *testing.T) {
	s := newTestServer(t)

	getMinGames := func() int {
		t.Helper()
		rec := do(t, s, "GET", "/api/settings", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("get settings: got %d", rec.Code)
		}
		var st Settings
		if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
			t.Fatalf("decode settings: %v", err)
		}
		return st.MinGroupGames
	}

	if got := getMinGames(); got != 4 {
		t.Fatalf("default min_group_games = %d, want 4", got)
	}
	// advance_total must be one of the powers of two a bracket can be built
	// from, so 6 and 96 are rejected as firmly as out-of-range values.
	for _, bad := range []string{`{"min_group_games":2}`, `{"min_group_games":11}`, `{"num_groups":201}`, `{"advance_total":1}`, `{"advance_total":17}`, `{"advance_total":6}`, `{"advance_total":96}`} {
		if rec := do(t, s, "PUT", "/api/settings", bad); rec.Code != http.StatusBadRequest {
			t.Fatalf("PUT %s: got %d, want 400", bad, rec.Code)
		}
	}
	// Updates are partial: an empty body is a valid no-op.
	if rec := do(t, s, "PUT", "/api/settings", `{}`); rec.Code != http.StatusNoContent {
		t.Fatalf("PUT {}: got %d, want 204", rec.Code)
	}
	if rec := do(t, s, "PUT", "/api/settings", `{"min_group_games":5}`); rec.Code != http.StatusNoContent {
		t.Fatalf("PUT valid setting: got %d: %s", rec.Code, rec.Body)
	}
	if got := getMinGames(); got != 5 {
		t.Fatalf("min_group_games after update = %d, want 5", got)
	}

	// A target of 5 games means groups of six: 40 players → 6 groups, with the
	// remainder of 4 making four groups of 7 (an extra game) and two of 6.
	if rec := do(t, s, "POST", "/api/tournament/import", `{"api_key":"demo","event_id":"demo-summer-open"}`); rec.Code != http.StatusOK {
		t.Fatalf("import: got %d: %s", rec.Code, rec.Body)
	}
	if rec := do(t, s, "POST", "/api/tournament/draw", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("auto draw: got %d: %s", rec.Code, rec.Body)
	}
	sizes := map[int]int{}
	for _, g := range listGroups(t, s) {
		sizes[len(g.Competitors)]++
	}
	if sizes[6] != 2 || sizes[7] != 4 || len(sizes) != 2 {
		t.Fatalf("group sizes with target 5 = %v, want two groups of 6 and four of 7", sizes)
	}

	// The setting lives outside the tournament tables, so re-importing (or
	// resetting) must not lose it.
	if rec := do(t, s, "POST", "/api/tournament/import", `{"api_key":"demo","event_id":"demo-summer-open"}`); rec.Code != http.StatusOK {
		t.Fatalf("re-import: got %d: %s", rec.Code, rec.Body)
	}
	if got := getMinGames(); got != 5 {
		t.Fatalf("min_group_games after re-import = %d, want 5", got)
	}
}

// TestAdvanceUsesSettings: advancing with an empty body (the Match History
// button) takes the qualifier count and bracket layout from the persisted
// settings rather than the request.
func TestAdvanceUsesSettings(t *testing.T) {
	s := newTestServer(t)
	importDemo(t, s) // 16 players
	putSettings(t, s, `{"num_groups":4}`)
	if rec := do(t, s, "POST", "/api/tournament/draw", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("draw: got %d: %s", rec.Code, rec.Body)
	}
	for _, m := range listMatches(t, s) {
		if code := patchMatch(t, s, m.ID, *m.Player1ID, 2); code != http.StatusOK {
			t.Fatalf("complete group match: got %d", code)
		}
	}

	if rec := do(t, s, "PUT", "/api/settings", `{"advance_total":4,"single_bracket":true}`); rec.Code != http.StatusNoContent {
		t.Fatalf("save settings: got %d: %s", rec.Code, rec.Body)
	}
	if rec := do(t, s, "POST", "/api/tournament/advance", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("advance from settings: got %d: %s", rec.Code, rec.Body)
	}

	// A total of 4 across 4 groups is one each, in a single bracket: 2 semis +
	// 1 final, all in bracket 1.
	bracket := listBracket(t, s)
	if len(bracket) != 3 {
		t.Fatalf("knockout matches = %d, want 3 (2 semis + final)", len(bracket))
	}
	for _, m := range bracket {
		if m.Bracket != nil && *m.Bracket != 1 {
			t.Fatalf("expected a single bracket, found match in bracket %d", *m.Bracket)
		}
	}
}
