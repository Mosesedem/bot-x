# Project Progress: instantf-bot-x

## Current Status

The project is in a stable, compilable state. The monorepo contains 10 microservices that communicate over gRPC, share generated protobuf code in `gen/go`, and rely on shared libraries under `shared`.

Recently completed changes improve local developer experience and production safety:

- Centralized TLS-aware gRPC dial helper added in `shared/grpcdial/dial.go` and wired into service entrypoints to avoid using insecure credentials in production.
- Local infrastructure in `docker-compose.yml` updated to use Postgres:15 and Redis:7 for Postgres-compatible code paths and Asynq/Redis-based background processing. ClickHouse and Vault remain in the compose stack.
- Database migration files are present in `/migrations` (Postgres + ClickHouse migrations). These provide the base schema used by service queries.
- Quick unit tests were added: giveaway state machine tests and an X webhook CRC handler test.
- A basic GitHub Actions CI workflow was added to run `go test` and `golangci-lint` on push/PR.

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
- [ ] **End-to-end integration tests**: Create integration tests that exercise webhooks → `xgateway` → DB → Asynq worker flows.
- [ ] **Webhook registration automation & docs**: Add scripts/docs to register the X/Twitter webhook (CRC flow) and instructions for producing `X_CONSUMER_SECRET` and `X_BEARER_TOKEN` values.
- [ ] **Expand CI/CD**: Extend the CI workflow to build Docker images, run database migrations in CI test jobs, and add releases/builds for staging/prod deploys.
- [ ] **Production validation**: Ensure `APP_ENV`, `GRPC_TLS_CA_FILE`, and `GRPC_TLS_SERVER_NAME` are set and validated in staging/prod; add tests for TLS handshakes if possible.
- [ ] **More unit tests**: Flesh out state machine tests, worker tests, and gateway integration tests beyond the initial test coverage added.
