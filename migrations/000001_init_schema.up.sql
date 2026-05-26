CREATE TABLE IF NOT EXISTS giveaways (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    host_twitter_id     VARCHAR(255) NOT NULL,
    source_tweet_id     VARCHAR(255) UNIQUE NOT NULL,
    command_tweet_id    VARCHAR(255),
    total_budget        BIGINT NOT NULL,
    currency            VARCHAR(10) NOT NULL DEFAULT 'NGN',
    winner_count        INT NOT NULL,
    amount_per_winner   BIGINT NOT NULL,
    entry_rule          VARCHAR(100) NOT NULL DEFAULT 'RANDOM',
    jurisdiction        VARCHAR(10) NOT NULL DEFAULT 'NG',
    status              VARCHAR(50) NOT NULL DEFAULT 'DRAFT',
    escrow_reference    VARCHAR(255),
    escrow_gateway      VARCHAR(50),
    funding_account     VARCHAR(255),
    funding_bank_code   VARCHAR(50),
    deadline_at         TIMESTAMP,
    created_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    closed_at           TIMESTAMP,
    frozen_at           TIMESTAMP,
    frozen_by           VARCHAR(255),
    cancel_reason       VARCHAR(500)
);

CREATE TABLE IF NOT EXISTS giveaway_entries (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    giveaway_id     UUID NOT NULL REFERENCES giveaways(id),
    twitter_id      VARCHAR(255) NOT NULL,
    twitter_handle  VARCHAR(255),
    entry_type      VARCHAR(50) NOT NULL DEFAULT 'REPLY',
    trust_score     DECIMAL(5,2) NOT NULL DEFAULT 0,
    is_eligible     BOOLEAN NOT NULL DEFAULT TRUE,
    reject_reason   VARCHAR(100),
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(giveaway_id, twitter_id)
);

CREATE TABLE IF NOT EXISTS giveaway_winners (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    giveaway_id             UUID NOT NULL REFERENCES giveaways(id),
    winner_twitter_id       VARCHAR(255) NOT NULL,
    winner_twitter_handle   VARCHAR(255),
    payout_destination      VARCHAR(255),
    payout_destination_type VARCHAR(50),
    bank_code               VARCHAR(50),
    kyc_status              VARCHAR(50) NOT NULL DEFAULT 'NOT_REQUIRED',
    kyc_provider            VARCHAR(50),
    kyc_reference           VARCHAR(255),
    payment_status          VARCHAR(50) NOT NULL DEFAULT 'PENDING',
    gateway_used            VARCHAR(50),
    gateway_reference       VARCHAR(255),
    idempotency_key         UUID NOT NULL DEFAULT gen_random_uuid(),
    amount                  BIGINT NOT NULL,
    currency                VARCHAR(10) NOT NULL DEFAULT 'NGN',
    payout_initiated_at     TIMESTAMP,
    payout_completed_at     TIMESTAMP,
    created_at              TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at              TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(idempotency_key),
    UNIQUE(giveaway_id, winner_twitter_id)
);

CREATE TABLE IF NOT EXISTS payment_gateway_config (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    gateway                 VARCHAR(50) NOT NULL UNIQUE,
    enabled                 BOOLEAN NOT NULL DEFAULT FALSE,
    priority                INT NOT NULL DEFAULT 99,
    supported_currencies    TEXT[] NOT NULL,
    supported_jurisdictions TEXT[],
    updated_at              TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_by              VARCHAR(255)
);

CREATE TABLE IF NOT EXISTS kyc_config (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    jurisdiction    VARCHAR(10) NOT NULL UNIQUE,
    kyc_enabled     BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_by      VARCHAR(255)
);

CREATE TABLE IF NOT EXISTS host_profiles (
    twitter_id      VARCHAR(255) PRIMARY KEY,
    twitter_handle  VARCHAR(255) NOT NULL,
    jurisdiction    VARCHAR(10) NOT NULL DEFAULT 'NG',
    is_suspended    BOOLEAN NOT NULL DEFAULT FALSE,
    suspended_by    VARCHAR(255),
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Seed default gateway config
INSERT INTO payment_gateway_config (gateway, enabled, priority, supported_currencies, supported_jurisdictions) VALUES
  ('safehaven',   TRUE,  1, ARRAY['NGN'],       ARRAY['NG']),
  ('flutterwave', FALSE, 2, ARRAY['NGN','USD'],  ARRAY['NG']),
  ('paystack',    FALSE, 3, ARRAY['NGN'],        ARRAY['NG']),
  ('stripe',      FALSE, 1, ARRAY['USD'],        ARRAY['US']),
  ('crypto_usdt', FALSE, 1, ARRAY['USDT'],       NULL),
  ('crypto_usdc', FALSE, 2, ARRAY['USDC'],       NULL)
ON CONFLICT (gateway) DO NOTHING;

-- Seed KYC config (disabled by default)
INSERT INTO kyc_config (jurisdiction, kyc_enabled) VALUES
  ('NG', FALSE),
  ('US', FALSE)
ON CONFLICT (jurisdiction) DO NOTHING;
