-- ============================================================================
-- Sets (coleções/expansões de Pokémon TCG)
-- ============================================================================
CREATE TABLE card_sets (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code            VARCHAR(16)  NOT NULL UNIQUE,        -- ex.: "sv7", "sm10", "s12a"
    name            VARCHAR(128) NOT NULL,
    series          VARCHAR(64),                          -- ex.: "Scarlet & Violet"
    language        VARCHAR(8)   NOT NULL,                -- "pt", "en", "jp"
    release_date    DATE,
    total_cards     INTEGER,
    image_url       TEXT,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_card_sets_language ON card_sets(language);
CREATE INDEX idx_card_sets_release  ON card_sets(release_date DESC);

-- ============================================================================
-- Cartas (uma linha por ID de carta dentro de um set; variantes ficam abaixo)
-- ============================================================================
CREATE TABLE cards (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    set_id          UUID NOT NULL REFERENCES card_sets(id) ON DELETE RESTRICT,
    number          VARCHAR(16) NOT NULL,                 -- "025/198", "SV-P 099"
    name            CITEXT      NOT NULL,                 -- busca case-insensitive
    rarity          VARCHAR(32),                          -- "Common", "Rare Holo", "Illustration Rare"
    supertype       VARCHAR(16),                          -- "Pokémon", "Trainer", "Energy"
    subtypes        TEXT[],                               -- ["Stage 1", "EX"]
    types           TEXT[],                               -- ["Fire", "Water"]
    hp              INTEGER,
    illustrator     VARCHAR(128),
    image_small_url TEXT,
    image_large_url TEXT,
    external_ids    JSONB NOT NULL DEFAULT '{}'::jsonb,   -- {"tcgplayer": "...", "pokemon_tcg_io": "..."}
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (set_id, number)
);

CREATE INDEX idx_cards_name_trgm    ON cards USING GIN (name gin_trgm_ops);
CREATE INDEX idx_cards_set          ON cards(set_id);
CREATE INDEX idx_cards_rarity       ON cards(rarity);
CREATE INDEX idx_cards_external_ids ON cards USING GIN (external_ids);

-- ============================================================================
-- Variantes da carta — diferenciar Master Ball / Poke Ball Mirror / Holo / Reverse Holo
-- Toda formação de preço acontece a nível de variante.
-- ============================================================================
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

CREATE TABLE card_variants (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    card_id         UUID NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    finish          variant_finish NOT NULL,
    label           VARCHAR(64),                          -- rótulo legível adicional, opcional
    is_promo        BOOLEAN NOT NULL DEFAULT FALSE,
    notes           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- UNIQUE com COALESCE precisa ser índice (Postgres não aceita expressão em
-- UNIQUE table constraint). Trata label NULL como string vazia, garantindo
-- "no máximo uma variante (card, finish) sem label".
CREATE UNIQUE INDEX idx_card_variants_natural_key
    ON card_variants (card_id, finish, COALESCE(label, ''));

CREATE INDEX idx_card_variants_card    ON card_variants(card_id);
CREATE INDEX idx_card_variants_finish  ON card_variants(finish);
