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

var demoNames = []string{
	"Alice Mortimer", "Ben Okoro", "Carla Reyes", "Dmitri Volkov",
	"Elena Vasquez", "Femi Adeyemi", "Grace Hartley", "Hiro Nakamura",
	"Ingrid Svensson", "Jomo Mwangi", "Kira Petrov", "Luca Ferretti",
	"Maya Osei", "Niall Brennan", "Olivia Chen", "Paulo Salave'a",
	"Quinn Dubois", "Rania El-Amin", "Sven Larsson", "Tara Nkosi",
	"Ugo Bianchi", "Vera Holloway", "Will Okonkwo", "Xena Papadopoulos",
	"Yusuf Hassan", "Zoe Tremblay", "Aaron Philips", "Bea Lindqvist",
	"Carlos Medina", "Dara Fitzpatrick", "Emeka Obi", "Fatima Al-Rashid",
	"George Stavros", "Hannah Byrne", "Ibrahim Al-Sayed", "Jade Okafor",
	"Kenji Watanabe", "Lucia Montoya", "Marco Russo", "Nadia Kowalski",
}

// DemoEvent returns the demo event with the given ID along with its issued
// tickets, mirroring what GetEvent + TicketsForEvent return for real events.
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

	names := demoNames
	if event.ID == "demo-winter-classic" {
		names = demoNames[:16]
	}

	tickets := make([]IssuedTicket, len(names))
	for i, name := range names {
		first, last, _ := strings.Cut(name, " ")
		tickets[i] = IssuedTicket{
			ID:        fmt.Sprintf("mock-ticket-%03d", i+1),
			FirstName: first,
			LastName:  last,
			Email:     demoEmail(name),
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
