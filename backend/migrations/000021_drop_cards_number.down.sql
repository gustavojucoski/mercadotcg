ALTER TABLE cards ADD COLUMN number VARCHAR(16);
-- Restaura a partir do collector_number
UPDATE cards SET number = collector_number WHERE number IS NULL;
-- Restaura a UNIQUE constraint original
ALTER TABLE cards ADD CONSTRAINT cards_set_id_number_key UNIQUE (set_id, number);
-- Remove a nova constraint
ALTER TABLE cards DROP CONSTRAINT uq_cards_set_collector_number;
