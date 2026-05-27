# Project Progress: bot-x

## Current Status

**Alpha/Pre-Production** — Core architecture complete, critical integrations remain mocked.

The monorepo contains 10 microservices communicating over gRPC. The project compiles and runs locally via Docker Compose.

### Recently Completed (July 2025)

#### Deployment Infrastructure ✅
- **NEW:** `docker-compose.prod.yml` — Production-ready compose for Digital Ocean Droplet
- **NEW:** `nginx/bot-x.conf` — Nginx SSL reverse proxy configuration
- **NEW:** `scripts/deploy.sh` — Automated deployment script
- **NEW:** `.env.production.example` — Production environment template
- **NEW:** `docs/DEPLOY_DO_DROPLET.md` — Complete deployment guide
- **UPDATED:** `Makefile` with `deploy-prod`, `prod-*` targets for DO management

#### Previously Completed
- TLS-aware gRPC dial helper (`shared/grpcdial/dial.go`)
- Local dev with Postgres 15, Redis 7, ClickHouse in compose
- Database migrations for PostgreSQL
- Basic unit tests (giveaway state machine, X webhook CRC)
- GitHub Actions CI (test + lint)
- Phase 1: Security hardening (webhook verification, Vault integration, OFAC fuzzy matching)
- Phase 2: Monetary storage migration (int64 cents throughout)

## What is Completed

### Core Architecture ✅
- [x] 10 microservices with multi-stage Dockerfiles
- [x] Protobuf schemas in `/proto`, generated code in `gen/go`
- [x] gRPC communication with TLS support (`shared/grpcdial/dial.go`)
- [x] Go workspace stable (`go.work`)
- [x] Database migrations (PostgreSQL + ClickHouse)
- [x] Background job processing (Asynq/Redis)

### Security & Compliance ✅
- [x] Webhook signature verification (production-enforced)
- [x] HashiCorp Vault integration for secrets
- [x] OFAC screening with Levenshtein fuzzy matching
- [x] `.env.example` scrubbed of secrets

### Data Layer ✅
- [x] Phase 2: All monetary fields as `int64` cents
- [x] PostgreSQL schema with `BIGINT` for money
- [x] Conversion migration for existing data
- [x] ClickHouse for audit logging

### Testing & CI ✅
- [x] Basic unit tests (giveaway state machine, webhook CRC)
- [x] GitHub Actions CI (test + lint)

### Deployment Infrastructure ✅
- [x] `docker-compose.prod.yml` for DO Droplet
- [x] Nginx SSL configuration
- [x] Automated deployment script (`scripts/deploy.sh`)
- [x] Production environment template
- [x] Complete deployment documentation

## Missing Steps / Production Blockers

### 🔴 Critical (Must Fix Before Launch)

| # | Item | Location | Impact |
|---|------|----------|--------|
| 1 | **Twitter API Integration** | `xgateway/grpc_handler.go` | Bot cannot send DMs or reply to tweets |
| 2 | **Reconciliation Worker** | `reconciliation/worker/reconcile.go` | Orphaned transactions possible |
| 3 | **End-to-end Tests** | `test/` | No integration test coverage for webhook→payout flow |

### 🟡 Important (Fix Before Scale)

| # | Item | Location | Impact |
|---|------|----------|--------|
| 4 | **Crypto Gateway** | `payment-router/crypto/` | Cannot process crypto payouts |
| 5 | **International KYC** | `kyc/` | Non-NG users bypass compliance |
| 6 | **CI Docker Builds** | `.github/workflows/ci.yml` | No automated image builds |
| 7 | **Webhook Idempotency** | `xgateway/webhook.go` | Duplicate webhook processing risk |

### 🟢 Nice to Have

- [ ] Circuit breakers for gRPC calls
- [ ] Structured logging with correlation IDs
- [ ] Prometheus metrics endpoint
- [ ] PagerDuty/Slack alerting integration

## Phase 2: Monetary Storage Migration ✅ COMPLETE

Status: **COMPLETE** — All monetary fields use `int64` cents at DB and RPC boundaries.

- Database migrations use `BIGINT` for fresh installs
- Conversion migration exists for existing data
- Protobufs converted to `int64` amounts
- Repository-wide sweep completed

Summary of work completed in Phase 2:

