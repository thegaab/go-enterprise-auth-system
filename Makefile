.PHONY: build run test clean docker-build docker-run migrate swagger help deps lint security-scan benchmark load-test

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
DOCKER_REGISTRY=your-registry.com

# Kubernetes parameters
K8S_NAMESPACE=production

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Development targets
deps: ## Download dependencies
	$(GOMOD) download
	$(GOMOD) tidy
	$(GOMOD) verify

build: ## Build the application
	CGO_ENABLED=0 GOOS=linux $(GOBUILD) -a -installsuffix cgo -ldflags '-extldflags "-static"' -o $(BINARY_NAME) ./cmd/api

run: ## Run the application locally
	$(GOCMD) run ./cmd/api/main.go

dev: ## Start full development environment
	@echo "Starting development environment..."
	docker-compose -f docker-compose.observability.yml up -d postgres redis prometheus grafana jaeger
	@echo "Waiting for services to be ready..."
	@sleep 10
	@echo "Running database migrations..."
	$(MAKE) migrate-up
	@echo "Starting API server..."
	$(MAKE) run

# Testing targets
test: ## Run unit tests
	$(GOTEST) -v -race -coverprofile=coverage.out ./...

test-integration: ## Run integration tests
	$(GOTEST) -v -tags=integration ./tests/integration/...

test-e2e: ## Run end-to-end tests
	$(GOTEST) -v -tags=e2e ./tests/e2e/...

test-all: test test-integration test-e2e ## Run all tests

test-coverage: test ## Generate and view test coverage
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

benchmark: ## Run benchmark tests
	$(GOTEST) -bench=. -benchmem -cpuprofile=cpu.prof -memprofile=mem.prof ./...

# Load testing
load-test: ## Run load tests
	@echo "Starting load test..."
	docker-compose -f docker-compose.observability.yml --profile load-test up k6

load-test-local: ## Run load test against local instance
	$(GOCMD) run ./tests/load/main.go

# Code quality
lint: ## Run linter
	golangci-lint run --timeout=5m

security-scan: ## Run security scan
	gosec ./...

format: ## Format code
	gofmt -s -w .
	goimports -w .

vet: ## Run go vet
	$(GOCMD) vet ./...

# Documentation
swagger: ## Generate swagger documentation
	swag init -g cmd/api/main.go

docs-serve: ## Serve documentation locally
	@echo "Starting documentation server at http://localhost:6060"
	godoc -http=:6060

# Database operations
migrate-up: ## Run database migrations
	@echo "Running database migrations..."
	$(GOCMD) run ./cmd/migrate/main.go up

migrate-down: ## Rollback database migrations
	@echo "Rolling back database migrations..."
	$(GOCMD) run ./cmd/migrate/main.go down

migrate-create: ## Create new migration file
	@read -p "Migration name: " name; \
	$(GOCMD) run ./cmd/migrate/main.go create $$name

db-reset: ## Reset database
	$(MAKE) migrate-down
	$(MAKE) migrate-up

# Docker operations
docker-build: ## Build Docker image
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

docker-build-multi: ## Build multi-platform Docker image
	docker buildx build --platform linux/amd64,linux/arm64 -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

docker-push: docker-build ## Push Docker image to registry
	docker tag $(DOCKER_IMAGE):$(DOCKER_TAG) $(DOCKER_REGISTRY)/$(DOCKER_IMAGE):$(DOCKER_TAG)
	docker push $(DOCKER_REGISTRY)/$(DOCKER_IMAGE):$(DOCKER_TAG)

docker-run: ## Run Docker container locally
	docker-compose up -d

docker-logs: ## Show Docker logs
	docker-compose logs -f go-api

docker-stop: ## Stop Docker containers
	docker-compose down

docker-clean: ## Clean Docker images and containers
	docker-compose down -v --rmi all
	docker system prune -f

# Observability stack
observability-up: ## Start full observability stack
	docker-compose -f docker-compose.observability.yml up -d

observability-down: ## Stop observability stack
	docker-compose -f docker-compose.observability.yml down

observability-logs: ## Show observability stack logs
	docker-compose -f docker-compose.observability.yml logs -f

# Kubernetes operations
k8s-deploy: ## Deploy to Kubernetes
	kubectl apply -f deployments/k8s/ -n $(K8S_NAMESPACE)

k8s-delete: ## Delete from Kubernetes
	kubectl delete -f deployments/k8s/ -n $(K8S_NAMESPACE)

k8s-status: ## Check Kubernetes deployment status
	kubectl get pods,svc,hpa -n $(K8S_NAMESPACE) -l app=go-auth-api

k8s-logs: ## Show Kubernetes logs
	kubectl logs -f -n $(K8S_NAMESPACE) -l app=go-auth-api

k8s-scale: ## Scale Kubernetes deployment
	@read -p "Number of replicas: " replicas; \
	kubectl scale deployment go-auth-api --replicas=$$replicas -n $(K8S_NAMESPACE)

k8s-rollback: ## Rollback Kubernetes deployment
	kubectl rollout undo deployment/go-auth-api -n $(K8S_NAMESPACE)

# Performance profiling
profile-cpu: ## Profile CPU usage
	$(GOCMD) test -cpuprofile=cpu.prof -bench=. ./...
	$(GOCMD) tool pprof cpu.prof

profile-memory: ## Profile memory usage
	$(GOCMD) test -memprofile=mem.prof -bench=. ./...
	$(GOCMD) tool pprof mem.prof

profile-block: ## Profile blocking operations
	$(GOCMD) test -blockprofile=block.prof -bench=. ./...
	$(GOCMD) tool pprof block.prof

# CI/CD helpers
pre-commit: lint vet test security-scan ## Run pre-commit checks
	@echo "✅ All pre-commit checks passed!"

ci-build: deps build test lint security-scan ## Full CI build pipeline
	@echo "✅ CI build completed successfully!"

cd-deploy: docker-build docker-push k8s-deploy ## Full CD deployment pipeline
	@echo "✅ CD deployment completed successfully!"

# Monitoring and debugging
health-check: ## Check application health
	@curl -s http://localhost:8081/health | jq .

metrics: ## View Prometheus metrics
	@curl -s http://localhost:8081/metrics

trace-list: ## List Jaeger traces
	@curl -s "http://localhost:16686/api/traces?service=go-auth-api&limit=10" | jq .

# Development utilities
install-tools: ## Install development tools
	$(GOCMD) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	$(GOCMD) install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest
	$(GOCMD) install github.com/swaggo/swag/cmd/swag@latest
	$(GOCMD) install golang.org/x/tools/cmd/goimports@latest

clean: ## Clean build artifacts
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_UNIX)
	rm -f coverage.out coverage.html
	rm -f cpu.prof mem.prof block.prof
	rm -rf dist/

# Release management
version: ## Show current version
	@git describe --tags --always --dirty

tag: ## Create new git tag
	@read -p "Tag version (e.g., v1.0.0): " version; \
	git tag -a $$version -m "Release $$version"; \
	git push origin $$version

release: ci-build docker-build docker-push ## Create release
	@echo "✅ Release completed successfully!"

# Environment setup
env-dev: ## Setup development environment
	cp .env.example .env
	@echo "📝 Please edit .env file with your configuration"

env-prod: ## Setup production environment
	@echo "🚨 Setting up production environment..."
	@echo "Make sure to configure secrets properly!"

# Quick commands
up: docker-run ## Quick start with Docker
down: docker-stop ## Quick stop
restart: down up ## Quick restart
logs: docker-logs ## Quick logs view