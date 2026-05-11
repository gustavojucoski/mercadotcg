-- Tabela de séries como entidade própria
CREATE TABLE card_series (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT        NOT NULL,
    name_pt    TEXT,
    tcg        VARCHAR(32) NOT NULL DEFAULT 'pokemon',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_card_series_name_tcg UNIQUE (name, tcg)
);

-- Popula card_series a partir dos valores distintos já existentes em card_sets
INSERT INTO card_series (name, tcg)
SELECT DISTINCT series, COALESCE(tcg, 'pokemon')
FROM card_sets
WHERE series IS NOT NULL AND series <> ''
ON CONFLICT (name, tcg) DO NOTHING;

-- FK em card_sets apontando para card_series
ALTER TABLE card_sets
    ADD COLUMN IF NOT EXISTS series_id UUID REFERENCES card_series(id);

-- Preenche series_id para todos os sets que já têm series
UPDATE card_sets cs
SET series_id = (
    SELECT id FROM card_series s
    WHERE s.name = cs.series AND s.tcg = COALESCE(cs.tcg, 'pokemon')
    LIMIT 1
)
WHERE cs.series IS NOT NULL AND cs.series <> '';

-- name_pt em card_sets (nullable — pokemontcg.io não fornece PT-BR)
ALTER TABLE card_sets
    ADD COLUMN IF NOT EXISTS name_pt TEXT;

-- collector_number em cards
ALTER TABLE cards
    ADD COLUMN IF NOT EXISTS collector_number TEXT NOT NULL DEFAULT '';

-- name_pt em cards (nullable)
ALTER TABLE cards
    ADD COLUMN IF NOT EXISTS name_pt TEXT;

-- Índice para lookup por collector_number (igualdade, exclui default vazio)
CREATE INDEX IF NOT EXISTS idx_cards_collector_number
    ON cards USING hash (collector_number)
    WHERE collector_number <> '';

-- Índice para queries "todos os sets desta série"
CREATE INDEX IF NOT EXISTS idx_card_sets_series_id
    ON card_sets (series_id);
