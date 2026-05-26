# Project Progress: bot-x

## Current Status

The project is in a stable, compilable state. The monorepo contains 10 microservices that communicate over gRPC, share generated protobuf code in `gen/go`, and rely on shared libraries under `shared`.

Recently completed changes improve local developer experience and production safety:

- Centralized TLS-aware gRPC dial helper added in `shared/grpcdial/dial.go` and wired into service entrypoints to avoid using insecure credentials in production.
- Local infrastructure in `docker-compose.yml` updated to use Postgres:15 and Redis:7 for Postgres-compatible code paths and Asynq/Redis-based background processing. ClickHouse and Vault remain in the compose stack.
- Database migration files are present in `/migrations` (Postgres + ClickHouse migrations). These provide the base schema used by service queries.
- Quick unit tests were added: giveaway state machine tests and an X webhook CRC handler test.
- A basic GitHub Actions CI workflow was added to run `go test` and `golangci-lint` on push/PR.

- [Phase 1 applied] Security & compliance hardening: enforced production webhook signature verification, switched runtime secret reads to Vault where available, and upgraded OFAC screening to use fuzzy matching (Levenshtein) in the shared screener.

## What is Completed

- [x] Defined all Protobuf schemas in `/proto`.
- [x] Fixed compilation errors and stabilized the Go workspace (`go.work`).
- [x] Multi-stage Dockerfiles exist for microservices.
- [x] Workspace builds successfully (`go build ./...`).
- [x] **Implemented Core Services** (same service list as before).
- [x] Added `shared/grpcdial/dial.go` and updated service dialers to prefer TLS in production.
- [x] Replaced CockroachDB/Dragonfly test infra with Postgres and Redis in `docker-compose.yml` for local development.
- [x] Added SQL migrations in `/migrations` (Postgres + ClickHouse) used by services.
- [x] Added unit tests for the giveaway state machine and a CRC test for the X webhook handler.
- [x] Added a basic GitHub Actions workflow at `.github/workflows/ci.yml` that runs `go test` and `golangci-lint`.

## Missing Steps / Next Actions

- [ ] **Migrations automation**: Wire `/migrations` into local startup (Makefile or a compose init job) so Postgres is seeded automatically when spinning up `docker-compose`.
- [ ] **Migrations automation**: Wire `/migrations` into local startup (Makefile or a compose init job) so Postgres is seeded automatically when spinning up `docker-compose`.
- [Phase 2 started] Database schema updated for monetary storage: initial migration changed monetary column types to `BIGINT` for fresh installs and a conversion migration `000003_migrate_amounts_to_bigint.up.sql` was added to convert existing `NUMERIC(12,2)` values to integer lowest-denomination values (multiplies by 100).

## Phase 2: Monetary storage migration (in-progress → mostly complete)

Status: major code + proto changes applied, generated code regenerated, repo sweep completed.

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

Next actions remaining for Phase 2:

- Automate migrations as part of startup/deploy manifests (Makefile / init container).
- Run integration tests against a staging environment seeded with converted data.
- Update deployment docs and CI workflows to include `make proto` and migration checks.
- [ ] **End-to-end integration tests**: Create integration tests that exercise webhooks → `xgateway` → DB → Asynq worker flows.
- [ ] **Webhook registration automation & docs**: Add scripts/docs to register the X/Twitter webhook (CRC flow) and instructions for producing `X_CONSUMER_SECRET` and `X_BEARER_TOKEN` values.
- [ ] **Expand CI/CD**: Extend the CI workflow to build Docker images, run database migrations in CI test jobs, and add releases/builds for staging/prod deploys.
- [ ] **Production validation**: Ensure `APP_ENV`, `GRPC_TLS_CA_FILE`, and `GRPC_TLS_SERVER_NAME` are set and validated in staging/prod; add tests for TLS handshakes if possible.
- [ ] **More unit tests**: Flesh out state machine tests, worker tests, and gateway integration tests beyond the initial test coverage added.

Recent actions:

- Completed Phase 1 (Security & Compliance Hardening):
  - Enforced strict webhook signature verification in `services/xgateway/internal/handler/webhook.go` for production environments.
  - `shared/config/config.go` now attempts to fetch sensitive secrets from Vault (`secret/x`, `secret/safehaven`) when `VAULT_*` vars are present; startup will fail in production if Vault is required but unavailable.
  - `shared/ofac/screener.go` now uses a Levenshtein fuzzy-match path as a secondary check to reduce false negatives from strict substring checks.
