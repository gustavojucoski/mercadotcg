ALTER TABLE card_sets DROP CONSTRAINT IF EXISTS chk_card_sets_tcg;
DROP INDEX IF EXISTS idx_card_sets_tcg;
ALTER TABLE card_sets DROP COLUMN IF EXISTS tcg;
