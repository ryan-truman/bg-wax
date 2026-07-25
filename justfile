set dotenv-load

# Show the available commands (runs when you type `just` with no arguments).
default:
    @just --list

# The frontend rebuilds automatically while the server runs — refresh the browser
# to see changes. (Watch mode skips type checking; `just release` runs it.)

# Develop against mock data on an isolated demo.db (40 fake competitors).
demo:
    #!/usr/bin/env bash
    set -euo pipefail
    (cd frontend && npm run build)
    DB_PATH=demo.db go run ./cmd/seed --mock
    (cd frontend && exec npm run watch) &
    watcher=$!
    trap 'kill $watcher 2>/dev/null || true' EXIT
    DB_PATH=demo.db DEMO_MODE=1 FRONTEND_DIR=internal/web/dist go run ./cmd/server

# Start the server using the real database (competitors imported via Settings page)
serve:
    go run ./cmd/server

# Run this after changing any API response type so the TypeScript stays in sync.

# Regenerate frontend/src/types.ts from the Go API structs (via tygo).
types:
    go run github.com/gzuidhof/tygo@v0.2.21 generate

# Build and package the macOS (Apple Silicon) binary for distribution
release: types
    cd frontend && npm run build
    GOOS=darwin GOARCH=arm64 go build -o dist/bgandw ./cmd/server
    zip -j dist/bgandw dist/bgandw

# Install frontend dependencies (once after cloning)
setup:
    cd frontend && npm install
