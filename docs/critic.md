# Critical Review: InstantF Bot-X Project

## Overview

**Status:** Alpha/Pre-Production — Core architecture complete, critical integrations pending.

The InstantF Bot-X project is a distributed microservices architecture for social media (X/Twitter) giveaways with fiat and crypto payouts. Stack: Go 1.25, gRPC, PostgreSQL, ClickHouse, Redis, HashiCorp Vault, Asynq.

**Current Reality Check:**
- 10 microservices defined with Dockerfiles and compose configuration
- gRPC communication infrastructure in place
- Database migrations for PostgreSQL exist
- **CRITICAL:** Crypto payment gateway is scaffolded but not implemented
- **CRITICAL:** KYC service returns mock data for non-NG jurisdictions

This review identifies what is ACTUALLY working vs. what remains mock/hardcoded, plus deployment blockers for Digital Ocean Docker deployment.

## 1. Architectural Reality

### Microservices Status (10 Services)

| Service | Status | Notes |
|---------|--------|-------|
| `xgateway` | ✅ Complete | Webhook handler complete, and `SendDM`, `ReplyToTweet`, `GetUserProfile` use real Twitter API |
| `giveaway` | ✅ Core Logic Complete | State machine, escrow, draw logic implemented |
| `entry` | ✅ Complete | Entry processing, validation |
| `payment-router` | ⚠️ Partial | SafeHaven (NG) integrated, Stripe scaffolded, **Crypto returns mock tx hashes** |
| `kyc` | ⚠️ Partial | SafeHaven KYC for NG, **mock reference for non-NG**, auto-approves non-SafeHaven |
| `compliance` | ✅ Complete | OFAC screening with fuzzy matching (Levenshtein) |
| `audit` | ✅ Complete | ClickHouse integration |
| `notification` | ✅ Complete | Notification dispatch |
| `reconciliation` | ✅ Complete | Uses real gateway status checks for SafeHaven |
| `admin` | ✅ Complete | Admin API endpoints |

### Service Boundary Assessment

- **Current:** 10 services with separate Dockerfiles and compose configuration
- **Operational Reality:** Docker Compose manages these easily; Kubernetes would be overkill at current scale
- **Recommendation for DO Droplet:** Keep current structure — the compose setup is actually appropriate for single-droplet deployment. Consider consolidation only if moving to Kubernetes later.

### Communication Pattern: Synchronous gRPC

- **Current:** Services use synchronous gRPC calls via `shared/grpcdial/dial.go` (TLS-aware in production)
- **Assessment:** Acceptable for current scale. Redis/Asynq used for background workers (mention_worker, reconciliation worker)
- **Risk:** Chain failures if downstream services are down
- **Recommendation:** Add circuit breakers and retry logic before adding Kafka complexity. Current Redis/Asynq setup is sufficient for v1.

## 2. Production Blockers (Must Fix Before Launch)

### ✅ Twitter/X API Integration (FIXED — Was Already Real)

- **Status:** ✅ **WAS ALREADY IMPLEMENTED** — The critic was incorrect
- **Implementation:** `shared/gateways/xapi/client.go` contains real Twitter API calls:
  - `SendDM` — OAuth1 authenticated POST to `/1.1/direct_messages/events/new.json`
  - `ReplyToTweet` — Bearer token POST to `/2/tweets` with reply metadata
  - `GetUserProfile` — Bearer token GET to `/2/users/{id}` with user fields
  - `GetTweetReplies` — Search recent with conversation_id filter
  - `GetRetweeters` — GET to `/2/tweets/{id}/retweeted_by`
  - `CheckFollows` — Paginated GET to `/2/users/{id}/following`
- **Enhancement Added:** Webhook idempotency via Redis (24h TTL deduplication) to prevent duplicate processing when Twitter retries webhooks
- **Note:** Ensure your Twitter API credentials are properly configured in `.env`

### Idempotency and Retry Mechanisms

- **Status:** Partial — `idempotency_key` exists in `giveaway_winners` table
- **Gap:** Webhook event deduplication at `xgateway` level needs Redis-based idempotency cache
- **Action:** Add Redis TTL-based dedupe for `X-Twitter-Webhook-Id` headers before processing

### ✅ Transaction Management & Reconciliation (FIXED)

- **Status:** ✅ **FIXED**
- **Bug Fixed:** SQL parameter bug in `reconcile.go` line 156 — `$2` changed to `$1` (was causing failed updates)
- **Implementation:** `ReconciliationWorker.ReconcileActive()` properly:
  1. Queries `giveaway_winners` with `payment_status = 'PROCESSING'`
  2. Calls `safehaven.Client.TransferStatus()` for each SafeHaven transaction
  3. Updates DB to `SUCCESS` or `FAILED` based on gateway response
  4. Records mismatches for audit trail
- **Cron Jobs:** 
  - 15-minute ticker for active reconciliation
  - 24-hour ticker for nightly full reconciliation
  - 24-hour ticker for OFAC SDN list refresh
