-- ============================================================================
-- Tipos enumerados de pricing
-- ============================================================================
CREATE TYPE card_condition AS ENUM (
    'NM',     -- Near Mint
    'LP',     -- Lightly Played
    'MP',     -- Moderately Played
    'HP',     -- Heavily Played
    'DMG',    -- Damaged
    'GRADED' -- carta gradeada (PSA, Beckett, CGC, etc.)
);

CREATE TYPE price_source AS ENUM (
    'mercadotcg',         -- vendas reais na nossa plataforma
    'mercadolivre',
    'shopee',
    'tcgplayer',
    'cardmarket',
    'ebay',
    'yahoo_auctions_jp',
    'manual'
);

CREATE TYPE currency_code AS ENUM ('BRL', 'USD', 'JPY', 'EUR');

CREATE TYPE observation_kind AS ENUM (
    'sale',       -- venda concluída (preferencial)
    'listing',    -- anúncio ativo (sinal mais fraco)
    'bid'         -- lance em leilão
);

-- ============================================================================
-- price_history — observações brutas de preço
--
-- Volume esperado: dezenas de milhões de linhas. Particionada por mês em
-- observed_at para permitir DROP de partições antigas e index pruning.
-- A PK precisa incluir a coluna de partição.
-- ============================================================================
CREATE TABLE price_history (
    id              UUID        NOT NULL DEFAULT gen_random_uuid(),
    variant_id      UUID        NOT NULL REFERENCES card_variants(id) ON DELETE CASCADE,
    condition       card_condition NOT NULL,
    grade           VARCHAR(8),                          -- "PSA 10", "BGS 9.5" — null quando condition != GRADED
    source          price_source NOT NULL,
    kind            observation_kind NOT NULL,

    -- Preço na moeda original (auditável)
    price_original  NUMERIC(14, 2) NOT NULL CHECK (price_original >= 0),
    currency        currency_code NOT NULL,

    -- Preço normalizado em BRL no momento da observação
    price_brl       NUMERIC(14, 2) NOT NULL CHECK (price_brl >= 0),
    fx_rate_used    NUMERIC(18, 8) NOT NULL,             -- cotação aplicada (1 unidade da moeda em BRL)

    quantity        INTEGER NOT NULL DEFAULT 1 CHECK (quantity > 0),
    external_url    TEXT,
    external_id     VARCHAR(128),                         -- id do anúncio/venda na fonte
    seller_country  VARCHAR(2),                           -- ISO 3166-1 alpha-2
    observed_at     TIMESTAMPTZ NOT NULL,
    ingested_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (id, observed_at),
    UNIQUE (source, external_id, observed_at)
) PARTITION BY RANGE (observed_at);

-- Índice "última observação por variante" — BTree composto.
CREATE INDEX idx_price_history_variant_time
    ON price_history (variant_id, observed_at DESC);

-- Pesquisas por janela de tempo: BRIN é ~1000x menor que BTree em séries temporais.
CREATE INDEX idx_price_history_observed_at_brin
    ON price_history USING BRIN (observed_at);

-- Pesquisas filtradas por fonte / condição / kind dentro de uma variante.
CREATE INDEX idx_price_history_variant_source_kind
    ON price_history (variant_id, source, kind, observed_at DESC);

-- Partições iniciais: 2025-01 .. 2026-12 (cobrir histórico próximo + futuro)
-- Em produção, criar via job mensal antes do mês começar.
CREATE TABLE price_history_2025_q1 PARTITION OF price_history
    FOR VALUES FROM ('2025-01-01') TO ('2025-04-01');
CREATE TABLE price_history_2025_q2 PARTITION OF price_history
    FOR VALUES FROM ('2025-04-01') TO ('2025-07-01');
CREATE TABLE price_history_2025_q3 PARTITION OF price_history
    FOR VALUES FROM ('2025-07-01') TO ('2025-10-01');
CREATE TABLE price_history_2025_q4 PARTITION OF price_history
    FOR VALUES FROM ('2025-10-01') TO ('2026-01-01');
CREATE TABLE price_history_2026_q1 PARTITION OF price_history
    FOR VALUES FROM ('2026-01-01') TO ('2026-04-01');
CREATE TABLE price_history_2026_q2 PARTITION OF price_history
    FOR VALUES FROM ('2026-04-01') TO ('2026-07-01');
CREATE TABLE price_history_2026_q3 PARTITION OF price_history
    FOR VALUES FROM ('2026-07-01') TO ('2026-10-01');
CREATE TABLE price_history_2026_q4 PARTITION OF price_history
    FOR VALUES FROM ('2026-10-01') TO ('2027-01-01');

-- ============================================================================
-- price_daily — agregações pré-calculadas por dia
--
-- Esta é a tabela quente para gráficos no front. Atualizada por job diário
-- que faz UPSERT a partir de price_history. Particionar por ano é suficiente
-- (escala muito menor que o raw).
-- ============================================================================
CREATE TABLE price_daily (
    variant_id      UUID NOT NULL REFERENCES card_variants(id) ON DELETE CASCADE,
    condition       card_condition NOT NULL,
    source          price_source NOT NULL,
    day             DATE NOT NULL,

    sales_count     INTEGER NOT NULL,
    listings_count  INTEGER NOT NULL,

    -- estatísticas em BRL — somente sobre vendas (kind = 'sale').
    sale_min        NUMERIC(14, 2),
    sale_max        NUMERIC(14, 2),
    sale_avg        NUMERIC(14, 2),
    sale_median     NUMERIC(14, 2),
    sale_p25        NUMERIC(14, 2),
    sale_p75        NUMERIC(14, 2),

    -- estatística em BRL — sobre anúncios ativos (sinal de "preço pedido").
    listing_min     NUMERIC(14, 2),
    listing_avg     NUMERIC(14, 2),

    last_updated    TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (variant_id, condition, source, day)
);

-- Acesso típico: "série temporal dos últimos N dias para uma variante".
CREATE INDEX idx_price_daily_variant_day
    ON price_daily (variant_id, day DESC);

-- ============================================================================
-- Função utilitária — atualizar updated_at automaticamente
-- (compartilhada por outras tabelas via triggers nas migrations seguintes)
-- ============================================================================
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_card_sets_updated_at
    BEFORE UPDATE ON card_sets
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_cards_updated_at
    BEFORE UPDATE ON cards
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
