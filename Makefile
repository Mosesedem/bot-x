# ============================================================
#  instantf-bot-x — Makefile
# ============================================================
.PHONY: dev proto migrate test lint build clean

MODULE_PREFIX := github.com/mosesedem/bot-x

# ── Local Dev ─────────────────────────────────────────────
dev:
	docker compose up --build

dev-infra:
	docker compose up cockroachdb dragonfly clickhouse vault --build -d

# ── Proto / gRPC ──────────────────────────────────────────
proto:
	cd proto && buf generate

proto-lint:
	cd proto && buf lint

# ── Database Migrations ───────────────────────────────────
migrate-up:
	migrate -path ./migrations -database "$$DATABASE_URL" up

migrate-down:
	migrate -path ./migrations -database "$$DATABASE_URL" down 1

migrate-create:
	migrate create -ext sql -dir ./migrations -seq $(name)

# ── Testing ───────────────────────────────────────────────
test:
	go test ./...

test-race:
	go test -race ./...

test-cover:
	go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# ── Code Generation ───────────────────────────────────────
sqlc:
	sqlc generate

generate:
	go generate ./...

# ── Linting ───────────────────────────────────────────────
lint:
	golangci-lint run ./...

# ── Build (all services) ─────────────────────────────────
build:
	@for svc in xgateway giveaway entry payment-router kyc compliance audit notification reconciliation admin; do \
		echo "Building $$svc..."; \
		go build -o bin/$$svc ./services/$$svc/cmd/main.go 2>/dev1/null || \
		go build -o bin/$$svc ./services/$$svc/cmd/; \
	done

# ── Clean ─────────────────────────────────────────────────
clean:
	rm -rf bin/ coverage.out

# ── Vault Dev Setup ───────────────────────────────────────
vault-setup:
	@echo "Seeding Vault dev with dummy secrets..."
	vault kv put secret/safehaven client_id=dev_client_id client_secret=dev_secret
	vault kv put secret/stripe secret_key=sk_test_dummy
	vault kv put secret/x consumer_key=dev_key consumer_secret=dev_secret

# ── OFAC Cache Seed ───────────────────────────────────────
ofac-refresh:
	curl -s https://www.treasury.gov/ofac/downloads/sdn.xml -o ./shared/ofac/testdata/sdn.xml
	@echo "OFAC SDN list downloaded."
