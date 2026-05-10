-- ============================================================================
-- Adiciona 'ligapokemon' ao ENUM price_source.
-- Postgres 12+ permite ALTER TYPE … ADD VALUE dentro de transação.
-- ============================================================================
ALTER TYPE price_source ADD VALUE IF NOT EXISTS 'ligapokemon';

-- ============================================================================
-- Lojas — multi-tenant. owner_id ainda sem FK (tabela users virá em fase futura).
-- ============================================================================
CREATE TABLE stores (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id        UUID NOT NULL,                       -- FK para users (pendente)
    name            VARCHAR(128) NOT NULL,
    slug            VARCHAR(64)  NOT NULL UNIQUE,        -- ex.: "mercado-do-gus"
    description     TEXT,
    logo_url        TEXT,
    is_active       BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_stores_owner ON stores(owner_id);

CREATE TRIGGER trg_stores_updated_at
    BEFORE UPDATE ON stores
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ============================================================================
-- Stock items — posição corrente de estoque.
--
-- Granularidade: uma linha por (loja, variante, condição, idioma, grade).
-- Quantidade é cumulativa; entradas e saídas são logadas em stock_movements.
-- cost_avg_brl é o custo médio ponderado (recalculado em cada compra).
-- ============================================================================
CREATE TABLE stock_items (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    store_id          UUID NOT NULL REFERENCES stores(id)        ON DELETE CASCADE,
    variant_id        UUID NOT NULL REFERENCES card_variants(id) ON DELETE RESTRICT,
    condition         card_condition NOT NULL,
    language          VARCHAR(8)   NOT NULL,                     -- 'pt', 'en', 'jp'
    grade             VARCHAR(16),                                -- 'PSA 10', 'BGS 9.5'; NULL se não graded
    quantity          INTEGER      NOT NULL DEFAULT 0 CHECK (quantity >= 0),
    cost_avg_brl      NUMERIC(14, 2),                             -- custo médio ponderado em BRL
    asking_price_brl  NUMERIC(14, 2),                             -- preço alvo opcional
    sku               VARCHAR(64),                                -- SKU interno opcional (único por loja)
    notes             TEXT,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- Natural key como índice único (UNIQUE constraint não aceita COALESCE).
-- Garante "no máximo uma linha por (store, variant, condition, language, grade)";
-- quando grade é NULL trata como string vazia para colidir com outro NULL.
CREATE UNIQUE INDEX idx_stock_items_natural_key
    ON stock_items (store_id, variant_id, condition, language, COALESCE(grade, ''));

CREATE INDEX idx_stock_items_store          ON stock_items(store_id);
CREATE INDEX idx_stock_items_variant        ON stock_items(variant_id);
CREATE INDEX idx_stock_items_store_active   ON stock_items(store_id) WHERE quantity > 0;
CREATE UNIQUE INDEX idx_stock_items_store_sku
    ON stock_items(store_id, sku) WHERE sku IS NOT NULL;

CREATE TRIGGER trg_stock_items_updated_at
    BEFORE UPDATE ON stock_items
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ============================================================================
-- Stock movements — log append-only de toda alteração no estoque.
-- Permite contabilidade (FIFO, custo médio, margem real por venda) sem
-- adicionar carga ao caminho de leitura quente em stock_items.
-- ============================================================================
CREATE TYPE stock_movement_kind AS ENUM (
    'purchase',       -- aquisição (entrada, +qty, registra unit_cost)
    'sale',           -- venda (saída, -qty, registra unit_price)
    'adjustment',     -- ajuste manual (qty +/-)
    'transfer_in',    -- entrada por transferência entre lojas
    'transfer_out',   -- saída por transferência
    'reservation',    -- reserva (não altera qty física; só "available")
    'release',        -- libera reserva
    'loss'            -- perda/dano (saída sem receita)
);

CREATE TABLE stock_movements (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    stock_item_id   UUID NOT NULL REFERENCES stock_items(id) ON DELETE RESTRICT,
    kind            stock_movement_kind NOT NULL,
    quantity_delta  INTEGER NOT NULL CHECK (quantity_delta <> 0),
    unit_price_brl  NUMERIC(14, 2),                       -- cost p/ purchase, price p/ sale
    reference_type  VARCHAR(32),                           -- 'listing', 'invoice', 'manual', 'scraper_match'
    reference_id    VARCHAR(128),
    notes           TEXT,
    occurred_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_stock_movements_item ON stock_movements(stock_item_id, occurred_at DESC);
CREATE INDEX idx_stock_movements_kind ON stock_movements(kind, occurred_at DESC);

-- ============================================================================
-- External card refs — mapeamento (variant_id) ↔ (source, external_id).
--
-- Crítico para o pipeline de scraping: uma observação só vira price_history
-- se houver match aqui. Sem match → fica em quarentena para revisão manual
-- (staging futura, ainda não modelada).
-- ============================================================================
CREATE TABLE external_card_refs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    variant_id      UUID NOT NULL REFERENCES card_variants(id) ON DELETE CASCADE,
    source          price_source NOT NULL,
    external_id     VARCHAR(128) NOT NULL,                 -- id da carta na fonte
    external_url    TEXT,
    language        VARCHAR(8)   NOT NULL,
    confidence      SMALLINT     NOT NULL DEFAULT 100
                    CHECK (confidence BETWEEN 0 AND 100),  -- score do matching
    raw_title       TEXT,                                   -- título capturado (debug)
    matched_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    UNIQUE (source, external_id)
);

CREATE INDEX idx_external_refs_variant     ON external_card_refs(variant_id);
CREATE INDEX idx_external_refs_source_lang ON external_card_refs(source, language);
