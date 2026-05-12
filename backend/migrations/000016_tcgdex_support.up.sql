-- ============================================================================
-- Migration 000016: TCGDex support
--   1. Add symbol_url column to card_sets
--   2. Add 'pocket' to the card_sets.tcg CHECK constraint
--   3. Add GIN trgm indexes for bilingual search on cards.name_pt and card_series.name_pt
-- ============================================================================

-- Symbol URL (set symbol image, different from the set logo).
ALTER TABLE card_sets ADD COLUMN IF NOT EXISTS symbol_url TEXT;

-- Expand TCG CHECK constraint to include 'pocket' (TCG Pocket sets).
-- Must drop and re-add because PostgreSQL doesn't support ALTER CONSTRAINT.
ALTER TABLE card_sets DROP CONSTRAINT IF EXISTS chk_card_sets_tcg;
ALTER TABLE card_sets ADD CONSTRAINT chk_card_sets_tcg
    CHECK (tcg IN ('pokemon', 'magic', 'yugioh', 'onepiece', 'lorcana', 'fab', 'pocket'));

-- GIN trgm index on cards.name_pt for bilingual autocomplete performance.
-- Partial index (WHERE name_pt IS NOT NULL) avoids indexing NULLs.
CREATE INDEX IF NOT EXISTS idx_cards_name_pt_trgm
    ON cards USING GIN (name_pt gin_trgm_ops)
    WHERE name_pt IS NOT NULL;

-- GIN trgm index on card_series.name_pt for bilingual series search.
CREATE INDEX IF NOT EXISTS idx_card_series_name_pt_trgm
    ON card_series USING GIN (name_pt gin_trgm_ops)
    WHERE name_pt IS NOT NULL;
