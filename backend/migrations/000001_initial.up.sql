-- =============================================================================
-- Migration inicial — schema completo do MercadoTCG.
-- Representa o estado final de todas as migrações incrementais anteriores
-- (000001–000021). A partir desta migration, todas as alterações são incrementais.
-- =============================================================================

-- ---------------------------------------------------------------------------
-- Extensões
-- ---------------------------------------------------------------------------
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "citext";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";

-- ---------------------------------------------------------------------------
-- ENUMs
-- ---------------------------------------------------------------------------
CREATE TYPE variant_finish AS ENUM (
    'normal',
    'holo',
    'reverse_holo',
    'master_ball_mirror',
    'poke_ball_mirror',
    'cosmos_holo',
    'galaxy_holo',
    'textured',
    'gold_etched',
    'first_edition',
    'shadowless',
    'unlimited'
);

CREATE TYPE card_condition AS ENUM ('NM', 'LP', 'MP', 'HP', 'DMG', 'GRADED');

CREATE TYPE price_source AS ENUM (
    'mercadotcg',
    'mercadolivre',
    'shopee',
    'tcgplayer',
    'cardmarket',
    'ebay',
    'yahoo_auctions_jp',
    'ligapokemon',
    'manual'
);

CREATE TYPE currency_code AS ENUM ('BRL', 'USD', 'JPY', 'EUR');

CREATE TYPE observation_kind AS ENUM ('sale', 'listing', 'bid');

CREATE TYPE listing_status AS ENUM (
    'draft', 'active', 'reserved', 'sold', 'cancelled', 'expired'
);

CREATE TYPE stock_movement_kind AS ENUM (
    'purchase', 'sale', 'adjustment', 'transfer_in', 'transfer_out',
    'reservation', 'release', 'loss'
);

CREATE TYPE platform_role AS ENUM ('platform_admin', 'user');
CREATE TYPE store_role    AS ENUM ('admin', 'stock_manager', 'viewer');

CREATE TYPE document_type   AS ENUM ('cpf', 'cnpj');
CREATE TYPE document_status AS ENUM ('pending', 'auto_verified', 'manually_verified');

-- ---------------------------------------------------------------------------
-- Função utilitária — atualizar updated_at automaticamente
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ---------------------------------------------------------------------------
-- card_series
-- ---------------------------------------------------------------------------
CREATE TABLE card_series (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT        NOT NULL,
    name_pt    TEXT,
    tcg        VARCHAR(32) NOT NULL DEFAULT 'pokemon',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_card_series_name_tcg UNIQUE (name, tcg)
);

CREATE INDEX idx_card_series_tcg         ON card_series (tcg);
CREATE INDEX idx_card_series_name_pt_trgm
    ON card_series USING GIN (name_pt gin_trgm_ops)
    WHERE name_pt IS NOT NULL;

-- ---------------------------------------------------------------------------
-- card_sets
-- ---------------------------------------------------------------------------
CREATE TABLE card_sets (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    code          VARCHAR(16) NOT NULL UNIQUE,
    name          VARCHAR(128) NOT NULL,
    series        VARCHAR(64),
    series_id     UUID        REFERENCES card_series(id),
    name_pt       TEXT,
    name_en       TEXT,
    tcg           VARCHAR(32) NOT NULL DEFAULT 'pokemon',
    language      VARCHAR(8)  NOT NULL,
    release_date  DATE,
    total_cards   INTEGER,
    printed_total INTEGER,
    image_url     TEXT,
    symbol_url    TEXT,
    import_source VARCHAR(32) NOT NULL DEFAULT 'tcgdex_legacy',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_card_sets_tcg CHECK (
        tcg IN ('pokemon', 'pokemon-pocket', 'magic', 'yugioh', 'onepiece', 'lorcana', 'fab')
    )
);

CREATE INDEX idx_card_sets_language  ON card_sets (language);
CREATE INDEX idx_card_sets_release   ON card_sets (release_date DESC);
CREATE INDEX idx_card_sets_tcg       ON card_sets (tcg);
CREATE INDEX idx_card_sets_series_id ON card_sets (series_id);

