.DEFAULT_GOAL := help
GO_VERSION := go1.26.8

.PHONY: build check-go

check-go:
	@test "$$(go env GOVERSION)" = "$(GO_VERSION)" || { echo "requires $(GO_VERSION), found $$(go env GOVERSION)" >&2; exit 1; }

build: check-go
	go build -buildvcs=false -o bin/perpetual ./cmd/perpetual
	go build -buildvcs=false -o bin/agent-plane ./cmd/agent-plane

include scripts/dev.mk

.PHONY: help test-fast test-sim test-integration test-fixture-cleanup test test-race vet coverage test-fuzz

help:
	@echo 'build             Build agent-plane and perpetual into bin/'
	@echo 'test-fast         Unit, local HTTP, subprocess contracts and fuzz seeds (no PostgreSQL)'
	@echo 'test-sim          Fixed scenarios, replay and invariant checks (no external services)'
	@echo 'test-integration  Owned disposable Docker PostgreSQL and real service/CLI checks'
	@echo 'test-fixture-cleanup  Verify owned containers/volumes are removed on success and failure'
	@echo 'test              Complete fast, simulation and integration suites'
	@echo 'test-race         Complete suite under Go race detector with real PostgreSQL'
	@echo 'vet               Static diagnostics on production and tests'
	@echo 'coverage          Full-suite statement profile plus reviewed decision inventory'
	@echo 'test-fuzz         Four named native fuzz targets, bounded 30s discovery each'
	@echo 'dev-db-up         Start the persistent local Docker PostgreSQL development fixture'
	@echo 'dev-db-down       Stop the development fixture'
	@echo 'dev-db-env        Show development connection settings'
	@echo 'dev-db-reset      Reset the owned development database'

test-fast:
	go test ./... -skip '^TestSimulation' -count=1 -timeout=60s

test-sim:
	go test ./internal/registration -run '^TestSimulation|^TestTrace' -count=1 -timeout=30s

test-fixture-cleanup:
	scripts/test-postgres-cleanup.sh

test-integration: test-fixture-cleanup
	scripts/test-postgres.sh go test -p 1 -tags=integration ./... -run '^(TestP[0-9]|TestProcess|TestFixture)' -count=1 -timeout=180s

test: test-fast test-sim test-integration

test-race:
	scripts/test-postgres.sh go test -p 1 -race -tags=integration ./... -count=1 -timeout=240s

vet:
	go vet -tags=integration ./...

coverage:
	mkdir -p artifacts
	scripts/test-postgres.sh go test -p 1 -tags=integration ./... -coverprofile=artifacts/statements.out -count=1 -timeout=180s
	go tool cover -func=artifacts/statements.out
	@echo 'The profile measures statements, not MC/DC. Reviewed conditions and gaps:'
	@cat docs/testing/decisions.md

test-fuzz:
	timeout 120s go test ./internal/registration -run '^$$' -fuzz '^FuzzIdentifiers$$' -fuzztime=30s -fuzzminimizetime=5s -parallel=2 -timeout=60s
	timeout 120s go test ./internal/registration -run '^$$' -fuzz '^FuzzCanonicalization$$' -fuzztime=30s -fuzzminimizetime=5s -parallel=2 -timeout=60s
	timeout 120s go test ./internal/httpapi -run '^$$' -fuzz '^FuzzDecodeRegistration$$' -fuzztime=30s -fuzzminimizetime=5s -parallel=2 -timeout=60s

	timeout 120s go test ./internal/registration -run '^$$' -fuzz '^FuzzPureCore$$' -fuzztime=30s -fuzzminimizetime=5s -parallel=2 -timeout=60s
