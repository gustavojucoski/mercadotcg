-- Revert migration 000016: TCGDex support

DROP INDEX IF EXISTS idx_card_series_name_pt_trgm;
DROP INDEX IF EXISTS idx_cards_name_pt_trgm;

-- Restore the original CHECK constraint (without 'pocket').
ALTER TABLE card_sets DROP CONSTRAINT IF EXISTS chk_card_sets_tcg;
ALTER TABLE card_sets ADD CONSTRAINT chk_card_sets_tcg
    CHECK (tcg IN ('pokemon', 'magic', 'yugioh', 'onepiece', 'lorcana', 'fab'));

ALTER TABLE card_sets DROP COLUMN IF EXISTS symbol_url;
