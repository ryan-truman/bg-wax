package main

import (
	"log"

	"backgammon/internal/db"
)

func main() {
	database, err := db.Open("backgammon.db")
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	log.Println("database ready")
}
