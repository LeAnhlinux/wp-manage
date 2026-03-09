.PHONY: build build-api build-agent build-frontend dev dev-api dev-frontend clean proto test

# Variables
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS = -ldflags "-s -w -X main.Version=$(VERSION)"

# Build all
build: build-frontend build-api build-agent

# Build API server
build-api:
	@echo "Building API server..."
	CGO_ENABLED=0 go build $(LDFLAGS) -o bin/wp-manage-api ./cmd/api

# Build Agent
build-agent:
	@echo "Building Agent..."
	CGO_ENABLED=0 go build $(LDFLAGS) -o bin/wp-manage-agent ./cmd/agent

# Build for Linux (cross-compile)
build-linux:
	@echo "Building for Linux amd64..."
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $(LDFLAGS) -o bin/wp-manage-api-linux-amd64 ./cmd/api
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $(LDFLAGS) -o bin/wp-manage-agent-linux-amd64 ./cmd/agent
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build $(LDFLAGS) -o bin/wp-manage-api-linux-arm64 ./cmd/api
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build $(LDFLAGS) -o bin/wp-manage-agent-linux-arm64 ./cmd/agent

# Build frontend
build-frontend:
	@echo "Building frontend..."
	cd web && npm install && npm run build

# Development
dev-api:
	go run ./cmd/api

dev-frontend:
	cd web && npm run dev

dev:
	@echo "Starting API and Frontend in parallel..."
	@make dev-api & make dev-frontend

# Generate protobuf
proto:
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		internal/proto/agent.proto

# Test
test:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Clean
clean:
	rm -rf bin/ web/dist/ coverage.out coverage.html

# Install dev tools
setup:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	cd web && npm install
