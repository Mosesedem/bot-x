# ============================================================
#  bot-x — Makefile
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
		go build -o bin/$$svc ./services/$$svc/cmd/main.go 2>/dev/null || \
		go build -o bin/$$svc ./services/$$svc/cmd/; \
	done

# ── Digital Ocean Droplet Deployment ─────────────────────

deploy-prod:
	./scripts/deploy.sh production

deploy-staging:
	./scripts/deploy.sh staging

# ── Production Infrastructure Management ─────────────────

prod-up:
	docker compose up -d

prod-down:
	docker compose down

prod-logs:
	docker compose logs -f

prod-ps:
	docker compose ps

prod-migrate:
	docker compose run --rm migrate

prod-backup:
	@echo "Creating database backup..."
	docker compose exec postgres pg_dump -U botx instantf_bot_x > backup-$$(date +%Y%m%d-%H%M%S).sql

# ── Heroku (Deprecated - use DO Droplet instead) ─────────

heroku-deploy:
	@echo "WARNING: Heroku deployment is deprecated. Use 'make deploy-prod' for Digital Ocean."
	@if [ -z "$(app)" ] || [ -z "$(service)" ]; then \
		echo "Usage: make heroku-deploy app=<heroku-app> service=<service-name>"; \
		exit 1; \
	fi
	heroku container:push web -a $(app) -f services/$(service)/Dockerfile .
	heroku container:release web -a $(app)

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
