DROP TRIGGER IF EXISTS trg_cards_updated_at     ON cards;
DROP TRIGGER IF EXISTS trg_card_sets_updated_at ON card_sets;
DROP FUNCTION IF EXISTS set_updated_at();

DROP TABLE IF EXISTS price_daily;
DROP TABLE IF EXISTS price_history CASCADE;

DROP TYPE IF EXISTS observation_kind;
DROP TYPE IF EXISTS currency_code;
DROP TYPE IF EXISTS price_source;
DROP TYPE IF EXISTS card_condition;