CREATE TRIGGER trg_card_sets_updated_at
    BEFORE UPDATE ON card_sets
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- cards
-- ---------------------------------------------------------------------------
CREATE TABLE cards (
    id               UUID    PRIMARY KEY DEFAULT gen_random_uuid(),
    set_id           UUID    NOT NULL REFERENCES card_sets(id) ON DELETE RESTRICT,
    collector_number TEXT    NOT NULL DEFAULT '',
    name             CITEXT  NOT NULL,
    name_pt          TEXT,
    rarity           VARCHAR(32),
    supertype        VARCHAR(16),
    subtypes         TEXT[],
    types            TEXT[],
    hp               INTEGER,
    illustrator      VARCHAR(128),
    image_small_url  TEXT,
    image_large_url  TEXT,
    image_url_pt     TEXT,
    external_ids     JSONB NOT NULL DEFAULT '{}'::jsonb,
    import_source    VARCHAR(32) NOT NULL DEFAULT 'tcgdex_legacy',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_cards_set_collector_number UNIQUE (set_id, collector_number)
);

CREATE INDEX idx_cards_set           ON cards (set_id);
CREATE INDEX idx_cards_rarity        ON cards (rarity);
CREATE INDEX idx_cards_external_ids  ON cards USING GIN (external_ids);
CREATE INDEX idx_cards_name_trgm     ON cards USING GIN (name gin_trgm_ops);
CREATE INDEX idx_cards_name_pt_trgm  ON cards USING GIN (name_pt gin_trgm_ops)
    WHERE name_pt IS NOT NULL;
CREATE INDEX idx_cards_collector_number
    ON cards USING hash (collector_number)
    WHERE collector_number <> '';

CREATE TRIGGER trg_cards_updated_at
    BEFORE UPDATE ON cards
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- card_variants
-- ---------------------------------------------------------------------------
CREATE TABLE card_variants (
    id         UUID           PRIMARY KEY DEFAULT gen_random_uuid(),
    card_id    UUID           NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    finish     variant_finish NOT NULL,
    label      VARCHAR(64),
    is_promo   BOOLEAN        NOT NULL DEFAULT FALSE,
    notes      TEXT,
    created_at TIMESTAMPTZ    NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_card_variants_natural_key
    ON card_variants (card_id, finish, COALESCE(label, ''));
CREATE INDEX idx_card_variants_card   ON card_variants (card_id);
CREATE INDEX idx_card_variants_finish ON card_variants (finish);

-- ---------------------------------------------------------------------------
-- price_history (particionada por trimestre)
-- ---------------------------------------------------------------------------
CREATE TABLE price_history (
    id             UUID           NOT NULL DEFAULT gen_random_uuid(),
    variant_id     UUID           NOT NULL REFERENCES card_variants(id) ON DELETE CASCADE,
    condition      card_condition NOT NULL,
    grade          VARCHAR(8),
    source         price_source   NOT NULL,
    kind           observation_kind NOT NULL,
    price_original NUMERIC(14, 2) NOT NULL CHECK (price_original >= 0),
    currency       currency_code  NOT NULL,
    price_brl      NUMERIC(14, 2) NOT NULL CHECK (price_brl >= 0),
    fx_rate_used   NUMERIC(18, 8) NOT NULL,
    quantity       INTEGER        NOT NULL DEFAULT 1 CHECK (quantity > 0),
    external_url   TEXT,
    external_id    VARCHAR(128),
    seller_country VARCHAR(2),
    observed_at    TIMESTAMPTZ    NOT NULL,
    ingested_at    TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, observed_at),
    UNIQUE (source, external_id, observed_at)
) PARTITION BY RANGE (observed_at);

CREATE INDEX idx_price_history_variant_time
    ON price_history (variant_id, observed_at DESC);
CREATE INDEX idx_price_history_observed_at_brin
    ON price_history USING BRIN (observed_at);
CREATE INDEX idx_price_history_variant_source_kind
    ON price_history (variant_id, source, kind, observed_at DESC);

