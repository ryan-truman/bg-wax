set dotenv-load

# Seed 40 fake competitors and start the server using an isolated demo database
demo:
    DB_PATH=demo.db go run ./cmd/seed --mock
    DB_PATH=demo.db go run ./cmd/server

# Start the server using the real database (competitors imported via Settings page)
serve:
    go run ./cmd/server

# Regenerate frontend/src/types.ts from the Go API structs in internal/api.
# Run this after changing any response type so the TypeScript stays in sync.
types:
    go run github.com/gzuidhof/tygo@v0.2.21 generate

# Build and package macOS binaries for distribution
release: types
    cd frontend && npm run build
    GOOS=darwin GOARCH=amd64 go build -o dist/backgammon-amd64 ./cmd/server
    GOOS=darwin GOARCH=arm64 go build -o dist/backgammon-arm64 ./cmd/server
    zip -j dist/backgammon-mac-intel.zip dist/backgammon-amd64
    zip -j dist/backgammon-mac-apple-silicon.zip dist/backgammon-arm64

# Install frontend dependencies (once after cloning)
setup:
    cd frontend && npm install
