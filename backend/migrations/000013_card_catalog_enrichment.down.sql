DROP INDEX IF EXISTS idx_card_sets_series_id;
DROP INDEX IF EXISTS idx_cards_collector_number;
ALTER TABLE cards DROP COLUMN IF EXISTS name_pt;
ALTER TABLE cards DROP COLUMN IF EXISTS collector_number;
ALTER TABLE card_sets DROP COLUMN IF EXISTS name_pt;
ALTER TABLE card_sets DROP COLUMN IF EXISTS series_id;
DROP TABLE IF EXISTS card_series;
