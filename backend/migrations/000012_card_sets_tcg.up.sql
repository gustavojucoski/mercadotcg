ALTER TABLE card_sets ADD COLUMN IF NOT EXISTS tcg VARCHAR(32) NOT NULL DEFAULT 'pokemon';

ALTER TABLE card_sets
    ADD CONSTRAINT chk_card_sets_tcg
    CHECK (tcg IN ('pokemon', 'magic', 'yugioh', 'onepiece', 'lorcana', 'fab'));

CREATE INDEX IF NOT EXISTS idx_card_sets_tcg ON card_sets (tcg);
