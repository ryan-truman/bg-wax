set dotenv-load

# Display name of the macOS app bundle produced by `just build`.
app := "bg-wax"

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
test:
    go run ./cmd/server

# Run this after changing any API response type so the TypeScript stays in sync.

# Regenerate frontend/src/types.ts from the Go API structs (via tygo).
types:
    go run github.com/gzuidhof/tygo@v0.2.21 generate

# Build and package the macOS (Apple Silicon) .app bundle for distribution.
build: types
    cd frontend && npm run build
    rm -rf "dist/{{app}}.app" "dist/{{app}}.zip"
    mkdir -p "dist/{{app}}.app/Contents/MacOS" "dist/{{app}}.app/Contents/Resources"
    GOOS=darwin GOARCH=arm64 go build -o "dist/{{app}}.app/Contents/MacOS/bgwax-server" ./cmd/server
    cp build/launcher.sh "dist/{{app}}.app/Contents/MacOS/bgwax"
    cp build/Info.plist "dist/{{app}}.app/Contents/Info.plist"
    cp frontend/public/logo.png "dist/{{app}}.app/Contents/Resources/icon.png"
    chmod +x "dist/{{app}}.app/Contents/MacOS/bgwax" "dist/{{app}}.app/Contents/MacOS/bgwax-server"
    cd dist && zip -qr "{{app}}.zip" "{{app}}.app"
    @echo "Built dist/{{app}}.zip (Apple Silicon)"

# Install frontend dependencies (once after cloning)
setup:
    cd frontend && npm install
