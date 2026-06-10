package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"backgammon/internal/api"
	"backgammon/internal/db"
	"backgammon/internal/web"
)

func main() {
	database, err := db.Open("backgammon.db")
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	server := &http.Server{
		Addr:    ":8080",
		Handler: api.NewServer(database, web.FS),
	}

	go func() {
		log.Println("listening on http://localhost:8080")
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	if runtime.GOOS == "darwin" {
		time.Sleep(200 * time.Millisecond) // brief pause so the server is ready
		exec.Command("open", "-a", "Safari", "http://localhost:8080").Start()
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(ctx)
	log.Println("stopped")
}
