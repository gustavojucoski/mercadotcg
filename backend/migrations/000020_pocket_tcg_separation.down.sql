-- Rollback migration 000020: revert Pocket sets back to tcg='pokemon'.

BEGIN;

-- Step 1: Recreate pokemon-flavoured series for the sets being reverted.
INSERT INTO card_series (name, name_pt, tcg, created_at)
SELECT DISTINCT cr.name, cr.name_pt, 'pokemon', cr.created_at
FROM card_series cr
WHERE cr.tcg = 'pokemon-pocket'
ON CONFLICT (name, tcg) DO NOTHING;

-- Step 2: Re-point card_sets.series_id and flip tcg back to 'pokemon'.
UPDATE card_sets cs
SET
    tcg       = 'pokemon',
    series_id = (
        SELECT old_ser.id
        FROM card_series old_ser
        JOIN card_series pkt_ser ON pkt_ser.id = cs.series_id
        WHERE old_ser.name = pkt_ser.name
          AND old_ser.tcg  = 'pokemon'
        LIMIT 1
    ),
    updated_at = now()
WHERE cs.code LIKE 'tcgp-%'
  AND cs.tcg = 'pokemon-pocket';

-- Step 3: Remove pokemon-pocket series that have no sets left.
DELETE FROM card_series
WHERE tcg = 'pokemon-pocket'
  AND id NOT IN (
      SELECT DISTINCT series_id
      FROM card_sets
      WHERE series_id IS NOT NULL
  );

-- Step 4: Restore CHECK constraint with 'pocket' (as migration 000016 defined).
ALTER TABLE card_sets DROP CONSTRAINT IF EXISTS chk_card_sets_tcg;
ALTER TABLE card_sets ADD CONSTRAINT chk_card_sets_tcg
    CHECK (tcg IN ('pokemon', 'magic', 'yugioh', 'onepiece', 'lorcana', 'fab', 'pocket'));

COMMIT;
