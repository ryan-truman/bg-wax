package main

import (
	"crypto/rand"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	"backgammon/internal/db"
	"backgammon/internal/tickettailor"
)

func main() {
	eventName := flag.String("event", "", "Ticket Tailor event name to import")
	mock := flag.Bool("mock", false, "Use fake data instead of calling Ticket Tailor")
	flag.Parse()

	if !*mock && *eventName == "" {
		fmt.Fprintln(os.Stderr, "usage: seed --event <event name>  OR  seed --mock")
		os.Exit(1)
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "backgammon.db"
	}
	database, err := db.Open(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	if *mock {
		if err := seedMock(database); err != nil {
			log.Fatal(err)
		}
		return
	}

	apiKey := os.Getenv("TICKET_TAILOR_API_KEY")
	if apiKey == "" {
		log.Fatal("TICKET_TAILOR_API_KEY environment variable is not set")
	}
	if err := wipeAndSeed(database, apiKey, *eventName); err != nil {
		log.Fatal(err)
	}
}

var mockNames = []string{
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

func seedMock(database *sql.DB) error {
	for _, table := range []string{"matches", "competitors", "groups", "tournaments"} {
		if _, err := database.Exec("DELETE FROM " + table); err != nil {
			return fmt.Errorf("clear %s: %w", table, err)
		}
	}

	tournamentID := newID()
	_, err := database.Exec(
		`INSERT INTO tournaments (id, name, status) VALUES (?, ?, 'setup')`,
		tournamentID, "Backgammon and Wax — Summer Open 2026",
	)
	if err != nil {
		return fmt.Errorf("insert tournament: %w", err)
	}

	for i, name := range mockNames {
		_, err := database.Exec(
			`INSERT INTO competitors (id, tournament_id, name, email, ticket_tailor_id)
			 VALUES (?, ?, ?, ?, ?)`,
			newID(), tournamentID, name,
			fmt.Sprintf("%s@example.com", mockEmail(name)),
			fmt.Sprintf("mock-ticket-%03d", i+1),
		)
		if err != nil {
			return fmt.Errorf("insert competitor %s: %w", name, err)
		}
	}

	log.Printf("seeded mock tournament with %d competitors", len(mockNames))
	return nil
}

func mockEmail(name string) string {
	result := ""
	for _, c := range name {
		switch {
		case c >= 'a' && c <= 'z':
			result += string(c)
		case c >= 'A' && c <= 'Z':
			result += string(c + 32)
		case c == ' ':
			result += "."
		}
	}
	return result
}

func wipeAndSeed(database *sql.DB, apiKey, eventName string) error {
	for _, table := range []string{"matches", "competitors", "groups", "tournaments"} {
		if _, err := database.Exec("DELETE FROM " + table); err != nil {
			return fmt.Errorf("clear %s: %w", table, err)
		}
	}

	client := tickettailor.New(apiKey)

	log.Printf("searching for event %q...", eventName)
	event, err := client.FindEventByName(eventName)
	if err != nil {
		return err
	}
	log.Printf("found event %s", event.ID)

	tournamentID := newID()
	_, err = database.Exec(
		`INSERT INTO tournaments (id, name, status) VALUES (?, ?, 'setup')`,
		tournamentID, event.Name,
	)
	if err != nil {
		return fmt.Errorf("insert tournament: %w", err)
	}

	log.Printf("fetching tickets...")
	tickets, err := client.TicketsForEvent(event.ID)
	if err != nil {
		return err
	}
	log.Printf("found %d tickets", len(tickets))

	for _, t := range tickets {
		_, err := database.Exec(
			`INSERT INTO competitors (id, tournament_id, name, email, ticket_tailor_id)
			 VALUES (?, ?, ?, ?, ?)`,
			newID(), tournamentID, t.FullName(), t.Email, t.ID,
		)
		if err != nil {
			return fmt.Errorf("insert competitor %s: %w", t.FullName(), err)
		}
	}

	log.Printf("seeded tournament %q with %d competitors", event.Name, len(tickets))
	return nil
}

func newID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
