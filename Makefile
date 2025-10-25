.PHONY: build run test clean docker-build docker-run migrate swagger help

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
BINARY_NAME=auth-api
BINARY_UNIX=$(BINARY_NAME)_unix

# Docker parameters
DOCKER_IMAGE=go-auth-api
DOCKER_TAG=latest

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the application
	$(GOBUILD) -o $(BINARY_NAME) -v ./cmd/api

run: ## Run the application
	$(GOCMD) run ./cmd/api/main.go

test: ## Run tests
	$(GOTEST) -v -race -coverprofile=coverage.out ./...

test-coverage: test ## Run tests and show coverage
	$(GOCMD) tool cover -html=coverage.out

clean: ## Clean build files
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_UNIX)
	rm -f coverage.out

deps: ## Download dependencies
	$(GOMOD) download
	$(GOMOD) tidy

swagger: ## Generate swagger documentation
	swag init -g cmd/api/main.go

lint: ## Run linter
	golangci-lint run

# Docker targets
docker-build: ## Build Docker image
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

docker-run: ## Run Docker container
	docker-compose up -d

docker-stop: ## Stop Docker container
	docker-compose down

docker-logs: ## Show Docker logs
	docker-compose logs -f

# Database targets
migrate-up: ## Run database migrations
	@echo "Running migrations..."
	$(GOCMD) run ./cmd/migrate/main.go up

migrate-down: ## Rollback database migrations
	@echo "Rolling back migrations..."
	$(GOCMD) run ./cmd/migrate/main.go down

# Development targets
dev: ## Start development environment
	docker-compose up -d postgres
	@echo "Waiting for PostgreSQL to be ready..."
	@sleep 5
	make migrate-up
	make run

# Production targets
build-linux: ## Build for Linux
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_UNIX) -v ./cmd/api

deploy: build-linux docker-build ## Build and deploy