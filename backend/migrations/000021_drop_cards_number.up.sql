-- Add UNIQUE constraint on (set_id, collector_number) before dropping number.
-- The old UNIQUE (set_id, number) was the natural key; collector_number takes its place.
-- Dropping the number column implicitly drops the old UNIQUE constraint.
ALTER TABLE cards ADD CONSTRAINT uq_cards_set_collector_number UNIQUE (set_id, collector_number);

ALTER TABLE cards DROP COLUMN number;
