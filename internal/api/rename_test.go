package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"backgammon/internal/db"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return NewServer(database, nil)
}

func do(t *testing.T, s *Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

// importDemo loads the built-in 16-competitor demo event, which exercises the
// real import path without touching the Ticket Tailor API.
func importDemo(t *testing.T, s *Server) {
	t.Helper()
	rec := do(t, s, "POST", "/api/tournament/import", `{"api_key":"demo","event_id":"demo-winter-classic"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("import: got %d: %s", rec.Code, rec.Body)
	}
}

// putSettings persists organiser preferences — the draw and advance endpoints
// take no request parameters, so tests configure them the same way the UI
// does.
func putSettings(t *testing.T, s *Server, body string) {
	t.Helper()
	if rec := do(t, s, "PUT", "/api/settings", body); rec.Code != http.StatusNoContent {
		t.Fatalf("put settings %s: got %d: %s", body, rec.Code, rec.Body)
	}
}

func listCompetitors(t *testing.T, s *Server) []Competitor {
	t.Helper()
	rec := do(t, s, "GET", "/api/competitors", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list competitors: got %d", rec.Code)
	}
	var cs []Competitor
	if err := json.Unmarshal(rec.Body.Bytes(), &cs); err != nil {
		t.Fatalf("decode competitors: %v", err)
	}
	return cs
}

func TestRenameCompetitor(t *testing.T) {
	s := newTestServer(t)
	importDemo(t, s)
	target := listCompetitors(t, s)[0]

	rec := do(t, s, "PATCH", "/api/competitors/"+target.ID, `{"name":"  Actual Player  "}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("rename: got %d: %s", rec.Code, rec.Body)
	}

	found := false
	for _, c := range listCompetitors(t, s) {
		if c.ID == target.ID {
			found = true
			if c.Name != "Actual Player" {
				t.Fatalf("name = %q, want %q (trimmed)", c.Name, "Actual Player")
			}
		}
	}
	if !found {
		t.Fatalf("renamed competitor missing from list")
	}

	if rec := do(t, s, "PATCH", "/api/competitors/"+target.ID, `{"name":"   "}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("blank name: got %d, want 400", rec.Code)
	}
	if rec := do(t, s, "PATCH", "/api/competitors/nonexistent", `{"name":"X"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown id: got %d, want 404", rec.Code)
	}
}

// TestImportNumbersDuplicateNames: several tickets bought under one name must
// come out of the import as distinguishable competitors — the extras get a
// numeric suffix — and the numbering must survive a re-import unchanged.
func TestImportNumbersDuplicateNames(t *testing.T) {
	s := newTestServer(t)
	importDemo(t, s)

	names := func() map[string]int {
		counts := map[string]int{}
		for _, c := range listCompetitors(t, s) {
			counts[c.Name]++
		}
		return counts
	}

	got := names()
	for _, want := range []string{"Alice Mortimer", "Alice Mortimer 2", "Alice Mortimer 3"} {
		if got[want] != 1 {
			t.Errorf("expected exactly one %q, found %d", want, got[want])
		}
	}
	for name, n := range got {
		if n > 1 {
			t.Errorf("duplicate competitor name %q after import (%d occurrences)", name, n)
		}
	}

	// Refreshing the contestant list must not renumber anyone.
	importDemo(t, s)
	after := names()
	for name, n := range got {
		if after[name] != n {
			t.Errorf("name %q changed across re-import: %d -> %d", name, n, after[name])
		}
	}
}

func TestReimportPreservesLocalEdits(t *testing.T) {
	s := newTestServer(t)
	importDemo(t, s)
	cs := listCompetitors(t, s)

	// Import numbers duplicate ticket names, so every competitor name is
	// unique and safe to assert on.
	renamed, removed := cs[0], cs[1]

	if rec := do(t, s, "PATCH", "/api/competitors/"+renamed.ID, `{"name":"Actual Player"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("rename: got %d", rec.Code)
	}
	if rec := do(t, s, "DELETE", "/api/competitors/"+removed.ID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("remove: got %d", rec.Code)
	}

	// Refreshing the contestant list must keep the rename and the removal.
	importDemo(t, s)

	names := map[string]bool{}
	for _, c := range listCompetitors(t, s) {
		names[c.Name] = true
	}
	if !names["Actual Player"] {
		t.Fatal("rename lost on re-import")
	}
	if names[renamed.Name] {
		t.Fatalf("original ticket name %q resurfaced after re-import", renamed.Name)
	}
	if names[removed.Name] {
		t.Fatalf("removed competitor %q resurrected by re-import", removed.Name)
	}
}
