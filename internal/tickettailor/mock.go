package tickettailor

import (
	"fmt"
	"strings"
)

// DemoKey is a magic API key that serves the built-in demo dataset below
// instead of calling the real Ticket Tailor API, so the full import workflow
// (list events → pick one → import attendees) can be exercised without
// credentials. `just demo` seeds from the same data.
const DemoKey = "demo"

// DemoEvents are the events the demo account pretends to have. The Summer
// Open carries the full 40-attendee list; the Winter Classic a smaller 16,
// so different group/advance configurations can be tried.
var DemoEvents = []Event{
	{ID: "demo-summer-open", Name: "Backgammon and Wax — Summer Open 2026"},
	{ID: "demo-winter-classic", Name: "Backgammon and Wax — Winter Classic 2026"},
}

// demoOrders groups the demo tickets by purchase order: each inner slice is
// one order with one ticket per name. Alice Mortimer's order carries three
// tickets all issued under her own name — the "bought for friends" case the
// app must handle: import numbers the extras ("Alice Mortimer 2", "3") ready
// for renaming, and the draw must keep tickets from one order out of the same
// group. Ben and Carla were bought together
// too, but named individually. 40 tickets in total.
var demoOrders = [][]string{
	{"Alice Mortimer", "Alice Mortimer", "Alice Mortimer"},
	{"Ben Okoro", "Carla Reyes"},
	{"Dmitri Volkov"},
	{"Elena Vasquez"},
	{"Femi Adeyemi"},
	{"Grace Hartley"},
	{"Hiro Nakamura"},
	{"Ingrid Svensson"},
	{"Jomo Mwangi"},
	{"Kira Petrov"},
	{"Luca Ferretti"},
	{"Maya Osei"},
	{"Niall Brennan"},
	{"Olivia Chen"},
	{"Paulo Salave'a"},
	{"Quinn Dubois"},
	{"Rania El-Amin"},
	{"Sven Larsson"},
	{"Tara Nkosi"},
	{"Ugo Bianchi"},
	{"Vera Holloway"},
	{"Will Okonkwo"},
	{"Xena Papadopoulos"},
	{"Yusuf Hassan"},
	{"Zoe Tremblay"},
	{"Aaron Philips"},
	{"Bea Lindqvist"},
	{"Carlos Medina"},
	{"Dara Fitzpatrick"},
	{"Emeka Obi"},
	{"Fatima Al-Rashid"},
	{"George Stavros"},
	{"Hannah Byrne"},
	{"Ibrahim Al-Sayed"},
	{"Jade Okafor"},
	{"Kenji Watanabe"},
	{"Lucia Montoya"},
}

// DemoEvent returns the demo event with the given ID along with its issued
// tickets, mirroring what GetEvent + TicketsForEvent return for real events.
// The Winter Classic takes the first 16 tickets, which include both
// multi-ticket orders.
func DemoEvent(id string) (*Event, []IssuedTicket, error) {
	var event *Event
	for i := range DemoEvents {
		if DemoEvents[i].ID == id {
			event = &DemoEvents[i]
			break
		}
	}
	if event == nil {
		return nil, nil, fmt.Errorf("no demo event with id %q", id)
	}

	limit := 40
	if event.ID == "demo-winter-classic" {
		limit = 16
	}

	var tickets []IssuedTicket
	for oi, order := range demoOrders {
		for _, name := range order {
			if len(tickets) == limit {
				return event, tickets, nil
			}
			first, last, _ := strings.Cut(name, " ")
			tickets = append(tickets, IssuedTicket{
				ID:        fmt.Sprintf("mock-ticket-%03d", len(tickets)+1),
				OrderID:   fmt.Sprintf("mock-order-%03d", oi+1),
				FirstName: first,
				LastName:  last,
				Email:     demoEmail(name),
				Status:    "valid",
			})
		}
	}
	return event, tickets, nil
}

func demoEmail(name string) string {
	var b strings.Builder
	for _, c := range name {
		switch {
		case c >= 'a' && c <= 'z':
			b.WriteRune(c)
		case c >= 'A' && c <= 'Z':
			b.WriteRune(c + 32)
		case c == ' ':
			b.WriteByte('.')
		}
	}
	return b.String() + "@example.com"
}