CREATE TABLE price_history_2025_q1 PARTITION OF price_history FOR VALUES FROM ('2025-01-01') TO ('2025-04-01');
CREATE TABLE price_history_2025_q2 PARTITION OF price_history FOR VALUES FROM ('2025-04-01') TO ('2025-07-01');
CREATE TABLE price_history_2025_q3 PARTITION OF price_history FOR VALUES FROM ('2025-07-01') TO ('2025-10-01');
CREATE TABLE price_history_2025_q4 PARTITION OF price_history FOR VALUES FROM ('2025-10-01') TO ('2026-01-01');
CREATE TABLE price_history_2026_q1 PARTITION OF price_history FOR VALUES FROM ('2026-01-01') TO ('2026-04-01');
CREATE TABLE price_history_2026_q2 PARTITION OF price_history FOR VALUES FROM ('2026-04-01') TO ('2026-07-01');
CREATE TABLE price_history_2026_q3 PARTITION OF price_history FOR VALUES FROM ('2026-07-01') TO ('2026-10-01');
CREATE TABLE price_history_2026_q4 PARTITION OF price_history FOR VALUES FROM ('2026-10-01') TO ('2027-01-01');
CREATE TABLE price_history_2027_q1 PARTITION OF price_history FOR VALUES FROM ('2027-01-01') TO ('2027-04-01');
CREATE TABLE price_history_2027_q2 PARTITION OF price_history FOR VALUES FROM ('2027-04-01') TO ('2027-07-01');
CREATE TABLE price_history_2027_q3 PARTITION OF price_history FOR VALUES FROM ('2027-07-01') TO ('2027-10-01');
CREATE TABLE price_history_2027_q4 PARTITION OF price_history FOR VALUES FROM ('2027-10-01') TO ('2028-01-01');

-- ---------------------------------------------------------------------------
-- price_daily
-- ---------------------------------------------------------------------------
CREATE TABLE price_daily (
    variant_id     UUID           NOT NULL REFERENCES card_variants(id) ON DELETE CASCADE,
    condition      card_condition NOT NULL,
    source         price_source   NOT NULL,
    day            DATE           NOT NULL,
    sales_count    INTEGER        NOT NULL,
    listings_count INTEGER        NOT NULL,
    sale_min       NUMERIC(14, 2),
    sale_max       NUMERIC(14, 2),
    sale_avg       NUMERIC(14, 2),
    sale_median    NUMERIC(14, 2),
    sale_p25       NUMERIC(14, 2),
    sale_p75       NUMERIC(14, 2),
    listing_min    NUMERIC(14, 2),
    listing_avg    NUMERIC(14, 2),
    last_updated   TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    PRIMARY KEY (variant_id, condition, source, day)
);

CREATE INDEX idx_price_daily_variant_day ON price_daily (variant_id, day DESC);

-- ---------------------------------------------------------------------------
-- forex_rates
-- ---------------------------------------------------------------------------
CREATE TABLE forex_rates (
    id          UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    currency    currency_code NOT NULL,
    rate_to_brl NUMERIC(18, 8) NOT NULL CHECK (rate_to_brl > 0),
    quoted_at   DATE          NOT NULL,
    source      VARCHAR(64)   NOT NULL,
    created_at  TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    UNIQUE (currency, quoted_at, source)
);

CREATE INDEX idx_forex_currency_date ON forex_rates (currency, quoted_at DESC);

-- ---------------------------------------------------------------------------
-- users
-- ---------------------------------------------------------------------------
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

CREATE INDEX idx_users_email ON users (email);

CREATE TRIGGER trg_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE user_oauth_providers (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider     VARCHAR(32) NOT NULL,
    provider_uid VARCHAR(256) NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (provider, provider_uid)
);

CREATE INDEX idx_oauth_user ON user_oauth_providers (user_id);

