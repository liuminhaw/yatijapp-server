DB_DSN := $(shell yq '.database.dsn' yatijapp.toml)

# =============================================================================
# HELPERS
# =============================================================================

## help: print this help message
.PHONY: help
help:
	@echo "Usage:"
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^/ /'

.PHONY: confirm
confirm:
	@echo -n "Are you sure? [y/N]: " && read ans && [ "$${ans:-N}" = "y" ]

# =============================================================================
# BUILD
# =============================================================================

## build/api: build the cmd/api application
.PHONY: build/api
build/api:
	@echo "Building cmd/api..."
	go build -ldflags='-s' -o bin/api ./cmd/api
	GOOS=linux GOARCH=amd64 go build -ldflags='-s' -o=./bin/linux_amd64/api ./cmd/api
	
# =============================================================================
# DEVELOPMENT
# =============================================================================

## run/api: run the cmd/api application
.PHONY: run/api
run/api:
	go run ./cmd/api

## db/migrations/new name=$1: create a new database migration
.PHONY: db/migrations/new
db/migrations/new:
	@echo "Creating migration file for ${name}..."
	migrate create -seq -ext=.sql -dir=./migrations ${name}

## db/migrations/version: print the current migration version
.PHONY: db/migrations/version
db/migrations/version:
	@echo "Current migration version..."
	migrate -path ./migrations -database "${DB_DSN}" version

## db/migrations/up: apply all up database migrations
.PHONY: db/migrations/up
db/migrations/up: confirm
	@echo "Running up migrations..."
	migrate -path ./migrations -database "${DB_DSN}" up

# =============================================================================
# QUALITY CONTROL
# =============================================================================

## tidy: tidy module dependencies and format all .go files
.PHONY: tidy
tidy:
	@echo "Tidying module dependencies..."
	go mod tidy
	@echo "Formatting .go files..."
	go fmt ./...

## audit: run quality control checks
.PHONY: audit
audit: 
	@echo "Checking module dependencies..."
	go mod tidy -diff
	go mod verify
	@echo "Vetting code..."
	go vet ./...
	go tool staticcheck ./...
	@echo "Running tests..."
	go test -race -vet=off ./...
