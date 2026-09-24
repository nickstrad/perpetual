# Development data persists across down/up; integration tests use other containers.
DEV_PROJECT ?= perpetual-dev
PERPETUAL_DEV_PORT ?= 54329
export PERPETUAL_DEV_PORT
DEV_COMPOSE = docker compose --project-name $(DEV_PROJECT) --env-file scripts/versions.env -f compose.yaml

.PHONY: dev-db-up dev-db-down dev-db-env dev-db-status dev-db-logs dev-db-reset
dev-db-up:
	$(DEV_COMPOSE) up --detach --wait --wait-timeout 40
	@$(MAKE) --no-print-directory dev-db-env

dev-db-down:
	$(DEV_COMPOSE) down --timeout 10

dev-db-env:
	@echo 'export PERPETUAL_DATABASE_URL="postgres://perpetual:perpetual-local@127.0.0.1:$(PERPETUAL_DEV_PORT)/perpetual?sslmode=disable"'
	@echo 'export PERPETUAL_RUNTIME_DIR="$(CURDIR)/.runtime"'
	@echo 'export PERPETUAL_API_URL="http://127.0.0.1:7777"'

dev-db-status:
	$(DEV_COMPOSE) ps

dev-db-logs:
	$(DEV_COMPOSE) logs --tail 100 postgres

# Explicitly destructive: deletes only this Compose project's development data.
dev-db-reset:
	$(DEV_COMPOSE) down --volumes --timeout 10
