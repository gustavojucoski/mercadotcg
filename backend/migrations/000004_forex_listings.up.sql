-- ============================================================================
-- Cotações de câmbio (1 unidade da moeda em BRL)
-- Snapshot diário; o scraper/serviço usa a cotação mais recente <= observed_at
-- ao normalizar price_brl em price_history.
-- ============================================================================
CREATE TABLE forex_rates (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    currency        currency_code NOT NULL,
    rate_to_brl     NUMERIC(18, 8) NOT NULL CHECK (rate_to_brl > 0),
    quoted_at       DATE NOT NULL,
    source          VARCHAR(64) NOT NULL,        -- "bcb", "openexchangerates", "manual"
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (currency, quoted_at, source)
);

CREATE INDEX idx_forex_currency_date ON forex_rates (currency, quoted_at DESC);

-- ============================================================================
-- Listings ativos no marketplace MercadoTCG
-- (vendas concretizadas viram price_history com source='mercadotcg', kind='sale')
-- ============================================================================
CREATE TYPE listing_status AS ENUM (
    'draft',
    'active',
    'reserved',
    'sold',
    'cancelled',
    'expired'
);

CREATE TABLE listings (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    seller_id       UUID NOT NULL,                       -- FK para users (criada em migration futura)
    variant_id      UUID NOT NULL REFERENCES card_variants(id) ON DELETE RESTRICT,
    condition       card_condition NOT NULL,
    grade           VARCHAR(8),
    quantity        INTEGER NOT NULL CHECK (quantity > 0),
    price_brl       NUMERIC(14, 2) NOT NULL CHECK (price_brl >= 0),
    status          listing_status NOT NULL DEFAULT 'draft',
    description     TEXT,
    photos          TEXT[] NOT NULL DEFAULT '{}',
    published_at    TIMESTAMPTZ,
    sold_at         TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_listings_variant_status_price
    ON listings (variant_id, status, price_brl)
    WHERE status = 'active';

CREATE INDEX idx_listings_seller       ON listings (seller_id);
CREATE INDEX idx_listings_published_at ON listings (published_at DESC);

CREATE TRIGGER trg_listings_updated_at
    BEFORE UPDATE ON listings
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
