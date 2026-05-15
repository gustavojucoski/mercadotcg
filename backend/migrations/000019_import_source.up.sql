ALTER TABLE card_sets ADD COLUMN IF NOT EXISTS import_source VARCHAR(32) NOT NULL DEFAULT 'tcgdex_legacy';
ALTER TABLE cards     ADD COLUMN IF NOT EXISTS import_source VARCHAR(32) NOT NULL DEFAULT 'tcgdex_legacy';
