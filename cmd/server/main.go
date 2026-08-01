package main

import (
	"context"
	"io/fs"
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
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "backgammon.db"
	}
	database, err := db.Open(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	var frontendFS fs.FS
	if dir := os.Getenv("FRONTEND_DIR"); dir != "" {
		frontendFS = os.DirFS(dir)
	} else {
		frontendFS = web.FS
	}

	server := &http.Server{
		Addr:    ":8080",
		Handler: api.NewServer(database, frontendFS),
	}

	go func() {
		log.Println("listening on http://localhost:8080")
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	time.Sleep(200 * time.Millisecond)
	openBrowser("http://localhost:8080")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(ctx)
	log.Println("stopped")
}

func openBrowser(url string) {
	appURL := "--app=" + url
	switch runtime.GOOS {
	case "darwin":
		cmd := exec.Command("open", "-a", "Google Chrome", "--args", appURL)
		if cmd.Start() != nil {
			exec.Command("open", url).Start()
		}
	case "linux":
		// Try chromium/chrome in app mode for the standalone PWA window.
		// Fall back to xdg-open if neither is installed.
		for _, bin := range []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable"} {
			if _, err := exec.LookPath(bin); err == nil {
				exec.Command(bin, appURL).Start()
				return
			}
		}
		exec.Command("xdg-open", url).Start()
	}
}
