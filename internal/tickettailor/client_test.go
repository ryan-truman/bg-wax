package tickettailor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestDisplayNames: duplicate ticket names are suffixed "2", "3", … in ticket
// order; unique names and the first occurrence stay untouched.
func TestDisplayNames(t *testing.T) {
	tickets := []IssuedTicket{
		{FirstName: "Alice", LastName: "Mortimer"},
		{FirstName: "Ben", LastName: "Okoro"},
		{FirstName: "Alice", LastName: "Mortimer"},
		{FirstName: "Alice", LastName: "Mortimer"},
		{FirstName: "Carla", LastName: "Reyes"},
	}
	want := []string{"Alice Mortimer", "Ben Okoro", "Alice Mortimer 2", "Alice Mortimer 3", "Carla Reyes"}
	got := DisplayNames(tickets)
	if len(got) != len(want) {
		t.Fatalf("got %d names, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("name %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestTicketsForEventExcludesVoided verifies both layers of void handling:
// the request asks the API for valid tickets only, and any voided ticket that
// slips into a response anyway is dropped client-side.
func TestTicketsForEventExcludesVoided(t *testing.T) {
	var gotStatus string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/issued_tickets" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		gotStatus = r.URL.Query().Get("status")
		json.NewEncoder(w).Encode(map[string]any{
			"data": []IssuedTicket{
				{ID: "t1", OrderID: "o1", FirstName: "Ada", LastName: "Lovelace", Status: "valid"},
				{ID: "t2", OrderID: "o2", FirstName: "Void", LastName: "Refund", Status: "voided"},
				{ID: "t3", OrderID: "o3", FirstName: "Grace", LastName: "Hopper", Status: "valid"},
			},
			"links": map[string]string{"next": ""},
		})
	}))
	defer srv.Close()

	c := New("test-key")
	c.baseURL = srv.URL

	tickets, err := c.TicketsForEvent("ev_1")
	if err != nil {
		t.Fatal(err)
	}

	if gotStatus != "valid" {
		t.Errorf("status query param = %q, want %q", gotStatus, "valid")
	}
	if len(tickets) != 2 {
		t.Fatalf("got %d tickets, want 2 (voided excluded): %+v", len(tickets), tickets)
	}
	for _, tk := range tickets {
		if tk.Status != "valid" {
			t.Errorf("ticket %s has status %q", tk.ID, tk.Status)
		}
	}
}
