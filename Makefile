.PHONY: proto gen lint build up down

# Generate Go + TS from proto
proto:
	buf generate

# Lint proto files
lint:
	buf lint

# Build server binary
build:
	cd server && go build -o ../bin/agentregistry-server ./cmd/server

# Build CLI binary
cli:
	cd cli && go build -o ../bin/sockridge ./cmd

# Start full local stack
up:
	docker compose up --build

# Stop and remove volumes
down:
	docker compose down -v

# Run server locally (no docker)
run:
	cd server && go run ./cmd/server

# Tidy all modules
tidy:
	cd server && go mod tidy
	cd cli && go mod tidy
