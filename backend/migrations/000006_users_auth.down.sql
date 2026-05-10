ALTER TABLE stores   DROP CONSTRAINT IF EXISTS fk_stores_owner;
ALTER TABLE listings DROP CONSTRAINT IF EXISTS fk_listings_seller;

DROP TABLE IF EXISTS store_members;
DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS password_reset_tokens;
DROP TABLE IF EXISTS email_verification_tokens;
DROP TABLE IF EXISTS user_oauth_providers;

DROP TRIGGER IF EXISTS trg_users_updated_at ON users;
DROP TABLE IF EXISTS users;

DROP TYPE IF EXISTS store_role;
DROP TYPE IF EXISTS platform_role;
