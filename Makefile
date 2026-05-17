.PHONY: help install services services-down logs web dev dev-seeded seed seed-reset test build clean generate

COMPOSE ?= docker compose

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS=":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

install: ## Install JS deps
	pnpm install

services: ## Start postgres + api in background
	$(COMPOSE) up -d --build

services-down: ## Stop services
	$(COMPOSE) down

logs: ## Tail service logs
	$(COMPOSE) logs -f

web: ## Run web dev server in foreground
	pnpm --filter web dev

dev: services ## Start services (detached) and web (foreground)
	@echo "API:      http://localhost:8080"
	@echo "Web:      http://localhost:5173"
	@echo "Postgres: localhost:5432"
	pnpm --filter web dev

dev-seeded: services seed ## Start services, apply dev seed, then web (foreground)
	@echo "API:      http://localhost:8080"
	@echo "Web:      http://localhost:5173"
	@echo "Postgres: localhost:5432 (seeded)"
	pnpm --filter web dev

seed: ## Apply dev seed for the bootstrap user (waits for api bootstrap)
	@echo "Waiting for postgres..."
	@$(COMPOSE) exec -T postgres sh -c 'until pg_isready -U omnifolio -d omnifolio >/dev/null 2>&1; do sleep 0.5; done'
	@echo "Waiting for bootstrap user dev@local.test..."
	@$(COMPOSE) exec -T postgres sh -c \
		'until psql -U omnifolio -d omnifolio -tA -c "SELECT 1 FROM users WHERE email='"'"'dev@local.test'"'"'" 2>/dev/null | grep -q 1; do sleep 0.5; done'
	$(COMPOSE) exec -T postgres psql -U omnifolio -d omnifolio -v ON_ERROR_STOP=1 \
		< apps/api/internal/storage/seed/dev_seed.sql
	@echo "Dev seed applied."

seed-reset: ## Wipe dev seed data for the bootstrap user
	$(COMPOSE) exec -T postgres psql -U omnifolio -d omnifolio -v ON_ERROR_STOP=1 -c "\
		DELETE FROM accounts WHERE user_id = (SELECT id FROM users WHERE email = 'dev@local.test'); \
		DELETE FROM portfolio_snapshots WHERE user_id = (SELECT id FROM users WHERE email = 'dev@local.test'); \
		DELETE FROM deposits WHERE user_id = (SELECT id FROM users WHERE email = 'dev@local.test'); \
		DELETE FROM instruments WHERE ticker IN ('SBER','LKOH','AAPL','BTC','ETH','USD');"

generate: ## Regenerate sqlc + oapi-codegen + orval clients
	$(COMPOSE) run --rm --no-deps api sh -c "sqlc generate && cd internal/server/oapi && go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.7.0 --config=config.yaml /workspace/api/openapi.yaml"
	pnpm --filter web generate

test: ## Run tests (api + web)
	$(COMPOSE) run --rm api go test ./...
	pnpm --filter web test --run

build: ## Build production artifacts
	$(COMPOSE) run --rm api go build -o bin/api ./cmd/api
	pnpm --filter web build

clean: ## Remove build artifacts
	rm -rf apps/api/bin apps/api/tmp apps/web/dist apps/web/node_modules/.vite
