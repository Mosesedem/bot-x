# X/Twitter Cash Giveaway Engine — Product Requirements Document

**Version:** 2.0  
**Classification:** Confidential  
**Status:** Pre-Build Review  
**Buildability:** High · Regulatory Risk: High · Complexity: Medium-High  
**Last Updated:** May 2026 — Payment architecture revised to support Nigeria-first multi-gateway model with global expansion rails

---

## Table of Contents

1. [Overview](#1-overview)
2. [Architecture Critique](#2-architecture-critique)
3. [Revised Architecture](#3-revised-architecture)
4. [Payment Gateway Architecture](#4-payment-gateway-architecture)
5. [Recommended Tech Stack](#5-recommended-tech-stack)
6. [Scaling to 1M RPS](#6-scaling-to-1m-rps)
7. [Security Model](#7-security-model)
8. [Build Roadmap](#8-build-roadmap)

---

## 1. Overview

### Problem Statement

Brands and influencers on X (Twitter) run cash giveaways manually — announcing winners in a thread, then DMing PayPal addresses, chasing unresponsive participants, and losing track of state across hundreds of replies. The process is error-prone, slow, and exposes hosts to fraud. There is no native, automated solution that closes the loop from "retweet to win" to verified payout.

### Geographic Strategy

The platform is **Nigeria-first**. Safe Haven MFB is the primary payment rail for Nigerian users, with Flutterwave and Paystack as supplementary options — all togglable from the admin dashboard. USA support via Stripe is available but disabled by default. Crypto payouts (stablecoins) are supported globally and configurable per-deployment from the admin dashboard.

### Core User Stories

| Persona          | Story                                                                                                                                          |
| ---------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| **Host**         | I can tweet a giveaway command and the bot handles entry collection, winner selection, and payouts without manual intervention.                |
| **Participant**  | I can enter a giveaway with a single interaction on X and receive payment to my preferred wallet if selected.                                  |
| **Platform Ops** | I can configure which payment gateways are active per region, toggle crypto support, and enable/disable Stripe (USA) from the admin dashboard. |
| **Admin**        | I can audit every giveaway state transition, payout attempt, gateway routing decision, and fraud flag in a tamper-evident ledger.              |

### High-Level Metrics

| Metric            | Target                                   |
| ----------------- | ---------------------------------------- |
| Target Scale      | 1,000,000 RPS                            |
| Core Risk         | Regulatory / Legal                       |
| MVP Timeline      | 14 Weeks                                 |
| Payout SLA        | 95% of payouts within 60 seconds of draw |
| Primary Market    | Nigeria (NGN)                            |
| Secondary Markets | USA (USD), Global (Stablecoins)          |

---

## 2. Architecture Critique

The original source document has sound instincts in several areas but contains critical gaps that would prevent a safe production launch. Each issue is ranked by severity with a concrete remediation path.

---

### 2.1 Critical Issues

#### ❌ No Regulatory / Licensing Model — CRITICAL

**Issue:** Distributing cash prizes via a social trigger constitutes a lottery or prize promotion in most jurisdictions (Nigeria, US, UK, EU). In Nigeria, this also intersects with CBN regulations on electronic fund transfers and prize promotions. The original document ignores this entirely.

**Fix:** Retain a Nigerian fintech attorney (familiar with CBN guidelines) and a US sweepstakes attorney before writing a line of code. Structure payouts as "skill-based" contests or "sweepstakes" with no purchase necessary. Geo-block unsupported jurisdictions at the webhook layer from day one. Obtain any required approvals from the CBN or FIRS for automated prize disbursement above threshold values.

---

#### ❌ No KYC / AML Compliance Layer — CRITICAL

**Issue:** Any platform moving money at scale requires KYC and AML controls. In Nigeria, Safe Haven MFB's Identity and Credit Check API provides BVN and NIN verification — this is the correct integration path for Nigerian users. For the US, FinCEN rules apply above a $600/year threshold.

**Fix:**

- **Nigeria:** Use Safe Haven's Identity Verification API (`POST /identity/initiate`, `POST /identity/validate`) for BVN/NIN verification of hosts and high-value winners.
- **USA:** Integrate Stripe Identity for winners above the $600 threshold. Configure Stripe for IRS Form 1099-MISC reporting.
- **Crypto:** Screen all wallet addresses against OFAC SDN list before any stablecoin payout is dispatched.

---

#### ❌ X API Cost Model Is Incorrect — CRITICAL

**Issue:** The original document claims that including URLs incurs "$0.20 per transaction." This is not how X API v2 pricing works. X charges per API access tier (Basic / Pro / Enterprise), not per URL in a tweet payload.

**Fix:** Re-audit X API v2 pricing tiers directly from X's developer portal. At meaningful volume (1M events/day), the Enterprise tier is required (~$42,000/month). This must be modelled into unit economics before any fundraising.

---

### 2.2 Major Issues

#### ⚠️ Single Payment Rail — MAJOR

**Issue (updated from v1):** The original spec used only Stripe Connect as the payment rail. This is unworkable for Nigeria — Stripe does not support NGN payouts to Nigerian bank accounts at any meaningful scale.

**Fix:** Implement a multi-gateway payment router (see Section 4). Safe Haven MFB is the primary rail for Nigeria. Flutterwave and Paystack provide redundancy and alternative collection methods. Stripe covers USD/USA payouts. Crypto covers global stablecoin settlements. The active gateway set is controlled per-deployment from the admin dashboard.

---

#### ⚠️ Single-Region Database — No HA or Disaster Recovery — MAJOR

**Issue:** The schema is well-structured, but there is no mention of replication, failover, or point-in-time recovery. A database failure during the `PAYING` state could result in duplicate or lost payouts.

**Fix:** Deploy PostgreSQL with synchronous streaming replication (primary + at least one standby). Use a managed solution (AWS RDS Multi-AZ or Supabase) in production. Enable WAL archiving to S3 for point-in-time recovery. Never run financial state on a single database node.

---

#### ⚠️ Sybil Defence Is Underpowered — MAJOR

**Issue:** The proposed "30-day account age" filter is trivially bypassed. Sophisticated actors purchase aged X accounts in bulk for under $1 each.

**Fix:** Implement a dedicated trust score combining: account age, follower/following ratio, tweet history entropy, device fingerprint (via OAuth callback metadata), and optional phone verification. Consider integrating a purpose-built bot-detection vendor such as Arkose Labs or DataDome.

---

### 2.3 Minor Issues

#### ℹ️ No Webhook Signature Verification Detail — MINOR

**Issue:** The document mentions "cryptographic webhook management" without specifying the implementation. X webhooks use a CRC token challenge plus HMAC-SHA256 header verification.

**Fix:** Implement X's CRC challenge-response endpoint. Verify the `x-twitter-webhooks-signature` header on every inbound payload using HMAC-SHA256 with the consumer secret. Reject any payload that fails verification with an HTTP 403 before it touches any business logic.

---

#### ℹ️ No Rate Limit on Giveaway Creation — MINOR

**Issue:** A single user could spam giveaway creation commands, exhausting both API quota and the pre-funded balance system simultaneously.

**Fix:** Enforce per-host rate limits (e.g., maximum 5 concurrently active giveaways) at the application layer. Add Redis-backed sliding window rate limiting on the webhook ingestion endpoint.

---

### 2.4 What the Original Document Gets Right

The following patterns are well-specified and should be preserved:

- **Idempotency keys on every payout call** — non-negotiable for financial correctness, correctly specified.
- **Pre-funded escrow model** — prevents chargebacks and ensures funds exist before exposure.
- **State machine for giveaway lifecycle** — clean approach that prevents double-spend with `FOR UPDATE` locks.
- **CSPRNG for winner selection** — eliminates statistical bias in winner selection.

---

## 3. Revised Architecture

### 3.1 System Topology

```
INGRESS
  → Cloudflare WAF + DDoS protection
  → Global Anycast Load Balancer
  → TLS termination + IP reputation filter

API GATEWAY
  → Kong / AWS API Gateway
  → JWT auth, rate limiting, request logging
  → Webhook signature verification middleware

APPLICATION (Go services)
  → Stateless Go pods (Kubernetes / HPA)
  → Asynq workers — entry ingestion, winner draw, payout dispatch
  → KYC orchestration service (Safe Haven Identity / Stripe Identity)
  → Payment Router service (gateway selection + fallback logic)
  → Compliance/geo-blocking service

DATA
  → CockroachDB (primary + read replicas, multi-region)
  → Redis Cluster / Dragonfly (queues + session cache)
  → ClickHouse (analytics + append-only audit log)

PAYMENTS (multi-gateway, admin-configurable)
  → Safe Haven MFB API  [Nigeria — PRIMARY, enabled by default]
  → Flutterwave          [Nigeria — fallback / alternative, admin toggle]
  → Paystack             [Nigeria — fallback / alternative, admin toggle]
  → Stripe Connect       [USA — disabled by default, admin toggle]
  → Stablecoin (USDT/USDC on-chain) [Global — admin toggle]
  → Internal ledger reconciliation worker
```

---

### 3.2 Revised State Machine

The state machine is extended with a `KYC_PENDING` state and a `GATEWAY_ROUTING` state to reflect the multi-gateway payment flow.

```
DRAFT → ACTIVE → LOCKED → DRAWING → KYC_PENDING → GATEWAY_ROUTING → PAYING → COMPLETED
```

Terminal states: `FAILED` · `CANCELLED` · `DISPUTED`

> **KYC_PENDING:** Winners must pass identity verification (BVN/NIN via Safe Haven Identity API for Nigerian users; Stripe Identity for US users) before the `PAYING` state is entered.

> **GATEWAY_ROUTING:** The payment router evaluates the winner's jurisdiction, the admin dashboard's active gateway configuration, and gateway health to select the optimal payout rail before dispatching.

---

### 3.3 Critical Additions vs. Source Document

#### Compliance Service

A dedicated microservice handling geo-blocking, jurisdiction rules, age verification flags, CBN threshold reporting (Nigeria), and IRS reporting thresholds (USA). Runs synchronously before any giveaway is marked `ACTIVE`.

#### Payment Router

A dedicated routing service that selects the correct payment gateway per winner based on: jurisdiction, admin-configured gateway enable/disable state, gateway health (circuit breaker), and currency denomination. Falls back to the next enabled gateway in the priority order if the primary fails.

#### Fraud Score Engine

Real-time trust scoring on participant registration. Combines X account signals, device fingerprint, and velocity checks. Participants scoring below a configured threshold are silently soft-rejected — the rejection reason is not exposed to prevent adversarial tuning.

#### Reconciliation Worker

A scheduled job (every 15 minutes during active giveaways, nightly otherwise) that cross-references internal ledger rows against payment gateway webhook events. Any mismatch triggers a PagerDuty alert and freezes the affected giveaway for manual review. Must reconcile across all active gateways simultaneously.

#### Append-Only Audit Log

Every state transition, payment attempt, gateway routing decision, and admin action is written to a separate ClickHouse table with no `UPDATE` or `DELETE` permissions granted to the application service account.

---

### 3.4 Updated Database Schema

```sql
-- Giveaways with compliance metadata
CREATE TABLE giveaways (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    host_twitter_id     VARCHAR(255) NOT NULL,
    tweet_id            VARCHAR(255) UNIQUE NOT NULL,
    total_budget        NUMERIC(12, 2) NOT NULL,
    currency            VARCHAR(10) NOT NULL DEFAULT 'NGN', -- NGN, USD, USDT, USDC
    winner_count        INT NOT NULL,
    amount_per_winner   NUMERIC(12, 2) NOT NULL,
    jurisdiction        VARCHAR(10) NOT NULL,   -- ISO 3166-1 alpha-2 (e.g. NG, US)
    status              VARCHAR(50) NOT NULL DEFAULT 'DRAFT',
    -- DRAFT, ACTIVE, LOCKED, DRAWING, KYC_PENDING, GATEWAY_ROUTING, PAYING, COMPLETED, FAILED, CANCELLED
    created_at          TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    closed_at           TIMESTAMP,
    escrow_reference    VARCHAR(255)   -- payment gateway reference (Safe Haven / Stripe / Paystack / Flutterwave)
);

-- Payment gateway configuration (admin-managed)
CREATE TABLE payment_gateway_config (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    gateway         VARCHAR(50) NOT NULL UNIQUE,
    -- safehaven, flutterwave, paystack, stripe, crypto_usdt, crypto_usdc
    enabled         BOOLEAN NOT NULL DEFAULT FALSE,
    priority        INT NOT NULL DEFAULT 99,   -- lower = higher priority in routing
    supported_currencies  TEXT[] NOT NULL,     -- e.g. '{NGN}', '{USD}', '{USDT,USDC}'
    supported_jurisdictions TEXT[],            -- NULL = all; e.g. '{NG}', '{US}'
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_by      VARCHAR(255)               -- admin user who toggled
);

-- Seed data (default state — Safe Haven on, others off)
INSERT INTO payment_gateway_config (gateway, enabled, priority, supported_currencies, supported_jurisdictions) VALUES
  ('safehaven',   TRUE,  1, '{NGN}',       '{NG}'),
  ('flutterwave', FALSE, 2, '{NGN,USD}',   '{NG}'),
  ('paystack',    FALSE, 3, '{NGN}',       '{NG}'),
  ('stripe',      FALSE, 1, '{USD}',       '{US}'),
  ('crypto_usdt', FALSE, 1, '{USDT}',      NULL),
  ('crypto_usdc', FALSE, 2, '{USDC}',      NULL);

-- Winners with KYC and payout state
CREATE TABLE giveaway_winners (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    giveaway_id             UUID REFERENCES giveaways(id),
    winner_twitter_id       VARCHAR(255) NOT NULL,
    payout_destination      VARCHAR(255),        -- bank account, wallet address, etc.
    payout_destination_type VARCHAR(50),         -- bank_account, mobile_money, crypto_wallet
    kyc_status              VARCHAR(50) DEFAULT 'NOT_REQUIRED',
    -- NOT_REQUIRED, PENDING, APPROVED, REJECTED
    kyc_provider            VARCHAR(50),         -- safehaven_identity, stripe_identity
    kyc_reference           VARCHAR(255),
    payment_status          VARCHAR(50) NOT NULL DEFAULT 'PENDING',
    -- PENDING, ROUTING, PROCESSING, SUCCESS, FAILED
    gateway_used            VARCHAR(50),         -- which gateway processed this payout
    gateway_reference       VARCHAR(255),        -- gateway-specific transaction reference
    idempotency_key         UUID UNIQUE NOT NULL,
    updated_at              TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Append-only audit log (ClickHouse schema)
CREATE TABLE audit_events (
    event_id        UUID,
    entity_type     LowCardinality(String),
    entity_id       String,
    action          LowCardinality(String),
    actor_id        String,
    gateway         LowCardinality(String),   -- payment gateway involved, if any
    payload         String,                   -- JSON
    created_at      DateTime64(3)
) ENGINE = MergeTree()
ORDER BY (created_at, entity_type, entity_id);
```

---

## 4. Payment Gateway Architecture

This section is new in v2. It defines how each gateway is integrated, how routing works, and what the admin dashboard controls.

---

### 4.1 Gateway Overview

| Gateway                | Region    | Currency    | Default State | Use Case                                                             |
| ---------------------- | --------- | ----------- | ------------- | -------------------------------------------------------------------- |
| **Safe Haven MFB**     | Nigeria   | NGN         | **Enabled**   | Primary Nigerian payout rail — bank transfers via CBN-licensed MFB   |
| **Flutterwave**        | Nigeria + | NGN, USD    | Disabled      | Fallback for Nigerian users; also supports mobile money              |
| **Paystack**           | Nigeria   | NGN         | Disabled      | Additional Nigerian fallback; strong local bank coverage             |
| **Stripe Connect**     | USA       | USD         | Disabled      | US winner payouts; handles 1099 reporting                            |
| **Crypto (USDT/USDC)** | Global    | Stablecoins | Disabled      | Cross-border payouts; settles on EVM-compatible chain (configurable) |

---

### 4.2 Safe Haven MFB Integration (Primary — Nigeria)

Safe Haven MFB is a CBN-licensed Microfinance Bank with a developer API. It is the primary payment rail for all Nigerian giveaway payouts.

**Key API flows used:**

**Authentication:** OAuth 2.0 client credentials flow. Exchange client assertion (JWT signed with RS256 private key) for a short-lived access token via `POST /oauth2/token`. Tokens must be refreshed before expiry — the Go payment service handles token lifecycle automatically.

**Account funding (escrow):** When a host funds a giveaway, a dedicated Safe Haven virtual account is created per giveaway (`POST /virtual-accounts`). The host transfers the prize pool to this virtual account. The system polls or receives a webhook confirming receipt before marking the giveaway `ACTIVE`.

**Winner payout (transfer):** On payout dispatch, the service calls `POST /transfers` with the winner's bank account details. Each call includes an idempotency key. The system calls `POST /transfers/status` to poll for terminal state, and also handles the transfer webhook event to update the internal ledger.

**Identity verification (KYC):** For Nigerian winners above the configured threshold (e.g. ₦100,000), call `POST /identity/initiate` to trigger BVN or NIN verification, then `POST /identity/validate` to confirm the OTP or result. KYC state is recorded in `giveaway_winners.kyc_status`.

**Name enquiry:** Before dispatching any transfer, call `POST /transfers/name-enquiry` to validate the destination account number and bank code. This prevents failed transfers due to incorrect account details.

**Webhooks:** Safe Haven sends webhook events for transfer completions and virtual account inflows. Verify webhook signatures per their documentation before processing. Update the internal ledger on every terminal event.

---

### 4.3 Flutterwave Integration (Nigeria Fallback)

Flutterwave is the first fallback for Nigerian payouts when Safe Haven is unavailable or disabled.

**Key flows:**

- **Disbursements:** Use Flutterwave's Transfer API (`POST /transfers`) for bank account payouts in NGN.
- **Collection:** Use Flutterwave's Payment Links or Virtual Account Number generation for escrow funding.
- **Webhook verification:** Validate the `verif-hash` header against the configured secret hash on all inbound events.
- **Idempotency:** Pass `reference` field (UUID) on every transfer call. Flutterwave deduplicates on this field.

---

### 4.4 Paystack Integration (Nigeria Fallback)

Paystack is the second fallback for Nigerian payouts.

**Key flows:**

- **Disbursements:** Use Paystack's Transfer API (`POST /transfer`) with a recipient code created via `POST /transferrecipient`.
- **Collection:** Use Paystack's Dedicated Virtual Account for per-giveaway escrow funding.
- **Webhook verification:** Validate the `x-paystack-signature` HMAC-SHA512 header on all inbound events.
- **Idempotency:** Pass `reference` field (UUID) on every transfer call.

---

### 4.5 Stripe Connect Integration (USA — Disabled by Default)

Stripe Connect handles USD payouts to US-based winners. It is disabled by default and must be explicitly enabled from the admin dashboard.

**Key flows:**

- **Escrow:** Use Stripe PaymentIntents with `capture_method: manual` to pre-authorize the giveaway budget.
- **Payouts:** Use Stripe Connect Transfers to connected accounts (winners). Use Stripe's Payout batch object (up to 1,000 recipients) for large concurrent draws.
- **KYC:** Stripe Identity for winners above the $600 threshold.
- **1099 reporting:** Configured automatically via Stripe Connect's tax reporting feature. Must be set up explicitly per the Stripe documentation.
- **Throughput:** Stripe enforces ~100 RPS per account. At scale, use multiple Platform accounts (one per US region) and batch payout objects. Implement exponential backoff with full jitter in the payout worker.

---

### 4.6 Crypto (Stablecoin) Integration (Global — Disabled by Default)

Stablecoin payouts offer a permissionless global payout rail for international winners. Disabled by default; enabled and configured from the admin dashboard.

**Supported assets:** USDT and USDC (configured per deployment). Recommended chains: Polygon or Base (low fees, fast finality) rather than Ethereum mainnet.

**Key design decisions:**

- The platform holds a treasury wallet per supported asset. This wallet is funded by the host as part of escrow.
- On payout, the service constructs and broadcasts an on-chain transfer transaction from the treasury wallet to the winner's wallet address.
- Use a non-custodial signing approach: the treasury wallet private key is stored in HashiCorp Vault (never in application config or environment variables). The payment service requests a signature from Vault at transaction time.
- Transaction hashes are stored as `gateway_reference` in `giveaway_winners`.
- **OFAC screening:** Screen every winner wallet address against the OFAC SDN list before any on-chain transfer is dispatched. Reject and flag any match.
- **Finality confirmation:** Wait for at least 12 block confirmations before marking a crypto payout as `SUCCESS`. Use a webhook or polling loop on the RPC provider (e.g. Alchemy, QuickNode).
- Crypto payouts do **not** require traditional KYC by default, but the admin dashboard can enforce a KYC gate before crypto payouts for compliance in specific jurisdictions.

---

### 4.7 Payment Router Logic

The payment router is a Go service that selects the correct gateway for each winner payout. The routing algorithm is:

```
1. Determine winner jurisdiction from giveaway.jurisdiction
2. Determine payout currency (NGN for NG, USD for US, or winner-chosen stablecoin)
3. Query payment_gateway_config for enabled gateways matching jurisdiction + currency, ordered by priority
4. If no enabled gateway matches → fail payout with status FAILED and alert ops
5. Attempt payout via highest-priority matching gateway
6. On gateway error or timeout → circuit breaker opens → retry with next enabled gateway
7. Record gateway_used and gateway_reference on success
8. Append routing decision to audit_events
```

This logic is entirely driven by the `payment_gateway_config` table. Admin dashboard changes take effect within 30 seconds (config is cached with a short TTL in Redis).

---

### 4.8 Admin Dashboard — Gateway Controls

The admin dashboard exposes the following controls per gateway:

| Control                       | Description                                                  |
| ----------------------------- | ------------------------------------------------------------ |
| Enable / Disable toggle       | Instantly activates or deactivates a gateway for new payouts |
| Priority order                | Drag to reorder routing priority among enabled gateways      |
| Supported jurisdictions       | Override which countries can use this gateway                |
| API credential status         | Shows whether credentials are configured and last verified   |
| Health status                 | Live circuit breaker state (CLOSED / OPEN / HALF_OPEN)       |
| Transaction volume (last 24h) | Monitoring widget                                            |

All admin changes are written to `audit_events` with the acting admin's ID and timestamp.

---

## 5. Recommended Tech Stack

Each layer is chosen for financial-grade reliability, Nigeria-first deployment, and scalability to 1M RPS.

### 5.1 Backend Runtime: Go (Golang)

**Go is the recommended backend runtime.** The rationale over the alternatives:

| Criterion                 | Go                                                  | Python                          | Fastify (Node.js)                           |
| ------------------------- | --------------------------------------------------- | ------------------------------- | ------------------------------------------- |
| Raw throughput            | ✅ Best — compiled, minimal GC pauses               | ❌ GIL limits concurrency       | ✅ Good — but single-threaded event loop    |
| Concurrency model         | ✅ Goroutines — trivial concurrent gateway calls    | ⚠️ asyncio works but complex    | ✅ async/await works                        |
| Memory footprint          | ✅ Lowest — important at 1M RPS pod density         | ❌ High interpreter overhead    | ⚠️ Medium — V8 overhead                     |
| Static typing             | ✅ Compile-time safety for financial logic          | ❌ Runtime errors               | ⚠️ TypeScript helps but is still transpiled |
| Deployment simplicity     | ✅ Single binary, no runtime dependency             | ❌ virtualenv / dependency hell | ⚠️ node_modules complexity                  |
| Payment SDK support       | ✅ Strong (Stripe Go, community Safe Haven clients) | ✅ Best SDK coverage            | ✅ Good SDK coverage                        |
| Team hire-ability (Lagos) | ⚠️ Growing Go community                             | ✅ Large pool                   | ✅ Largest pool                             |

**Verdict:** Go's superior throughput, concurrency model, and single-binary deployments make it the correct choice for a financial-grade, high-throughput payout system. The Nigeria tech community has a solid and growing Go ecosystem.

---

### 5.2 Layer-by-Layer Stack

| Layer             | Technology              | Rationale                                                                           |
| ----------------- | ----------------------- | ----------------------------------------------------------------------------------- |
| Ingress           | Cloudflare              | Anycast DDoS mitigation, webhook IP allowlisting, edge Workers for geo-blocking     |
| API Gateway       | Kong OSS                | Plugin ecosystem: JWT auth, rate limiting, structured request logging               |
| Runtime           | Go 1.22+                | Compiled, low-latency, goroutine-per-request concurrency; single binary deploys     |
| HTTP Framework    | Chi or Gin              | Lightweight Go routers; Chi is stdlib-compatible; Gin is faster for JSON-heavy APIs |
| Job Queue         | Asynq (Redis-backed)    | Go-native task queue; Redis-backed; built-in retry, deduplication, scheduling       |
| Primary DB        | CockroachDB             | Distributed SQL, ACID, horizontal write scaling; PostgreSQL wire-compatible         |
| Cache             | Dragonfly               | Redis-compatible; up to 25x throughput vs Redis on identical hardware               |
| Analytics         | ClickHouse              | Columnar OLAP; handles billions of audit rows cheaply                               |
| Payments Nigeria  | Safe Haven MFB API      | Primary NGN payout rail for Nigerian winners                                        |
| Payments Nigeria+ | Flutterwave / Paystack  | Fallback Nigerian rails; togglable from admin                                       |
| Payments USA      | Stripe Connect          | Licensed US money transmitter; 1099 reporting; disabled by default                  |
| Payments Global   | USDT / USDC on-chain    | Stablecoin payouts on Polygon/Base; disabled by default                             |
| KYC Nigeria       | Safe Haven Identity API | BVN / NIN verification for Nigerian winners                                         |
| KYC USA           | Stripe Identity         | US winner identity verification                                                     |
| Infra             | Kubernetes (EKS)        | Pod autoscaling (HPA), rolling deploys, service mesh (Linkerd)                      |
| Observability     | Grafana + Tempo + Loki  | Distributed tracing, SLO dashboards, log aggregation                                |
| Secrets           | HashiCorp Vault         | Dynamic secrets, auto-rotation, crypto wallet key management                        |

---

### 5.3 What to Avoid and Why

| Avoid                                    | Reason                                                                                                                           |
| ---------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| **Python as primary backend**            | GIL limits true parallelism. Poor choice for high-throughput concurrent payment dispatching.                                     |
| **Node.js / Fastify as primary backend** | Single-threaded event loop; V8 memory overhead at pod density; TypeScript adds build complexity with no runtime benefit over Go. |
| **Prisma or any Node ORM**               | Not applicable — Go uses `pgx` or `sqlc` for type-safe DB access with no query planner overhead.                                 |
| **Single PostgreSQL node**               | Cannot scale writes past ~10K TPS. CockroachDB required at target scale.                                                         |
| **DIY money movement (Nigeria)**         | Always use CBN-licensed rails (Safe Haven, Flutterwave, Paystack). Never build custom interbank transfer logic.                  |
| **Ethereum mainnet for crypto**          | Gas fees make micro-payouts impractical. Use Polygon or Base.                                                                    |
| **Storing crypto private keys in .env**  | Critical security failure. Use HashiCorp Vault with dynamic secret leases.                                                       |

---

## 6. Scaling to 1M RPS

1,000,000 requests per second equates to approximately 86 billion requests per day. Architectural decisions for this scale must be made at day one — retrofitting is prohibitively expensive.

---

### 6.1 Scaling Tiers

#### Tier 1 — Launch (up to 10K RPS)

- 3-node Go cluster (bare metal or EC2)
- Single CockroachDB region (3 nodes)
- Redis Standalone
- Asynq with 4 workers
- Cloudflare Free/Pro
- Safe Haven + one fallback gateway active

#### Tier 2 — Growth (up to 100K RPS)

- Kubernetes with Horizontal Pod Autoscaler (20 pods)
- CockroachDB multi-region deployment
- Redis Cluster (6 nodes) → Dragonfly
- Kong API Gateway with rate limiting plugins
- All Nigerian gateways active; Stripe optionally enabled

#### Tier 3 — Scale (up to 1M RPS)

- Global Anycast + Cloudflare Workers for edge pre-filtering (jurisdiction check, basic bot scoring)
- CockroachDB 9-node cluster across 3 regions
- Dragonfly + CDN edge cache for read-heavy paths
- Kafka replacing Asynq for high-throughput event streaming
- ClickHouse sharded cluster for analytics
- All payment gateways active and load-balanced

---

### 6.2 The Key Bottlenecks at 1M RPS

#### Bottleneck 1 — Database Write Contention

**Problem:** At 1M RPS, a PostgreSQL primary saturates around 50K writes/second.

**Solution:** CockroachDB distributes writes across nodes using range-based sharding. Participant inserts are partitioned by `giveaway_id`. Giveaway state is updated via optimistic locking (compare-and-swap), not `FOR UPDATE`, to eliminate lock contention at high concurrency.

---

#### Bottleneck 2 — X API Fan-Out Limits

**Problem:** X's Enterprise API has hard rate limits. Naive polling will exhaust quota in seconds at scale.

**Solution:** Use X's Filtered Stream endpoint (v2) with rules scoped per active giveaway. Fan out to isolated Asynq/Kafka queues per giveaway. All read consolidation happens at draw time via a single paginated `conversation_id` lookup.

---

#### Bottleneck 3 — Payment Gateway Throughput (Multi-Gateway)

**Problem:** Every gateway has rate limits. Safe Haven, Flutterwave, and Paystack each have API throughput ceilings. Stripe enforces ~100 RPS per account. On-chain crypto transactions are limited by block time and RPC provider rate limits.

**Solution:**

- **Nigeria:** Distribute load across Safe Haven, Flutterwave, and Paystack simultaneously (not sequentially). The payment router can fan out in parallel when multiple gateways are enabled, using the most available gateway per batch.
- **USA (Stripe):** Multiple Platform accounts (one per US region); Stripe batch Payout objects (up to 1,000 recipients per batch); exponential backoff with full jitter.
- **Crypto:** Use a batch transaction approach where supported (ERC-20 batch transfer contracts on Polygon/Base). One on-chain transaction can pay multiple winners, reducing RPC calls and gas.

---

### 6.3 Event Stream Architecture (1M RPS)

```
X Filtered Stream
  → Cloudflare Worker (edge pre-filter: bot score, jurisdiction check)
    → Kafka topic: raw-events (partitioned by giveaway_id)
      → Go Kafka consumer (dedup + sybil scoring)
        → CockroachDB (participant insert — idempotent upsert)
        → ClickHouse (analytics event append)

At draw time:
  single paginated read → CSPRNG selection → KYC check (Safe Haven / Stripe) → payment router → gateway dispatch → reconciliation
```

---

## 7. Security Model

### 7.1 Webhook Hardening

- HMAC-SHA256 signature verification on every inbound X event payload
- CRC challenge-response endpoint for X webhook registration and revalidation
- Safe Haven webhook signature verification per their API documentation
- Flutterwave `verif-hash` and Paystack `x-paystack-signature` HMAC verification
- Stripe `stripe-signature` webhook verification
- IP allowlist restricted to each gateway's published webhook egress IP ranges
- Replay protection: reject any event with a timestamp older than 5 minutes

### 7.2 Financial Controls

- **Escrow-only model:** funds are locked and cleared before a giveaway is marked `ACTIVE`
- Idempotency keys on 100% of outbound payment API calls across all gateways
- Name enquiry (Safe Haven / Flutterwave / Paystack) before any Nigerian bank transfer
- OFAC SDN list screening before every payout — including crypto wallet addresses
- Dual-approval workflow for any manual payout override above ₦500,000 / $1,000
- Reconciliation job runs every 15 minutes during active giveaways, across all active gateways
- Circuit breakers on every gateway client — automatically route around degraded providers

### 7.3 Infrastructure Security

- HashiCorp Vault for all secrets — no `.env` files in production; crypto private keys stored in Vault with dynamic lease
- Zero-trust network topology: all service-to-service communication via mTLS (Linkerd)
- Least-privilege IAM: separate roles for DB reader, DB writer, payment dispatcher, and crypto signer
- Append-only audit log in ClickHouse satisfies SOC 2 Type II requirements

### 7.4 Fraud and Abuse

- Trust score below 40 → entry silently rejected (reason not disclosed to prevent adversarial tuning)
- Velocity cap: maximum 1 entry per participant per giveaway (DB-layer unique constraint)
- Bulk account detection via shared phone number, BVN, or email at KYC
- Winning wallet/account screened against OFAC SDN list before any payout is dispatched

---

### 7.5 Regulatory Non-Negotiables

#### Nigeria — CBN Compliance

Automated prize disbursements above CBN-defined thresholds may require additional reporting or approval. Work with a Nigerian fintech attorney to determine applicable thresholds. Safe Haven MFB is a CBN-licensed entity — operating through them significantly reduces direct regulatory exposure.

#### No Purchase Necessary (US)

All US-facing giveaways must offer a free alternative method of entry. Without this, the product is legally classified as an illegal lottery under most US state statutes.

#### IRS 1099 Reporting (US)

Winners receiving more than $600 in aggregate prizes in a calendar year require Form 1099-MISC. Stripe Connect handles this automatically when configured correctly.

#### GDPR / NDPR / CCPA

Participant Twitter IDs, BVN references, bank account numbers, and wallet addresses are PII. Implement data retention limits, right-to-erasure workflows, and data processing agreements with all sub-processors. Nigeria's NDPR (National Data Protection Regulation) applies to Nigerian users.

#### State-Level Prize Law (US)

Florida, New York, and Rhode Island have specific registration requirements for promotional sweepstakes above certain prize thresholds. Geo-block these states or complete the required filings before operating there.

---

## 8. Build Roadmap

Assumes a 3-engineer team. Legal and compliance work runs in parallel from Week 1.

---

### Phase 1 — Foundation (Weeks 1–4) · MVP

- Legal entity setup and attorneys (Nigerian fintech + US sweepstakes)
- X Developer account (Basic tier) and webhook registration
- Core Go service scaffold: Chi/Gin router, Asynq workers, CockroachDB schema
- Webhook ingestion endpoint with HMAC-SHA256 verification
- Safe Haven MFB integration: virtual account creation, name enquiry, transfer, webhook handling
- Safe Haven Identity integration: BVN/NIN KYC for Nigerian winners
- BullMQ → Asynq worker: entry collection and CSPRNG winner selection
- Basic host authentication (OAuth 2.0 + PKCE via X)
- Admin dashboard skeleton: gateway enable/disable toggles backed by `payment_gateway_config`

---

### Phase 2 — Trust & Compliance (Weeks 5–8) · Beta

- Flutterwave integration (Nigerian fallback)
- Paystack integration (Nigerian fallback)
- Payment router service with circuit breaker and fallback logic
- Fraud trust score engine (account age, velocity, follower/following ratio)
- Geo-blocking middleware (OFAC screening, jurisdiction rules, NDPR data handling)
- Append-only audit log in ClickHouse (including gateway routing decisions)
- Reconciliation worker with Slack/PagerDuty alerting (multi-gateway aware)
- Closed beta with 10 verified hosts
- Admin dashboard: full gateway management UI, health status, transaction volume widgets

---

### Phase 3 — Global Rails (Weeks 9–12) · GA Prep

- Stripe Connect integration for USA (disabled by default, admin toggle)
- Stripe Identity for US winners above $600 threshold
- IRS 1099 reporting configuration via Stripe
- Crypto payout integration (USDT/USDC on Polygon or Base)
- HashiCorp Vault for crypto private key management
- OFAC wallet address screening in payment router
- Admin dashboard: crypto network/asset configuration
- Load testing to 50K RPS; identify and resolve hotspots

---

### Phase 4 — Scale Infrastructure (Weeks 13–16) · General Availability

- Kubernetes migration (EKS) with Horizontal Pod Autoscaler
- Redis → Dragonfly cluster migration
- Kong API Gateway deployment with rate limiting plugins
- Multi-region CockroachDB deployment
- Public launch with billing system (per-giveaway fee model)
- SOC 2 Type II audit initiation

---

### Phase 5 — 1M RPS Architecture (Month 6+) · Scale

- Kafka event stream replacing direct Asynq fan-out
- Cloudflare Workers for edge pre-filtering
- Flink stateful processing for real-time sybil detection at stream ingress
- Multi-region active-active CockroachDB topology
- X Enterprise API contract negotiation
- Parallel gateway payout routing at draw time (fan-out across all active gateways simultaneously for large winner sets)

---

## Appendix A — Overall Buildability Assessment

| Dimension                 | Rating | Notes                                                                                                                                                                            |
| ------------------------- | ------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Technical Feasibility** | HIGH   | All components are proven, open-source or SaaS. Go + Safe Haven is a production-viable combination for Nigerian fintech.                                                         |
| **Regulatory Risk**       | HIGH   | This is the primary kill risk. Non-compliance with CBN rules, Nigerian prize law, or FinCEN rules can end the company at any stage.                                              |
| **Unit Economics**        | MEDIUM | X Enterprise API alone is ~$42K/month. Safe Haven, Flutterwave, and Paystack charge per-transfer fees in addition. Requires meaningful giveaway volume to achieve profitability. |
| **Payment Coverage**      | HIGH   | Safe Haven (primary) + Flutterwave + Paystack covers the Nigerian market comprehensively. Stripe covers USA. Crypto covers the rest.                                             |

---

## Appendix B — Safe Haven MFB API Quick Reference

| Action                 | Endpoint                       | Notes                                             |
| ---------------------- | ------------------------------ | ------------------------------------------------- |
| Authenticate           | `POST /oauth2/token`           | Client credentials + signed JWT assertion (RS256) |
| Create virtual account | `POST /virtual-accounts`       | One per giveaway for escrow                       |
| Name enquiry           | `POST /transfers/name-enquiry` | Validate account before transfer                  |
| Initiate transfer      | `POST /transfers`              | Include UUID idempotency key as `reference`       |
| Check transfer status  | `POST /transfers/status`       | Poll for terminal state                           |
| Initiate KYC           | `POST /identity/initiate`      | BVN or NIN verification                           |
| Validate KYC           | `POST /identity/validate`      | Confirm OTP / verification result                 |
| Receive webhooks       | —                              | Verify signature per Safe Haven webhook docs      |

---

_Document prepared as a build-proof PRD. All regulatory estimates should be validated with qualified legal counsel (Nigerian and US) before proceeding. Safe Haven MFB API capabilities should be validated directly with their developer team for production rate limits and agreement terms._