- Updated database migrations for fresh installs to use `BIGINT` for monetary columns (e.g., `total_budget`, `amount_per_winner`, `giveaway_winners.amount`).
- Added conversion migration `migrations/000003_migrate_amounts_to_bigint.up.sql` to migrate existing `NUMERIC(12,2)` values to lowest-denomination integers (multiply by 100).
- Converted Protobuf definitions to use `int64` for monetary fields in `/proto/*/v1/*.proto` and regenerated Go code via `make proto` (updated `gen/go`).
- Swept service code and gateway client structs to use `int64` cents at the DB and RPC boundaries. HTTP endpoints that are public-facing still emit major-unit floats for human-friendly consumption.
- Updated gateways and Safe Haven / Paystack / Flutterwave client structs to encode/decode amounts as integer cents where the external API expects cents. Where external APIs expect floats, adapter code is in place to convert as needed.

Migration checklist (suggested order for staging/production rollout):

1. Backup your database. Example:

```bash
pg_dump "$DATABASE_URL" > backup-before-amount-migration.sql
```

2. Deploy server code that understands both numeric and integer DB values (compat layer) — optional but safer for blue/green.
3. Run `migrations/000003_migrate_amounts_to_bigint.up.sql` in staging and validate amounts (spot-check totals and a few rows).
4. Deploy services with the new protobufs (ensure `gen/go` is up-to-date) and run smoke tests: create giveaway → fund escrow → draw winners → payout.
5. Run reconciliation checks and verify `TotalDisbursed` sums match expected values (be mindful of rounding rules applied during conversion).
6. After successful staging validation, schedule a maintenance window for production, repeat steps 1–5.

Notes & risks:

- The conversion multiplies stored decimals by 100 and rounds half-up. This may change some historical totals if previous data used fractional sub-cent values — validate before applying to production.
- External gateway contracts must be reviewed: some gateways accept amounts in cents (preferred), others expect floats. Adapters have been added in `shared/gateways/*` but please verify with each provider.
- We updated `shared/nlp/commandparser` to return integer cents to reduce downstream conversion errors.

## Deployment Strategy: Digital Ocean Droplet ✅

**Selected Approach:** Docker Compose on Ubuntu Droplet (not App Platform)

**Rationale:**
- All-in-one deployment simplifies initial launch
- Containerized Postgres/Redis/ClickHouse on single droplet
- Easy to migrate to managed services later
- Nginx reverse proxy with SSL termination
- Single-command deployment via `make deploy-prod`

**Files Created:**
- `docker-compose.prod.yml` — 10 services + 3 data stores + migrations
- `nginx/bot-x.conf` — SSL reverse proxy with rate limiting
- `scripts/deploy.sh` — Automated deployment with validation
- `docs/DEPLOY_DO_DROPLET.md` — Complete step-by-step guide

**Next Actions:**
- [ ] Test deployment on staging droplet
- [ ] Set up production droplet
- [ ] Configure SSL certificates
- [ ] Register Twitter webhook
- [ ] Run smoke tests

---

## Upcoming Phase 3: Production Hardening

**After Twitter API integration is complete:**

1. End-to-end integration tests (testcontainers-go)
2. CI/CD pipeline for Docker builds
3. Monitoring (Prometheus/Grafana)
4. Alerting (PagerDuty/Slack)
5. Load testing
6. Documentation for operators

---

## Recent Actions Log

**July 2025:**
- ✅ Created production deployment infrastructure for DO Droplet
- ✅ Updated critic.md with accurate project state
- ✅ Created comprehensive deployment guide

**June 2025:**
- ✅ Completed Phase 2 (monetary storage migration)
- ✅ Security hardening (Phase 1)

**May 2025:**
- ✅ Fixed compilation errors, stabilized workspace
- ✅ Added basic unit tests
- ✅ Created CI workflow

Recent actions:

- Completed Phase 1 (Security & Compliance Hardening):
  - Enforced strict webhook signature verification in `services/xgateway/internal/handler/webhook.go` for production environments.
  - `shared/config/config.go` now attempts to fetch sensitive secrets from Vault (`secret/x`, `secret/safehaven`) when `VAULT_*` vars are present; startup will fail in production if Vault is required but unavailable.
  - `shared/ofac/screener.go` now uses a Levenshtein fuzzy-match path as a secondary check to reduce false negatives from strict substring checks.
