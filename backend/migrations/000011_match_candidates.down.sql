DROP TABLE IF EXISTS match_candidates;

DROP INDEX IF EXISTS idx_external_refs_needs_review;

ALTER TABLE external_card_refs
    DROP COLUMN IF EXISTS needs_review;
