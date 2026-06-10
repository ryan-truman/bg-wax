set dotenv-load

# Import competitors from Ticket Tailor and reset the database
seed event_name:
    go run ./cmd/seed --event "{{event_name}}"

# Seed with fake data (no API key needed)
mock:
    go run ./cmd/seed --mock

# Start the server
serve:
    go run ./cmd/server

# Seed then start the server
dev event_name: (seed event_name)
    go run ./cmd/server

# Run Go tests
test:
    go test ./...

# Build frontend assets
build-frontend:
    cd frontend && npm run build

# Cross-compile for Intel Mac
build-amd64: build-frontend
    GOOS=darwin GOARCH=amd64 go build -o dist/backgammon-amd64 ./cmd/server

# Cross-compile for Apple Silicon
build-arm64: build-frontend
    GOOS=darwin GOARCH=arm64 go build -o dist/backgammon-arm64 ./cmd/server

# Build both macOS targets and zip for distribution
release: build-amd64 build-arm64
    zip -j dist/backgammon-mac-intel.zip dist/backgammon-amd64
    zip -j dist/backgammon-mac-apple-silicon.zip dist/backgammon-arm64

# Install frontend dependencies
setup:
    cd frontend && npm install
