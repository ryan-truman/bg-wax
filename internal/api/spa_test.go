package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// TestSPAServesClientRoutes: the popped-out display window is a fresh request
// to /display, not a client-side navigation, so the server has to answer it
// with index.html and let React Router take over. Real files must still be
// served as themselves.
func TestSPAServesClientRoutes(t *testing.T) {
	frontend := fstest.MapFS{
		"index.html":    {Data: []byte("<!doctype html><div id=root></div>")},
		"assets/app.js": {Data: []byte("console.log(1)")},
	}
	s := NewServer(newTestServer(t).db, frontend)

	for _, path := range []string{"/", "/display", "/matches", "/settings"} {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: got %d, want 200", path, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), `id=root`) {
			t.Fatalf("GET %s: want the app shell, got %q", path, rec.Body.String())
		}
	}

	req := httptest.NewRequest("GET", "/assets/app.js", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if body := rec.Body.String(); body != "console.log(1)" {
		t.Fatalf("real asset should be served as itself, got %q", body)
	}
}
