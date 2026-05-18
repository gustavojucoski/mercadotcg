-- Drop em ordem inversa de dependência
DROP TABLE IF EXISTS store_audit_log;
DROP TABLE IF EXISTS match_candidates;
DROP TABLE IF EXISTS external_card_refs;
DROP TABLE IF EXISTS stock_movements;
DROP TABLE IF EXISTS stock_items;
DROP TABLE IF EXISTS store_members;
DROP TABLE IF EXISTS listings;
DROP TABLE IF EXISTS stores;
DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS password_reset_tokens;
DROP TABLE IF EXISTS email_verification_tokens;
DROP TABLE IF EXISTS user_oauth_providers;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS forex_rates;
DROP TABLE IF EXISTS price_daily;
DROP TABLE IF EXISTS price_history;
DROP TABLE IF EXISTS card_variants;
DROP TABLE IF EXISTS cards;
DROP TABLE IF EXISTS card_sets;
DROP TABLE IF EXISTS card_series;

DROP FUNCTION IF EXISTS set_updated_at;

DROP TYPE IF EXISTS document_status;
DROP TYPE IF EXISTS document_type;
DROP TYPE IF EXISTS store_role;
DROP TYPE IF EXISTS platform_role;
DROP TYPE IF EXISTS stock_movement_kind;
DROP TYPE IF EXISTS listing_status;
DROP TYPE IF EXISTS observation_kind;
DROP TYPE IF EXISTS currency_code;
DROP TYPE IF EXISTS price_source;
DROP TYPE IF EXISTS card_condition;
DROP TYPE IF EXISTS variant_finish;

DROP EXTENSION IF EXISTS "pg_trgm";
DROP EXTENSION IF EXISTS "citext";
DROP EXTENSION IF EXISTS "pgcrypto";
