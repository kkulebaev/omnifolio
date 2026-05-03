.PHONY: help install services services-down logs web dev test build clean

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

test: ## Run tests (api + web)
	$(COMPOSE) run --rm api go test ./...
	pnpm --filter web test --run

build: ## Build production artifacts
	$(COMPOSE) run --rm api go build -o bin/api ./cmd/api
	pnpm --filter web build

clean: ## Remove build artifacts
	rm -rf apps/api/bin apps/api/tmp apps/web/dist apps/web/node_modules/.vite