CREATE TABLE email_verification_tokens (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT        NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ev_user    ON email_verification_tokens (user_id);
CREATE INDEX idx_ev_expires ON email_verification_tokens (expires_at);

CREATE TABLE password_reset_tokens (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT        NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_pr_user    ON password_reset_tokens (user_id);
CREATE INDEX idx_pr_expires ON password_reset_tokens (expires_at);

CREATE TABLE refresh_tokens (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT        NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_rt_user    ON refresh_tokens (user_id);
CREATE INDEX idx_rt_hash    ON refresh_tokens (token_hash);
CREATE INDEX idx_rt_expires ON refresh_tokens (expires_at);

-- ---------------------------------------------------------------------------
-- stores
-- ---------------------------------------------------------------------------
CREATE TABLE stores (
    id                   UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id             UUID            NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    name                 VARCHAR(128)    NOT NULL,
    slug                 VARCHAR(64)     NOT NULL UNIQUE,
    description          TEXT,
    logo_url             TEXT,
    is_active            BOOLEAN         NOT NULL DEFAULT TRUE,
    document_type        document_type,
    document_number      VARCHAR(14),
    document_status      document_status NOT NULL DEFAULT 'pending',
    legal_name           VARCHAR(255),
    document_verified_at TIMESTAMPTZ,
    document_verified_by UUID            REFERENCES users(id) ON DELETE SET NULL,
    trade_name           VARCHAR(128),
    phone                VARCHAR(32),
    address_zip          VARCHAR(8),
    address_street       TEXT,
    address_number       VARCHAR(16),
    address_complement   VARCHAR(128),
    address_neighborhood VARCHAR(128),
    address_city         VARCHAR(128),
    address_state        CHAR(2),
    address_country      CHAR(2)         NOT NULL DEFAULT 'BR',
    created_at           TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ     NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_stores_document
    ON stores (document_type, document_number)
    WHERE document_number IS NOT NULL;
CREATE INDEX idx_stores_owner ON stores (owner_id);

CREATE TRIGGER trg_stores_updated_at
    BEFORE UPDATE ON stores
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- listings
-- ---------------------------------------------------------------------------
CREATE TABLE listings (
    id           UUID           PRIMARY KEY DEFAULT gen_random_uuid(),
    seller_id    UUID           NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    variant_id   UUID           NOT NULL REFERENCES card_variants(id) ON DELETE RESTRICT,
    condition    card_condition NOT NULL,
    grade        VARCHAR(8),
    quantity     INTEGER        NOT NULL CHECK (quantity > 0),
    price_brl    NUMERIC(14, 2) NOT NULL CHECK (price_brl >= 0),
    status       listing_status NOT NULL DEFAULT 'draft',
    description  TEXT,
    photos       TEXT[]         NOT NULL DEFAULT '{}',
    published_at TIMESTAMPTZ,
    sold_at      TIMESTAMPTZ,
    created_at   TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ    NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_listings_variant_status_price
    ON listings (variant_id, status, price_brl)
    WHERE status = 'active';
CREATE INDEX idx_listings_seller       ON listings (seller_id);
CREATE INDEX idx_listings_published_at ON listings (published_at DESC);

CREATE TRIGGER trg_listings_updated_at
    BEFORE UPDATE ON listings
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- store_members
-- ---------------------------------------------------------------------------
CREATE TABLE store_members (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    store_id   UUID        NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    user_id    UUID        NOT NULL REFERENCES users(id)  ON DELETE CASCADE,
    role       store_role  NOT NULL,
    invited_by UUID        REFERENCES users(id) ON DELETE SET NULL,
    joined_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (store_id, user_id)
);

CREATE INDEX idx_sm_store ON store_members (store_id);
CREATE INDEX idx_sm_user  ON store_members (user_id);

-- ---------------------------------------------------------------------------
-- stock_items
-- ---------------------------------------------------------------------------
CREATE TABLE stock_items (
    id               UUID           PRIMARY KEY DEFAULT gen_random_uuid(),
    store_id         UUID           NOT NULL REFERENCES stores(id)        ON DELETE CASCADE,
    variant_id       UUID           NOT NULL REFERENCES card_variants(id) ON DELETE RESTRICT,
    condition        card_condition NOT NULL,
    language         VARCHAR(8)     NOT NULL,
    grade            VARCHAR(16),
    quantity         INTEGER        NOT NULL DEFAULT 0 CHECK (quantity >= 0),
    cost_avg_brl     NUMERIC(14, 2),
    asking_price_brl NUMERIC(14, 2),
    sku              VARCHAR(64),
    notes            TEXT,
    created_at       TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ    NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_stock_items_natural_key
    ON stock_items (store_id, variant_id, condition, language, COALESCE(grade, ''));
CREATE INDEX idx_stock_items_store        ON stock_items (store_id);
CREATE INDEX idx_stock_items_variant      ON stock_items (variant_id);
CREATE INDEX idx_stock_items_store_active ON stock_items (store_id) WHERE quantity > 0;
CREATE UNIQUE INDEX idx_stock_items_store_sku
    ON stock_items (store_id, sku) WHERE sku IS NOT NULL;

CREATE TRIGGER trg_stock_items_updated_at
    BEFORE UPDATE ON stock_items
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- stock_movements
-- ---------------------------------------------------------------------------
CREATE TABLE stock_movements (
    id             UUID                PRIMARY KEY DEFAULT gen_random_uuid(),
    stock_item_id  UUID                NOT NULL REFERENCES stock_items(id) ON DELETE RESTRICT,
    kind           stock_movement_kind NOT NULL,
    quantity_delta INTEGER             NOT NULL CHECK (quantity_delta <> 0),
    unit_price_brl NUMERIC(14, 2),
    reference_type VARCHAR(32),
    reference_id   VARCHAR(128),
    notes          TEXT,
    occurred_at    TIMESTAMPTZ         NOT NULL DEFAULT NOW(),
    created_at     TIMESTAMPTZ         NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_stock_movements_item ON stock_movements (stock_item_id, occurred_at DESC);
CREATE INDEX idx_stock_movements_kind ON stock_movements (kind, occurred_at DESC);

-- ---------------------------------------------------------------------------
-- external_card_refs
-- ---------------------------------------------------------------------------
CREATE TABLE external_card_refs (
    id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    variant_id   UUID         NOT NULL REFERENCES card_variants(id) ON DELETE CASCADE,
    source       price_source NOT NULL,
    external_id  VARCHAR(128) NOT NULL,
    external_url TEXT,
    language     VARCHAR(8)   NOT NULL,
    confidence   SMALLINT     NOT NULL DEFAULT 100 CHECK (confidence BETWEEN 0 AND 100),
    raw_title    TEXT,
    needs_review BOOLEAN      NOT NULL DEFAULT FALSE,
    matched_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (source, external_id)
);

CREATE INDEX idx_external_refs_variant      ON external_card_refs (variant_id);
CREATE INDEX idx_external_refs_source_lang  ON external_card_refs (source, language);
CREATE INDEX idx_external_refs_needs_review ON external_card_refs (needs_review)
    WHERE needs_review = TRUE;

-- ---------------------------------------------------------------------------
-- match_candidates
-- ---------------------------------------------------------------------------
CREATE TABLE match_candidates (
    id                        UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    source                    price_source NOT NULL,
    external_id               TEXT         NOT NULL,
    raw_title                 TEXT         NOT NULL,
    raw_number                TEXT,
    raw_set_code              TEXT,
    best_candidate_variant_id UUID         REFERENCES card_variants(id),
    best_score                INT          NOT NULL DEFAULT 0,
    created_at                TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    reviewed_by               UUID         REFERENCES users(id),
    reviewed_at               TIMESTAMPTZ,
    resolution                TEXT
);

CREATE UNIQUE INDEX idx_match_candidates_pending
    ON match_candidates (source, external_id)
    WHERE reviewed_at IS NULL;

-- ---------------------------------------------------------------------------
-- store_audit_log
-- ---------------------------------------------------------------------------
CREATE TABLE store_audit_log (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    store_id    UUID        NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    changed_by  UUID        NOT NULL REFERENCES users(id),
    change_type VARCHAR(64) NOT NULL DEFAULT 'update',
    changes     JSONB       NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_store_audit_log_store ON store_audit_log (store_id, created_at DESC);

-- ---------------------------------------------------------------------------
-- Seed: usuário administrador padrão da plataforma
-- ---------------------------------------------------------------------------
INSERT INTO users (email, display_name, password_hash, platform_role, email_verified_at)
VALUES (
    'gustavojucoski@gmail.com',
    'Gustavo Jucoski',
    crypt('ewq9brd5gan2dzf@FZD', gen_salt('bf', 12)),
    'platform_admin',
    NOW()
)
ON CONFLICT (email) DO NOTHING;