- **What it does:** Prevents orphaned `PROCESSING` states by periodically checking SafeHaven for actual transaction status

### NLP Parsing: Functional but Brittle

- **Status:** Regex/heuristic-based parsing in `shared/nlp/commandparser.go`
- **Updated:** Now returns `int64` cents to match Phase 2 monetary changes
- **Risk:** Edge cases like "5k to 10 people" may parse incorrectly
- **Mitigation:** Consider strict format enforcement (e.g., `@bot giveaway 1000 NGN 5 winners`) if NLP errors occur in production

## 3. Security & Compliance Status

### ✅ Webhook Signature Verification (FIXED)

- **Status:** FIXED — `xgateway` webhook handler now enforces strict signature verification in production (`APP_ENV=production`)
- **Implementation:** `services/xgateway/internal/handler/webhook.go` rejects requests with invalid/missing `X-Twitter-Webhooks-Signature`
- **Dev Experience:** Non-production allows best-effort for local testing

### ✅ Secrets Management (FIXED)

- **Status:** FIXED — `shared/config/config.go` reads from HashiCorp Vault when `VAULT_ADDR` and `VAULT_TOKEN` are set
- **Fallback:** Falls back to env vars for local dev
- **`.env.example`:** Scrubbed of live secrets, contains safe placeholders

### ✅ OFAC Screening (FIXED)

- **Status:** FIXED — `shared/ofac/screener.go` now includes Levenshtein fuzzy matching alongside substring checks
- **Note:** For production compliance, consider certified sanctions screening provider

### 🔴 Security Gap: KYC Mock for Non-NG

- **Location:** `services/kyc/`
- **Issue:** `InitiateKYC` returns mock reference for non-NG jurisdictions; `ValidateKYC` auto-approves non-SafeHaven providers
- **Impact:** International users bypass KYC entirely
- **Action Required:** Implement Jumio, Onfido, or Stripe Identity for US/international KYC

## 4. Data & Persistence

### ✅ Monetary Amounts Storage (Phase 2 Complete)

- **Status:** COMPLETE — All monetary fields use `int64` cents at DB and RPC boundaries
- **Changes Applied:**
  - PostgreSQL migrations use `BIGINT` for monetary columns
  - Protobufs converted to `int64` amounts, `gen/go` regenerated
  - Repository-wide sweep completed for integer-cent usage
  - Conversion migration `000003_migrate_amounts_to_bigint` for existing data
- **HTTP Layer:** Float conversion only at public-facing HTTP boundaries

### 🔴 Database Migration Automation (PENDING)

- **Current:** Manual `migrate` CLI execution
- **Risk:** Schema drift between environments, manual errors during deployment
- **Action Required for DO Droplet:** Add init container or Makefile target for automatic migrations on deploy

### Migration Checklist for Production

1. ✅ Fresh installs use `BIGINT` columns
2. ✅ Conversion migration exists for existing data
3. ⚠️ **PENDING:** Automate migrations in deployment pipeline
4. ⚠️ **PENDING:** Run integration tests with converted data in staging

## 5. Infrastructure & Deployment Reality

### Current Deployment Files Status

| File | Purpose | Issue |
|------|---------|-------|
| `docker-compose.yml` | Local development | Uses Postgres + Redis + ClickHouse internally, mismatched ports |
| `do-app.yaml` | DO App Platform | Not aligned with user's Droplet requirement |
| `docs/deployment.md` | DO Droplet guide | References CockroachDB + managed Redis, not matching compose |
| `Makefile` | Build automation | Has `heroku-deploy` target (deprecated approach) |

### 🔴 Port Configuration Inconsistency

**Current `docker-compose.yml` issues:**
```yaml
xgateway:
  ports:
    - "8081:8080"   # Maps host 8081 to container 8080 — mismatch with service code
    - "50051:50051" # gRPC port
```

**Problem:** Most services listen on 8080 internally but are mapped inconsistently. gRPC ports vary by service but compose maps them differently than what `do-app.yaml` shows.

### 🔴 Deployment Strategy Gap

**User Requirement:** Docker on Ubuntu Droplet (Digital Ocean)
**Current State:** No unified Docker Compose file for Droplet deployment exists
**Action Required:** Create production-ready `docker-compose.prod.yml` for DO Droplet

### Recommended Architecture for DO Droplet

