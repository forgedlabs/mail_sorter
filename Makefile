# Email Validator - Makefile

.PHONY: help dev build up down logs test clean

# Default target
help:
	@echo "Email Validator - Available Commands:"
	@echo ""
	@echo "  make dev          - Start development environment (Docker Compose)"
	@echo "  make build        - Build Go service Docker image"
	@echo "  make up           - Start all services"
	@echo "  make down         - Stop all services"
	@echo "  make logs         - View logs"
	@echo "  make test         - Run tests"
	@echo "  make clean        - Clean up containers and volumes"
	@echo "  make init-db      - Initialize database schema"
	@echo "  make go-deps      - Install Go dependencies"
	@echo ""

# Development
dev: go-deps
	docker-compose up --build

# Build Go service
build:
	docker-compose build smtp-verifier

# Start services
up:
	docker-compose up -d

# Stop services
down:
	docker-compose down

# View logs
logs:
	docker-compose logs -f

# Test the service
test:
	@echo "Testing email validation..."
	@curl -X POST http://localhost:8080/v1/validate \
		-H "Content-Type: application/json" \
		-d '{"email": "test@gmail.com"}' | jq .

# Clean everything
clean:
	docker-compose down -v
	rm -rf services/verifier/vendor

# Initialize database
init-db:
	docker-compose exec postgres psql -U email_validator_app -d email_validation -f /docker-entrypoint-initdb.d/01-schema.sql

# Install Go dependencies
go-deps:
	cd services/verifier && go mod download

# Run Go service locally (without Docker)
run-local:
	cd services/verifier && \
		DATABASE_HOST=localhost \
		REDIS_HOST=localhost \
		go run .

# Watch logs for specific service
logs-verifier:
	docker-compose logs -f smtp-verifier

logs-postgres:
	docker-compose logs -f postgres

logs-redis:
	docker-compose logs -f redis

# Database operations
db-shell:
	docker-compose exec postgres psql -U email_validator_app -d email_validation

redis-cli:
	docker-compose exec redis redis-cli

# Check service health
health:
	@echo "Checking service health..."
	@curl http://localhost:8080/health | jq .

# Metrics
metrics:
	@curl http://localhost:9090/metrics

# Production Kubernetes deployment
k8s-deploy:
	kubectl apply -f deploy/kubernetes.yaml

k8s-delete:
	kubectl delete -f deploy/kubernetes.yaml

# Development with debug tools (Redis Commander, pgAdmin)
dev-debug:
	docker-compose --profile debug up --build
