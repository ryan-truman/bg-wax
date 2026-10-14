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
    @just bundle arm64 "{{app}}" "Apple Silicon"

# Same, for Intel Macs — produces dist/bg-wax-intel.zip.
build-intel: types
    cd frontend && npm run build
    @just bundle amd64 "{{app}}-intel" "Intel"

# Assemble the .app bundle for one architecture under dist/<arch>/ and zip it to
# dist/<out>.zip, so the two builds can sit side by side.
[private]
bundle arch out label:
    #!/usr/bin/env bash
    set -euo pipefail
    stage="dist/{{arch}}"
    rm -rf "$stage" "dist/{{out}}.zip"
    mkdir -p "$stage/{{app}}.app/Contents/MacOS" "$stage/{{app}}.app/Contents/Resources"
    GOOS=darwin GOARCH={{arch}} go build -o "$stage/{{app}}.app/Contents/MacOS/bgwax-server" ./cmd/server
    cp build/launcher.sh "$stage/{{app}}.app/Contents/MacOS/bgwax"
    cp build/Info.plist "$stage/{{app}}.app/Contents/Info.plist"
    cp frontend/public/logo.png "$stage/{{app}}.app/Contents/Resources/icon.png"
    chmod +x "$stage/{{app}}.app/Contents/MacOS/bgwax" "$stage/{{app}}.app/Contents/MacOS/bgwax-server"
    (cd "$stage" && zip -qr "../{{out}}.zip" "{{app}}.app")
    echo "Built dist/{{out}}.zip ({{label}})"

# Install frontend dependencies (once after cloning)
setup:
    cd frontend && npm install