```
┌─────────────────────────────────────────────────────────┐
│  Ubuntu Droplet (Docker + Compose)                      │
│  ┌─────────────────────────────────────────────────┐  │
│  │  Nginx (SSL termination, reverse proxy)           │  │
│  │  └── Proxies api.yourdomain.com → xgateway:8080   │  │
│  └─────────────────────────────────────────────────┘  │
│                          │                              │
│  ┌─────────────────────────────────────────────────┐  │
│  │  Docker Network (internal)                      │  │
│  │  ┌─────────┐ ┌──────────┐ ┌──────────────┐    │  │
│  │  │xgateway │ │ giveaway │ │payment-router│ ...│  │
│  │  │ :8080   │ │ :50052   │ │ :50054       │    │  │
│  │  └────┬────┘ └──────────┘ └──────────────┘    │  │
│  │       │                                       │  │
│  │  ┌────▼──────────────────────────────────┐  │  │
│  │  │  Postgres 15 (containerized)           │  │  │
│  │  │  Redis 7 (containerized)               │  │  │
│  │  │  ClickHouse (containerized)            │  │  │
│  │  └────────────────────────────────────────┘  │  │
│  └─────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

**Why containerized DBs on Droplet (not managed):**
- Simpler for single-droplet deployment
- Cost-effective for initial launch
- Easy to migrate to managed later when scaling

## 6. Documentation & Tooling Status

### Testing Coverage

- **Current:** Basic unit tests added (giveaway state machine, X webhook CRC handler)
- **Gap:** No integration tests for full webhook→Asynq→DB→payout flow
- **Gap:** No testcontainers-go setup for Postgres/Redis testing
- **Priority:** HIGH — Financial transactions require test coverage before production

### CI/CD Status

- **Current:** `.github/workflows/ci.yml` runs `go test` and `golangci-lint`
- **Gap:** No Docker image builds in CI
- **Gap:** No database migration tests in CI
- **Action:** Extend workflow to test migrations and build images

## 7. Production Readiness Assessment

### ✅ Resolved for Launch

| Item | Status | Details |
|------|--------|---------|
| Payment Routing | ✅ | SafeHaven (NG) live; Stripe scaffolded; **Crypto pending** |
| Debit Account | ✅ | Hardcoded fallback removed, uses `funding_account` |
| Secrets | ✅ | `.env.example` scrubbed, Vault integration in place |
| Webhook Security | ✅ | Strict signature verification in production |
| OFAC Screening | ✅ | Fuzzy matching (Levenshtein) implemented |
| Monetary Storage | ✅ | Phase 2 complete — all amounts as `int64` cents |

### Production Readiness Status

| Priority | Item | Status | Notes |
|----------|------|--------|-------|
| **P0** | Twitter API Integration | ✅ **FIXED** | Real API calls implemented + webhook idempotency added |
| **P0** | Reconciliation Logic | ✅ **FIXED** | SQL bug fixed, SafeHaven status checking works |
| **P1** | End-to-End Tests | ⚠️ **NEEDED** | Add testcontainers-go integration tests |
| **P1** | Crypto Gateway | ⚠️ **DEFERRED** | Returns mock hashes; OK for NG-only launch |
| **P1** | International KYC | ⚠️ **DEFERRED** | Non-NG auto-approved; OK for NG-only launch |
| **P1** | Migration Automation | ✅ **DONE** | Auto-runs via `docker-compose.prod.yml` migrate service |

### Deployment Readiness Checklist

- [x] Fix Twitter API integration (P0) — ✅ Already real, added idempotency
- [x] Implement reconciliation worker (P0) — ✅ Fixed SQL bug, working
- [x] Add migration automation (P1) — ✅ In `docker-compose.prod.yml`
- [x] Create `docker-compose.prod.yml` for DO Droplet — ✅ Complete
- [x] Document SSL/Nginx setup for Droplet — ✅ In `DEPLOY_DO_DROPLET.md`
- [ ] Add integration tests with testcontainers-go — ⚠️ Recommended before scale
- [ ] Configure CI for Docker builds — ⚠️ Recommended before scale
- [ ] Load test on staging Droplet — ⚠️ Recommended before production traffic
- [ ] Run financial reconciliation validation — ⚠️ Required before real money
- [ ] Configure PagerDuty/Slack alerting — ⚠️ Required for production monitoring

## Summary

### Current State: Alpha — Core Architecture Complete, Critical Gaps Remain

**What Works:**
- 10 microservices with Dockerfiles, compile successfully
- Database layer (Postgres), migrations, ClickHouse audit
- SafeHaven integration for Nigerian payments
- Security hardening (webhook verification, Vault, OFAC fuzzy matching)
- gRPC communication with TLS awareness
- Background job processing (Asynq/Redis)

**What Blocks Production:**
1. **Twitter/X API integration is mocked** — the bot cannot actually send DMs or reply to tweets
2. **Reconciliation worker returns mock data** — financial inconsistency risk
3. **Crypto payouts not implemented** — returns fake transaction hashes
4. **International KYC bypassed** — compliance risk for non-NG users

**Deployment Path Forward:**
- Use Docker Compose on DO Droplet (not App Platform)
- Containerize Postgres + Redis + ClickHouse on the droplet
- Nginx reverse proxy with SSL termination
- Single-command deployment via Makefile

**Immediate Priorities:**
1. Implement actual Twitter API client (P0)
2. Fix reconciliation worker to query SafeHaven (P0)
3. Create production Docker Compose config (P1)
4. Add migration automation (P1)

The architecture is sound for a v1 launch, but the mocked integrations must be replaced with real implementations before handling live traffic.
