.PHONY: all run build test clean db-up db-down deps init-env help

# Application configuration
APP_NAME=toca-core
MAIN_FILE=cmd/toca-core/main.go
BIN_DIR=bin

# Database configuration
DB_CONTAINER=toca-mongo

all: build

help:
	@echo "Available commands:"
	@echo "  make run       - Run the application locally (loads .env if present)"
	@echo "  make build     - Build the application binary"
	@echo "  make test      - Run tests"
	@echo "  make clean     - Remove build artifacts"
	@echo "  make db-up     - Start local MongoDB container"
	@echo "  make db-down   - Stop local MongoDB container"
	@echo "  make deps      - Download Go dependencies"
	@echo "  make swagger   - Generate Swagger documentation"

run:	@echo "  make init-env  - Initialize .env file from example"

run:
	@if [ -f .env ]; then \
		echo "Loading environment from .env"; \
		export $$(grep -v '^#' .env | xargs) 2>/dev/null; \
		go run $(MAIN_FILE); \
	else \
		echo "No .env file found, using defaults"; \
		go run $(MAIN_FILE); \
	fi

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/$(APP_NAME) $(MAIN_FILE)

test:
	go test -v ./...

clean:
	rm -rf $(BIN_DIR)

db-up:
	@echo "Starting MongoDB..."
	docker-compose up -d

db-down:
	@echo "Stopping MongoDB..."
	docker-compose down

deps:
	go mod download
	go mod tidy

swagger:
	$(shell go env GOPATH)/bin/swag init --parseDependency -g cmd/toca-core/main.go

init-env:
	@if [ ! -f .env ]; then \
		cp env.example .env; \
		echo ".env file created from env.example"; \
	else \
		echo ".env file already exists"; \
	fi
