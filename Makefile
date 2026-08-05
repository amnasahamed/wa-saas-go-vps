.PHONY: help build test lint run-api run-bridge migrate-up migrate-down migrate-status sqlc dev up down logs clean \
	prod-build prod-up prod-down prod-restart prod-logs prod-ps prod-migrate prod-migrate-status prod-psql prod-backup

# ---- help ----
help: ## Show this help.
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

# ---- build ----
build: ## Build both binaries into ./bin
	mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/api    ./cmd/api
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/bridge ./cmd/bridge
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/migrate ./cmd/migrate

build-api: ## Build cmd/api only
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/api ./cmd/api

build-bridge: ## Build cmd/bridge only
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/bridge ./cmd/bridge

# ---- test ----
test: ## Run all tests
	go test ./... -count=1

test-race: ## Run tests with race detector
	go test ./... -race -count=1

# ---- lint ----
lint: ## Run golangci-lint (install separately)
	golangci-lint run ./...

fmt: ## Format all Go code
	gofmt -s -w .

vet: ## go vet
	go vet ./...

# ---- run ----
run-api: ## Run cmd/api from source
	ENV=dev LOG_LEVEL=debug \
	  DATABASE_URL=postgres://postgres:postgres@localhost:5432/niosengine?sslmode=disable \
	  REDIS_URL=redis://localhost:6379/0 \
	  go run ./cmd/api

run-bridge: ## Run cmd/bridge from source
	ENV=dev LOG_LEVEL=debug \
	  DATABASE_URL=postgres://postgres:postgres@localhost:5432/niosengine?sslmode=disable \
	  REDIS_URL=redis://localhost:6379/0 \
	  BRIDGE_WORKER_ID=local-dev \
	  go run ./cmd/bridge

# ---- migrations ----
migrate-up: ## Apply all pending migrations
	go run ./cmd/migrate up

migrate-down: ## Roll back the most recent migration
	go run ./cmd/migrate down

migrate-status: ## Show migration status
	go run ./cmd/migrate status

migrate-create: ## Create a new migration (usage: make migrate-create NAME=add_foo)
	go run ./cmd/migrate create $(NAME) sql

# ---- sqlc ----
sqlc: ## Regenerate type-safe Go from db/queries/*.sql
	sqlc generate

# ---- dev ----
up: ## Bring up local dev stack (postgres, redis, localstack, api, bridge)
	docker compose -f docker-compose.dev.yml up -d --build

down: ## Stop the local dev stack
	docker compose -f docker-compose.dev.yml down

logs: ## Tail logs from api + bridge
	docker compose -f docker-compose.dev.yml logs -f api bridge

dev-up: ## up + run migrations
	$(MAKE) up
	$(MAKE) migrate-up

# ---- production (single VPS) ----
COMPOSE_PROD := docker compose -f docker-compose.prod.yml --env-file .env.prod

prod-build: ## Build the production images
	$(COMPOSE_PROD) build

prod-up: ## Build and start the full production stack
	$(COMPOSE_PROD) up -d --build
	@echo
	@echo "Stack starting. Watch cert issuance + boot with: make prod-logs"

prod-down: ## Stop the production stack (volumes are preserved)
	$(COMPOSE_PROD) down

prod-restart: ## Restart api, bridge, and web without touching the datastores
	$(COMPOSE_PROD) restart api bridge web

prod-logs: ## Tail logs from all production services
	$(COMPOSE_PROD) logs -f --tail=100

prod-ps: ## Show production container status
	$(COMPOSE_PROD) ps

prod-migrate: ## Run pending migrations against the production database
	$(COMPOSE_PROD) run --rm migrate up

prod-migrate-status: ## Show production migration status
	$(COMPOSE_PROD) run --rm migrate status

prod-psql: ## Open a psql shell on the production database
	$(COMPOSE_PROD) exec postgres psql -U $${POSTGRES_USER:-niosengine} -d $${POSTGRES_DB:-niosengine}

prod-backup: ## Run the backup script now (Postgres + bridge device state)
	PROJECT_DIR=$(CURDIR) bash deploy/vps/backup.sh

# ---- clean ----
clean: ## Remove built binaries and dev artifacts
	rm -rf bin/
	go clean -cache -testcache
