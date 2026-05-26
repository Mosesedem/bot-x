# Pre-Production Update Plan: InstantF Bot-X

This document outlines the actionable steps required to address the technical debt, architectural bottlenecks, and security vulnerabilities identified in the `critic.md` review. Completing this plan will ensure the system is stable, secure, and ready for production deployment.

## Phase 1: Security & Compliance Hardening (High Priority)
Security flaws must be addressed immediately to protect funds and user data.

- [ ] **Strict Webhook Signature Verification**
  - **Action**: Modify `services/xgateway/internal/handler/webhook.go`.
  - **Implementation**: Check the `APP_ENV` variable. If set to `production`, mandate the presence of the `X-Twitter-Webhooks-Signature` header and fail the request (HTTP 401) if validation fails. Remove the fail-open fallback.
- [ ] **Vault-Centric Secrets Management**
  - **Action**: Refactor `shared/config/config.go`.
  - **Implementation**: Instead of reading sensitive keys (e.g., `SAFEHAVEN_CLIENT_SECRET`, `X_CONSUMER_SECRET`) from `.env` via Viper, use the `vault` package to fetch these dynamically on application startup.
- [ ] **Enhanced OFAC Screening**
  - **Action**: Update `shared/ofac/screener.go`.
  - **Implementation**: Replace the simple `strings.Contains` logic with a fuzzy string matching algorithm (e.g., Levenshtein distance) or integrate a third-party compliance API for accurate sanctions screening.

## Phase 2: Data & Persistence Reliability
Ensure data integrity, especially concerning monetary values and schema consistency.

- [ ] **Refactor Monetary Storage**
  - **Action**: Update PostgreSQL schema (`migrations/000001_init_schema.up.sql`) and Go structs.
  - **Implementation**: Change `total_budget`, `amount_per_winner`, and `amount` fields from `NUMERIC(12, 2)` to `BIGINT`. Store values in their lowest denomination (e.g., kobo, cents). Update all formatting logic in the presentation/notification layer.
- [ ] **Automate Database Migrations**
  - **Action**: Update deployment manifests and `Makefile`.
  - **Implementation**: Add an init-container step in Kubernetes (or a release phase task in Heroku) that automatically runs the `migrate` CLI before any service starts accepting traffic.

## Phase 3: Code Quality & Core Logic Fixes
Address brittle code paths to prevent duplicate payouts and inconsistent states.

- [ ] **Robust Webhook Idempotency**
  - **Action**: Update `services/xgateway/internal/handler/webhook.go` and `mention_worker.go`.
  - **Implementation**: Cache the X webhook event IDs in Redis with a 24-hour TTL. Before processing any payload, check Redis; if the ID exists, drop the request to prevent duplicate processing.
- [ ] **State Reconciliation Worker**
  - **Action**: Add a background cron job in `services/payment-router/internal/worker`.
  - **Implementation**: Periodically query `giveaway_winners` for records stuck in `PROCESSING` for more than 5 minutes. Query the SafeHaven API for the actual status and update the database to either `SUCCESS` or `FAILED`.
- [ ] **Improve NLP Parsing**
  - **Action**: Refactor `shared/nlp/commandparser/parser.go`.
  - **Implementation**: Enforce stricter command syntax for the bot (e.g., `/giveaway <winners> <amount> <currency>`) to reduce reliance on brittle regex heuristics, or implement a fallback logic that asks the user to clarify if confidence is low.

## Phase 4: Architectural Consolidation & Asynchrony
Reduce operational overhead and improve system resilience.

- [ ] **Service Consolidation**
  - **Action**: Merge tightly coupled services.
  - **Implementation**: Combine `giveaway` and `entry` into a single `core` service. Combine `compliance` and `kyc` into a `risk` service. This reduces the microservice count, simplifying deployment and lowering internal gRPC traffic.
- [ ] **Introduce Event-Driven Communication**
  - **Action**: Refactor inter-service communication.
  - **Implementation**: Instead of `xgateway` making synchronous gRPC calls to `giveaway` and `payment-router`, use the existing Asynq/Redis setup as an event bus. `xgateway` should publish a `GiveawayRequested` event, allowing downstream services to react asynchronously.

## Phase 5: Infrastructure & Testing (Pre-Launch)
Solidify the deployment pipeline and ensure code reliability.

- [ ] **Migrate Deployment Infrastructure**
  - **Action**: Move away from the proposed 10-app Heroku strategy.
  - **Implementation**: Create a Kubernetes configuration (Helm charts or Kustomize) or an AWS ECS/Fargate setup. Ensure internal services are not exposed to the public internet.
- [ ] **Comprehensive Test Suite**
  - **Action**: Expand testing across all domains.
  - **Implementation**: Write integration tests using `testcontainers-go` to spin up ephemeral Postgres and Redis instances. Ensure the full end-to-end webhook-to-payout flow is tested under various failure conditions.

## Execution Strategy
1. **Immediate**: Execute Phase 1 (Security) and Phase 2 (Data) as they involve schema and configuration changes that form the bedrock of the app.
2. **Short-Term**: Address Phase 3 (Code Quality) to ensure transactional safety.
3. **Mid-Term**: Tackle Phase 4 (Architecture) and Phase 5 (Infrastructure) as they require significant refactoring and DevSecOps involvement.
