-- ============================================================================
-- Adiciona needs_review a external_card_refs
--
-- Matchings automáticos com score 60–84 são marcados para revisão humana
-- antes de influenciar price_history de forma confiante.
-- ============================================================================
ALTER TABLE external_card_refs
    ADD COLUMN IF NOT EXISTS needs_review BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_external_refs_needs_review
    ON external_card_refs (needs_review)
    WHERE needs_review = TRUE;

-- ============================================================================
-- match_candidates — quarentena para observações sem match suficiente
--
-- Quando o scoring não encontra variant algum (score < 60), ou quando a
-- revisão está pendente, a observação cai aqui. Um operador pode aceitar,
-- rejeitar ou adiar a resolução.
-- ============================================================================
CREATE TABLE match_candidates (
    id                        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source                    price_source NOT NULL,
    external_id               TEXT NOT NULL,
    raw_title                 TEXT NOT NULL,
    raw_number                TEXT,
    raw_set_code              TEXT,
    best_candidate_variant_id UUID REFERENCES card_variants(id),
    best_score                INT NOT NULL DEFAULT 0,
    created_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reviewed_by               UUID REFERENCES users(id),
    reviewed_at               TIMESTAMPTZ,
    resolution                TEXT  -- 'accepted', 'rejected', 'deferred'
);

-- Garante unicidade de candidatos pendentes: evita duplicar a mesma
-- observação em quarentena enquanto não há revisão.
CREATE UNIQUE INDEX idx_match_candidates_pending
    ON match_candidates (source, external_id)
    WHERE reviewed_at IS NULL;
