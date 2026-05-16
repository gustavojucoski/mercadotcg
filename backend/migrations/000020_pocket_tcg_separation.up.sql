-- Migration 000020: Separate Pokémon TCG Pocket sets into tcg='pokemon-pocket'
--
-- Sets imported with the 'tcgp-' ID prefix belong to the TCG Pocket mobile
-- game and must use tcg='pokemon-pocket', distinct from tcg='pokemon'.
--
-- Steps:
--   1. Update the CHECK constraint to replace 'pocket' with 'pokemon-pocket'
--      (migration 000016 added 'pocket'; this migration supersedes that value).
--   2. Create card_series rows with tcg='pokemon-pocket' for exclusively
--      Pocket series (the UNIQUE constraint is (name, tcg), so we can't UPDATE
--      tcg in-place; we insert new rows and re-point the FK).
--   3. Flip card_sets.tcg from 'pokemon' to 'pokemon-pocket' for tcgp-* sets
--      and reassign series_id to the new series rows.
--   4. Remove orphaned 'pokemon' series that have no sets left.

BEGIN;

-- Step 1: Replace 'pocket' with 'pokemon-pocket' in the CHECK constraint.
ALTER TABLE card_sets DROP CONSTRAINT IF EXISTS chk_card_sets_tcg;
ALTER TABLE card_sets ADD CONSTRAINT chk_card_sets_tcg
    CHECK (tcg IN ('pokemon', 'pokemon-pocket', 'magic', 'yugioh', 'onepiece', 'lorcana', 'fab'));

-- Step 2: Create pokemon-pocket series for series used exclusively by Pocket sets.
INSERT INTO card_series (name, name_pt, tcg, created_at)
SELECT DISTINCT cr.name, cr.name_pt, 'pokemon-pocket', cr.created_at
FROM card_series cr
WHERE cr.tcg = 'pokemon'
  AND cr.id IN (
      SELECT cs.series_id
      FROM card_sets cs
      WHERE cs.series_id IS NOT NULL
      GROUP BY cs.series_id
      HAVING COUNT(*) = COUNT(*) FILTER (WHERE cs.code LIKE 'tcgp-%')
  )
ON CONFLICT (name, tcg) DO NOTHING;

-- Step 3: Re-point card_sets.series_id and flip tcg to 'pokemon-pocket'.
UPDATE card_sets cs
SET
    tcg       = 'pokemon-pocket',
    series_id = (
        SELECT new_ser.id
        FROM card_series new_ser
        JOIN card_series old_ser ON old_ser.id = cs.series_id
        WHERE new_ser.name = old_ser.name
          AND new_ser.tcg  = 'pokemon-pocket'
        LIMIT 1
    ),
    updated_at = now()
WHERE cs.code LIKE 'tcgp-%'
  AND cs.tcg = 'pokemon';

-- Step 4: Remove orphaned 'pokemon' series that no longer have any sets.
DELETE FROM card_series
WHERE tcg = 'pokemon'
  AND id NOT IN (
      SELECT DISTINCT series_id
      FROM card_sets
      WHERE series_id IS NOT NULL
  );

COMMIT;
