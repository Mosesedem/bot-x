-- Migration: Convert monetary NUMERIC(12,2) columns to BIGINT (store lowest denomination, e.g., kobo/cents)
-- This migration multiplies existing decimal values by 100 and casts to BIGINT.

BEGIN;

-- Add temporary bigint columns
ALTER TABLE giveaways ADD COLUMN IF NOT EXISTS total_budget_bigint BIGINT;
ALTER TABLE giveaways ADD COLUMN IF NOT EXISTS amount_per_winner_bigint BIGINT;
ALTER TABLE giveaway_winners ADD COLUMN IF NOT EXISTS amount_bigint BIGINT;

-- Populate temporary columns by converting NUMERIC(12,2) -> BIGINT (multiply by 100)
UPDATE giveaways SET total_budget_bigint = ROUND(total_budget * 100)::BIGINT WHERE total_budget IS NOT NULL;
UPDATE giveaways SET amount_per_winner_bigint = ROUND(amount_per_winner * 100)::BIGINT WHERE amount_per_winner IS NOT NULL;
UPDATE giveaway_winners SET amount_bigint = ROUND(amount * 100)::BIGINT WHERE amount IS NOT NULL;

-- Verify population (optional - left for DB admin)

-- Drop old columns and rename
ALTER TABLE giveaways DROP COLUMN IF EXISTS total_budget;
ALTER TABLE giveaways DROP COLUMN IF EXISTS amount_per_winner;
ALTER TABLE giveaway_winners DROP COLUMN IF EXISTS amount;

ALTER TABLE giveaways RENAME COLUMN total_budget_bigint TO total_budget;
ALTER TABLE giveaways RENAME COLUMN amount_per_winner_bigint TO amount_per_winner;
ALTER TABLE giveaway_winners RENAME COLUMN amount_bigint TO amount;

COMMIT;
