-- Migration 000006: tabela de usuários, OAuth providers, tokens e membros de loja.
-- Também resolve as FKs pendentes em listings e stores.

CREATE TYPE platform_role AS ENUM ('platform_admin', 'user');
CREATE TYPE store_role    AS ENUM ('admin', 'stock_manager', 'viewer');

-- ----------------------------------------------------------------------------
-- users — identidade central da plataforma. Sem vínculo a loja específica.
-- password_hash NULL para usuários OAuth-only.
-- email_verified_at NULL = conta não ativada (login bloqueado).
-- ----------------------------------------------------------------------------
CREATE TABLE users (
    id                UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    email             CITEXT        NOT NULL UNIQUE,
    display_name      VARCHAR(128)  NOT NULL,
    avatar_url        TEXT,
    password_hash     TEXT,
    platform_role     platform_role NOT NULL DEFAULT 'user',
    email_verified_at TIMESTAMPTZ,
    is_active         BOOLEAN       NOT NULL DEFAULT TRUE,
    created_at        TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_users_email ON users(email);

CREATE TRIGGER trg_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ----------------------------------------------------------------------------
-- user_oauth_providers — um usuário pode ter N provedores OAuth.
-- ----------------------------------------------------------------------------
CREATE TABLE user_oauth_providers (
    id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider     VARCHAR(32)  NOT NULL,
    provider_uid VARCHAR(256) NOT NULL,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (provider, provider_uid)
);

CREATE INDEX idx_oauth_user ON user_oauth_providers(user_id);

-- ----------------------------------------------------------------------------
-- email_verification_tokens — hex 32 bytes, one-time-use, 24h TTL.
-- Armazenamos o SHA-256 do token enviado ao usuário, não o token em si.
-- ----------------------------------------------------------------------------
CREATE TABLE email_verification_tokens (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT        NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ev_user    ON email_verification_tokens(user_id);
CREATE INDEX idx_ev_expires ON email_verification_tokens(expires_at);

-- ----------------------------------------------------------------------------
-- password_reset_tokens — hex 32 bytes, one-time-use, 1h TTL.
-- ----------------------------------------------------------------------------
CREATE TABLE password_reset_tokens (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT        NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_pr_user    ON password_reset_tokens(user_id);
CREATE INDEX idx_pr_expires ON password_reset_tokens(expires_at);

-- ----------------------------------------------------------------------------
-- refresh_tokens — armazenamos o SHA-256 do token opaco enviado ao cliente.
-- revoked_at preenchido em logout.
-- ----------------------------------------------------------------------------
CREATE TABLE refresh_tokens (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT        NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    revoked_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_rt_user    ON refresh_tokens(user_id);
CREATE INDEX idx_rt_hash    ON refresh_tokens(token_hash);
CREATE INDEX idx_rt_expires ON refresh_tokens(expires_at);

-- ----------------------------------------------------------------------------
-- store_members — vínculo usuário ↔ loja com papel (RBAC scoped por loja).
-- ----------------------------------------------------------------------------
CREATE TABLE store_members (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    store_id    UUID        NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    user_id     UUID        NOT NULL REFERENCES users(id)  ON DELETE CASCADE,
    role        store_role  NOT NULL,
    invited_by  UUID        REFERENCES users(id)           ON DELETE SET NULL,
    joined_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (store_id, user_id)
);

CREATE INDEX idx_sm_store ON store_members(store_id);
CREATE INDEX idx_sm_user  ON store_members(user_id);

-- ----------------------------------------------------------------------------
-- Resolve FKs pendentes de migrations anteriores
-- ----------------------------------------------------------------------------
ALTER TABLE listings ADD CONSTRAINT fk_listings_seller
    FOREIGN KEY (seller_id) REFERENCES users(id) ON DELETE RESTRICT;

ALTER TABLE stores ADD CONSTRAINT fk_stores_owner
    FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE RESTRICT;
