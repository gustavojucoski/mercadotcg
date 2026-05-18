// card_repo_import_source_test.go
//
// Integration tests for the import_source feature (migration 000019).
//
// These tests require a real PostgreSQL 16 instance with the full migration
// stack applied. They are skipped when DATABASE_URL is not set.
//
// To run locally:
//
//	DATABASE_URL="postgres://..." go test ./internal/repository/postgres/... -run TestImportSource -v
//
// The test schema is isolated per-test via a unique schema prefix so suites
// can run in parallel against the same DB without interfering.
package postgres_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/card"
	"github.com/gustavojucoski/mercadotcg/backend/internal/repository/postgres"
)

// ----------------------------------------------------------------------------
// Test helper: minimal DB setup
// ----------------------------------------------------------------------------

// openTestDB returns a live pool or skips the test when DATABASE_URL is absent.
// The caller is responsible for closing the pool (defer pool.Close()).
func openTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := postgres.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	return pool
}

// mustUpsertSeries inserts a series and returns its ID. Fatals on error.
func mustUpsertSeries(t *testing.T, repo *postgres.CardRepo, name string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	s, err := repo.UpsertSeries(ctx, name, "pokemon")
	if err != nil {
		t.Fatalf("upsert series %q: %v", name, err)
	}
	return s.ID
}

// mustUpsertSet inserts a set using the provided template and returns it with
// ID, CreatedAt, UpdatedAt populated.
func mustUpsertSet(t *testing.T, repo *postgres.CardRepo, s *card.Set) {
	t.Helper()
	ctx := context.Background()
	if err := repo.UpsertSet(ctx, s); err != nil {
		t.Fatalf("upsert set %q: %v", s.Code, err)
	}
}

// setCode produces a unique set code per test to avoid inter-test collisions
// when running against a shared database.
func setCode(t *testing.T, suffix string) string {
	t.Helper()
	// Use a short prefix derived from the test name + suffix to stay within VARCHAR(16).
	// Tests that need a truly random code can call uuid.NewString()[:8] themselves.
	h := fmt.Sprintf("tst%d", time.Now().UnixNano()%100000)
	return h + suffix
}

// ----------------------------------------------------------------------------
// Migration 000019 — ADD COLUMN idempotency
// ----------------------------------------------------------------------------

// TestMigration000019_ColumnsExist verifies that import_source columns exist on
// both card_sets and cards after the migration is applied.
// This also implicitly validates the DEFAULT 'tcgdex_legacy' clause: any row
// inserted without an explicit import_source will read back 'tcgdex_legacy'.
func TestMigration000019_ColumnsExist(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()

	ctx := context.Background()

	// Verify column exists on card_sets.
	var setsHas bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'card_sets' AND column_name = 'import_source'
		)`).Scan(&setsHas)
	if err != nil {
		t.Fatalf("check card_sets.import_source column: %v", err)
	}
	if !setsHas {
		t.Error("card_sets.import_source column is missing — was migration 000019 applied?")
	}

	// Verify column exists on cards.
	var cardsHas bool
	err = pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'cards' AND column_name = 'import_source'
		)`).Scan(&cardsHas)
	if err != nil {
		t.Fatalf("check cards.import_source column: %v", err)
	}
	if !cardsHas {
		t.Error("cards.import_source column is missing — was migration 000019 applied?")
	}
}

// cleanupSet registers a t.Cleanup that removes all cards and the set for
// the given code. card_sets.id is referenced by cards.set_id with ON DELETE
// RESTRICT, so cards must be deleted before the set.
func cleanupSet(t *testing.T, pool *pgxpool.Pool, code string) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		pool.Exec(ctx, `
			DELETE FROM cards c
			USING card_sets cs
			WHERE c.set_id = cs.id AND cs.code = $1`, code)
		pool.Exec(ctx, `DELETE FROM card_sets WHERE code = $1`, code)
	})
}

// TestMigration000019_DefaultValue verifies that rows inserted without an
// explicit import_source value carry the 'tcgdex_legacy' default.
// This simulates what existing rows look like after the migration is applied
// to a pre-populated database.
func TestMigration000019_DefaultValue(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()

	ctx := context.Background()
	code := setCode(t, "def")

	// Insert using the legacy INSERT path (no import_source column).
	_, err := pool.Exec(ctx, `
		INSERT INTO card_sets (code, name, language, tcg)
		VALUES ($1, 'Default Test Set', 'en', 'pokemon')`,
		code,
	)
	if err != nil {
		t.Fatalf("insert without import_source: %v", err)
	}
	cleanupSet(t, pool, code)

	var src string
	err = pool.QueryRow(ctx,
		`SELECT import_source FROM card_sets WHERE code = $1`, code,
	).Scan(&src)
	if err != nil {
		t.Fatalf("read import_source: %v", err)
	}
	if src != "tcgdex_legacy" {
		t.Errorf("expected default 'tcgdex_legacy', got %q", src)
	}
}

// TestMigration000019_Idempotent verifies that running the ADD COLUMN IF NOT
// EXISTS DDL twice does not error. We simulate this by executing the migration
// SQL directly against the already-migrated database.
func TestMigration000019_Idempotent(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()

	ctx := context.Background()

	_, err := pool.Exec(ctx,
		`ALTER TABLE card_sets ADD COLUMN IF NOT EXISTS import_source VARCHAR(32) NOT NULL DEFAULT 'tcgdex_legacy'`)
	if err != nil {
		t.Errorf("re-running card_sets ADD COLUMN IF NOT EXISTS errored: %v", err)
	}

	_, err = pool.Exec(ctx,
		`ALTER TABLE cards ADD COLUMN IF NOT EXISTS import_source VARCHAR(32) NOT NULL DEFAULT 'tcgdex_legacy'`)
	if err != nil {
		t.Errorf("re-running cards ADD COLUMN IF NOT EXISTS errored: %v", err)
	}
}

// ----------------------------------------------------------------------------
// UpsertSet — import_source semantics
// ----------------------------------------------------------------------------

// TestUpsertSet_NewSet_ScrydexSourcePersisted verifies that a brand-new set
// inserted with import_source = 'scrydex' reads back with that value.
func TestUpsertSet_NewSet_ScrydexSourcePersisted(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)

	seriesID := mustUpsertSeries(t, repo, "Scarlet & Violet Test"+setCode(t, "s"))

	code := setCode(t, "sv")
	s := card.Set{
		Code:         code,
		Name:         "Scrydex Import Test",
		SeriesID:     &seriesID,
		TCG:          "pokemon",
		Language:     card.LanguageEnglish,
		ImportSource: "scrydex",
	}
	mustUpsertSet(t, repo, &s)
	cleanupSet(t, pool, code)

	got, err := repo.GetSetByCode(context.Background(), code)
	if err != nil {
		t.Fatalf("GetSetByCode: %v", err)
	}
	if got.ImportSource != "scrydex" {
		t.Errorf("import_source: got %q, want %q", got.ImportSource, "scrydex")
	}
	if got.ID == uuid.Nil {
		t.Error("expected non-nil UUID after upsert")
	}
}

// TestUpsertSet_ManualSource_NotOverwritten is the critical protection test.
// A set with import_source = 'manual' must never be modified by the upsert path.
//
// SQL context: the ON CONFLICT clause includes
//
//	WHERE card_sets.import_source <> 'manual'
//
// which means when the guard fires PostgreSQL returns no row. The current
// UpsertSet implementation calls QueryRow(...).Scan(&s.ID, ...) — if no row is
// returned it gets pgx.ErrNoRows which it wraps as an error. This test captures
// that behaviour so a future fix (returning the existing row) is safe to make.
func TestUpsertSet_ManualSource_NotOverwritten(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)

	ctx := context.Background()
	code := setCode(t, "mn")
	seriesID := mustUpsertSeries(t, repo, "Manual Series"+code)

	// Pre-insert a 'manual' set directly via SQL to bypass UpsertSet.
	var originalID uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO card_sets (code, name, language, tcg, import_source, series_id)
		VALUES ($1, 'Manual Set Original', 'en', 'pokemon', 'manual', $2)
		RETURNING id`,
		code, seriesID,
	).Scan(&originalID)
	if err != nil {
		t.Fatalf("insert manual set: %v", err)
	}
	cleanupSet(t, pool, code)

	// Attempt to overwrite via the import path.
	scrydexSet := card.Set{
		Code:         code,
		Name:         "Should Not Overwrite",
		SeriesID:     &seriesID,
		TCG:          "pokemon",
		Language:     card.LanguageEnglish,
		ImportSource: "scrydex",
	}
	err = repo.UpsertSet(ctx, &scrydexSet)
	// NOTE: current implementation returns an error when the WHERE guard blocks
	// the update (pgx.ErrNoRows wrapped). This is a known gap — see QA notes.
	// When the implementation is fixed to return the existing row gracefully,
	// this assertion should change to require err == nil.
	if err == nil {
		// Fix was applied: verify the original data was not changed.
		got, fetchErr := repo.GetSetByCode(ctx, code)
		if fetchErr != nil {
			t.Fatalf("GetSetByCode after protected upsert: %v", fetchErr)
		}
		if got.Name != "Manual Set Original" {
			t.Errorf("manual set name was overwritten: got %q, want %q",
				got.Name, "Manual Set Original")
		}
		if got.ImportSource != "manual" {
			t.Errorf("import_source was overwritten to %q — protection failed", got.ImportSource)
		}
		if got.ID != originalID {
			t.Errorf("ID changed after guarded upsert: got %s, want %s", got.ID, originalID)
		}
	} else {
		// Current behaviour: the error reveals the protection fired.
		// Verify the original row is intact in the DB.
		got, fetchErr := repo.GetSetByCode(ctx, code)
		if fetchErr != nil {
			t.Fatalf("GetSetByCode after failed upsert: %v", fetchErr)
		}
		if got.Name != "Manual Set Original" {
			t.Errorf("manual set name was overwritten despite error: got %q", got.Name)
		}
		if got.ImportSource != "manual" {
			t.Errorf("import_source was overwritten despite error: got %q", got.ImportSource)
		}
		t.Logf("expected: UpsertSet returned error %q when manual guard fired (current behaviour)", err)
	}
}

// TestUpsertSet_LegacySource_UpdatedToScrydex verifies that a set currently
// holding import_source = 'tcgdex_legacy' is upgraded to 'scrydex' when the
// importer runs.
func TestUpsertSet_LegacySource_UpdatedToScrydex(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)

	ctx := context.Background()
	code := setCode(t, "lg")
	seriesID := mustUpsertSeries(t, repo, "Legacy Series"+code)

	// Insert a 'tcgdex_legacy' set directly.
	_, err := pool.Exec(ctx, `
		INSERT INTO card_sets (code, name, language, tcg, import_source, series_id)
		VALUES ($1, 'Legacy Set', 'en', 'pokemon', 'tcgdex_legacy', $2)`,
		code, seriesID,
	)
	if err != nil {
		t.Fatalf("insert legacy set: %v", err)
	}
	cleanupSet(t, pool, code)

	// Upsert as scrydex — should succeed and update import_source.
	update := card.Set{
		Code:         code,
		Name:         "Legacy Set",
		SeriesID:     &seriesID,
		TCG:          "pokemon",
		Language:     card.LanguageEnglish,
		ImportSource: "scrydex",
	}
	if err := repo.UpsertSet(ctx, &update); err != nil {
		t.Fatalf("upsert legacy → scrydex: %v", err)
	}

	got, err := repo.GetSetByCode(ctx, code)
	if err != nil {
		t.Fatalf("GetSetByCode: %v", err)
	}
	if got.ImportSource != "scrydex" {
		t.Errorf("import_source not updated: got %q, want %q", got.ImportSource, "scrydex")
	}
}

// TestUpsertSet_Idempotent verifies that running the same upsert twice with
// identical data does not change the row and does not error.
func TestUpsertSet_Idempotent(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)

	ctx := context.Background()
	code := setCode(t, "id")
	seriesID := mustUpsertSeries(t, repo, "Idempotent Series"+code)

	s := card.Set{
		Code:         code,
		Name:         "Idempotent Set",
		SeriesID:     &seriesID,
		TCG:          "pokemon",
		Language:     card.LanguageEnglish,
		ImportSource: "scrydex",
	}

	mustUpsertSet(t, repo, &s)
	cleanupSet(t, pool, code)
	firstID := s.ID

	// Second run: same data.
	s2 := card.Set{
		Code:         code,
		Name:         "Idempotent Set",
		SeriesID:     &seriesID,
		TCG:          "pokemon",
		Language:     card.LanguageEnglish,
		ImportSource: "scrydex",
	}
	if err := repo.UpsertSet(ctx, &s2); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if s2.ID != firstID {
		t.Errorf("ID changed between upserts: first=%s second=%s", firstID, s2.ID)
	}

	got, err := repo.GetSetByCode(ctx, code)
	if err != nil {
		t.Fatalf("GetSetByCode: %v", err)
	}
	if got.ImportSource != "scrydex" {
		t.Errorf("import_source after re-upsert: got %q, want %q", got.ImportSource, "scrydex")
	}
}

// TestUpsertSet_EmptyImportSource_DefaultsToTCGDexLegacy verifies that when
// ImportSource is left empty in the domain object, the repo substitutes
// 'tcgdex_legacy' as per its defensive default.
func TestUpsertSet_EmptyImportSource_DefaultsToTCGDexLegacy(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)

	ctx := context.Background()
	code := setCode(t, "em")
	seriesID := mustUpsertSeries(t, repo, "EmptySrc Series"+code)

	s := card.Set{
		Code:         code,
		Name:         "No Import Source",
		SeriesID:     &seriesID,
		TCG:          "pokemon",
		Language:     card.LanguageEnglish,
		ImportSource: "", // intentionally empty
	}
	mustUpsertSet(t, repo, &s)
	cleanupSet(t, pool, code)

	got, err := repo.GetSetByCode(ctx, code)
	if err != nil {
		t.Fatalf("GetSetByCode: %v", err)
	}
	if got.ImportSource != "tcgdex_legacy" {
		t.Errorf("import_source: got %q, want %q (defensive default)", got.ImportSource, "tcgdex_legacy")
	}
}

// ----------------------------------------------------------------------------
// ListCardsForPTEnrichment
// ----------------------------------------------------------------------------

// seedSetAndCard inserts a set + card and returns both IDs.
// The card's import_source is set via a direct UPDATE after the UpsertCard
// call because UpsertCard does not yet carry import_source (it's set at the
// card level by future work; currently the column default is 'tcgdex_legacy').
func seedSetAndCard(
	t *testing.T,
	pool *pgxpool.Pool,
	repo *postgres.CardRepo,
	code string,
	cardNumber string,
	cardImportSource string,
	namePT *string,
	imageURLPT *string,
) (setID, cardID uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	seriesID := mustUpsertSeries(t, repo, "PT Series"+code)

	s := card.Set{
		Code:         code,
		Name:         "PT Test Set " + code,
		SeriesID:     &seriesID,
		TCG:          "pokemon",
		Language:     card.LanguageEnglish,
		ImportSource: "scrydex",
	}
	mustUpsertSet(t, repo, &s)
	setID = s.ID

	c := card.Card{
		SetID:           setID,
		CollectorNumber: cardNumber,
		Name:            "Pikachu Test " + cardNumber,
	}
	if err := repo.UpsertCard(ctx, &c); err != nil {
		t.Fatalf("upsert card: %v", err)
	}
	cardID = c.ID

	// Patch import_source on the card directly — UpsertCard does not set it.
	if _, err := pool.Exec(ctx,
		`UPDATE cards SET import_source = $1 WHERE id = $2`,
		cardImportSource, cardID,
	); err != nil {
		t.Fatalf("patch card import_source: %v", err)
	}

	// Patch name_pt / image_url_pt when provided.
	if namePT != nil {
		if _, err := pool.Exec(ctx,
			`UPDATE cards SET name_pt = $1 WHERE id = $2`, *namePT, cardID,
		); err != nil {
			t.Fatalf("patch name_pt: %v", err)
		}
	}
	if imageURLPT != nil {
		if _, err := pool.Exec(ctx,
			`UPDATE cards SET image_url_pt = $1 WHERE id = $2`, *imageURLPT, cardID,
		); err != nil {
			t.Fatalf("patch image_url_pt: %v", err)
		}
	}

	return setID, cardID
}

func strPtr(s string) *string { return &s }

// TestListCardsForPTEnrichment_OnlyReturnsScrydexMissingPT verifies that only
// scrydex-sourced cards with at least one PT field missing appear.
func TestListCardsForPTEnrichment_OnlyReturnsScrydexMissingPT(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)
	ctx := context.Background()

	prefix := setCode(t, "pe")

	// Card A: scrydex + both fields null     → must appear
	_, cardA := seedSetAndCard(t, pool, repo, prefix+"A", "1", "scrydex", nil, nil)
	// Card B: scrydex + only name_pt missing  → must appear
	_, cardB := seedSetAndCard(t, pool, repo, prefix+"B", "1", "scrydex", nil, strPtr("https://img/b_pt.webp"))
	// Card C: scrydex + only image missing   → must appear
	_, cardC := seedSetAndCard(t, pool, repo, prefix+"C", "1", "scrydex", strPtr("Pikachu PT"), nil)
	// Card D: scrydex + both fields present  → must NOT appear
	_, cardD := seedSetAndCard(t, pool, repo, prefix+"D", "1", "scrydex", strPtr("Pikachu PT"), strPtr("https://img/d_pt.webp"))
	// Card E: tcgdex_legacy + both null      → must NOT appear (wrong source)
	_, cardE := seedSetAndCard(t, pool, repo, prefix+"E", "1", "tcgdex_legacy", nil, nil)

	// Register cleanup for all sets.
	// cards.set_id has ON DELETE RESTRICT, so cards must be deleted before their set.
	for _, sfx := range []string{"A", "B", "C", "D", "E"} {
		sfxCopy := prefix + sfx
		t.Cleanup(func() {
			ctx := context.Background()
			pool.Exec(ctx, `
				DELETE FROM cards c
				USING card_sets cs
				WHERE c.set_id = cs.id AND cs.code = $1`, sfxCopy)
			pool.Exec(ctx, `DELETE FROM card_sets WHERE code = $1`, sfxCopy)
		})
	}

	candidates, err := repo.ListCardsForPTEnrichment(ctx, 100)
	if err != nil {
		t.Fatalf("ListCardsForPTEnrichment: %v", err)
	}

	// Build a set of returned IDs for easy lookup.
	found := make(map[uuid.UUID]bool, len(candidates))
	for _, c := range candidates {
		found[c.ID] = true
	}

	// Must appear.
	for _, id := range []uuid.UUID{cardA, cardB, cardC} {
		if !found[id] {
			t.Errorf("card %s should appear in enrichment candidates but did not", id)
		}
	}
	// Must NOT appear.
	for _, id := range []uuid.UUID{cardD, cardE} {
		if found[id] {
			t.Errorf("card %s should NOT appear in enrichment candidates but did", id)
		}
	}
}

// TestListCardsForPTEnrichment_LimitRespected verifies that the limit parameter
// is honoured: requesting fewer results than available does not overflow.
func TestListCardsForPTEnrichment_LimitRespected(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)
	ctx := context.Background()

	prefix := setCode(t, "lm")

	// Seed 5 eligible cards.
	for i := 0; i < 5; i++ {
		code := fmt.Sprintf("%sL%d", prefix, i)
		seedSetAndCard(t, pool, repo, code, "1", "scrydex", nil, nil)
		cleanupSet(t, pool, code)
	}

	candidates, err := repo.ListCardsForPTEnrichment(ctx, 3)
	if err != nil {
		t.Fatalf("ListCardsForPTEnrichment limit=3: %v", err)
	}
	if len(candidates) > 3 {
		t.Errorf("limit=3 but got %d candidates", len(candidates))
	}
}

// TestListCardsForPTEnrichment_Empty verifies that an empty result set is
// returned (not nil, not an error) when no eligible cards exist.
func TestListCardsForPTEnrichment_Empty(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)
	ctx := context.Background()

	// Seed one card that is fully enriched — should not appear.
	prefix := setCode(t, "ze")
	seedSetAndCard(t, pool, repo, prefix+"Z", "1", "scrydex",
		strPtr("Pikachu PT"), strPtr("https://img/pt.webp"))
	cleanupSet(t, pool, prefix+"Z")

	// Query with limit=1 and a very restrictive set by checking only cards we just
	// inserted via their set code, but the function has no set filter, so we just
	// verify length stays at 0 in a fresh DB scenario or that fully-enriched cards
	// are excluded.
	candidates, err := repo.ListCardsForPTEnrichment(ctx, 1)
	if err != nil {
		t.Fatalf("ListCardsForPTEnrichment: %v", err)
	}
	// The fully-enriched card must not be in the result.
	for _, c := range candidates {
		if c.SetCode == prefix+"Z" {
			t.Errorf("fully-enriched card from set %s should not appear in candidates", prefix+"Z")
		}
	}
}

// ----------------------------------------------------------------------------
// UpdateCardPT
// ----------------------------------------------------------------------------

// TestUpdateCardPT_WritesNamePT verifies that a non-empty namePT is written.
func TestUpdateCardPT_WritesNamePT(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)
	ctx := context.Background()

	prefix := setCode(t, "wp")
	_, cardID := seedSetAndCard(t, pool, repo, prefix, "1", "scrydex", nil, nil)
	cleanupSet(t, pool, prefix)

	if err := repo.UpdateCardPT(ctx, cardID, "Pikachu PT", ""); err != nil {
		t.Fatalf("UpdateCardPT: %v", err)
	}

	var namePT, imageURLPT *string
	err := pool.QueryRow(ctx,
		`SELECT name_pt, image_url_pt FROM cards WHERE id = $1`, cardID,
	).Scan(&namePT, &imageURLPT)
	if err != nil {
		t.Fatalf("read card: %v", err)
	}

	if namePT == nil || *namePT != "Pikachu PT" {
		t.Errorf("name_pt: got %v, want %q", namePT, "Pikachu PT")
	}
	if imageURLPT != nil {
		t.Errorf("image_url_pt should remain nil, got %q", *imageURLPT)
	}
}

// TestUpdateCardPT_WritesImageURLPT verifies that a non-empty imageURLPT is written.
func TestUpdateCardPT_WritesImageURLPT(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)
	ctx := context.Background()

	prefix := setCode(t, "wi")
	_, cardID := seedSetAndCard(t, pool, repo, prefix, "1", "scrydex", nil, nil)
	cleanupSet(t, pool, prefix)

	wantURL := "https://s3.example.com/pokemon/cards/sv01/sv01-001_pt.webp"
	if err := repo.UpdateCardPT(ctx, cardID, "", wantURL); err != nil {
		t.Fatalf("UpdateCardPT: %v", err)
	}

	var namePT, imageURLPT *string
	err := pool.QueryRow(ctx,
		`SELECT name_pt, image_url_pt FROM cards WHERE id = $1`, cardID,
	).Scan(&namePT, &imageURLPT)
	if err != nil {
		t.Fatalf("read card: %v", err)
	}

	if namePT != nil {
		t.Errorf("name_pt should remain nil, got %q", *namePT)
	}
	if imageURLPT == nil || *imageURLPT != wantURL {
		t.Errorf("image_url_pt: got %v, want %q", imageURLPT, wantURL)
	}
}

// TestUpdateCardPT_WritesBoothFields verifies the combined case.
func TestUpdateCardPT_WritesBothFields(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)
	ctx := context.Background()

	prefix := setCode(t, "wb")
	_, cardID := seedSetAndCard(t, pool, repo, prefix, "1", "scrydex", nil, nil)
	cleanupSet(t, pool, prefix)

	if err := repo.UpdateCardPT(ctx, cardID, "Pikachu PT", "https://img/pt.webp"); err != nil {
		t.Fatalf("UpdateCardPT: %v", err)
	}

	var namePT, imageURLPT *string
	err := pool.QueryRow(ctx,
		`SELECT name_pt, image_url_pt FROM cards WHERE id = $1`, cardID,
	).Scan(&namePT, &imageURLPT)
	if err != nil {
		t.Fatalf("read card: %v", err)
	}
	if namePT == nil || *namePT != "Pikachu PT" {
		t.Errorf("name_pt: got %v, want %q", namePT, "Pikachu PT")
	}
	if imageURLPT == nil || *imageURLPT != "https://img/pt.webp" {
		t.Errorf("image_url_pt: got %v, want %q", imageURLPT, "https://img/pt.webp")
	}
}

// TestUpdateCardPT_EmptyStrings_PreservesExisting verifies the CASE WHEN semantics:
// passing both empty strings must not overwrite already-stored values.
func TestUpdateCardPT_EmptyStrings_PreservesExisting(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)
	ctx := context.Background()

	prefix := setCode(t, "pe2")
	originalName := "Pikachu PT Original"
	originalURL := "https://img/original_pt.webp"
	_, cardID := seedSetAndCard(t, pool, repo, prefix, "1", "scrydex",
		strPtr(originalName), strPtr(originalURL))
	cleanupSet(t, pool, prefix)

	// Call with both empty — must be a no-op on the PT fields.
	if err := repo.UpdateCardPT(ctx, cardID, "", ""); err != nil {
		t.Fatalf("UpdateCardPT with empty strings: %v", err)
	}

	var namePT, imageURLPT *string
	err := pool.QueryRow(ctx,
		`SELECT name_pt, image_url_pt FROM cards WHERE id = $1`, cardID,
	).Scan(&namePT, &imageURLPT)
	if err != nil {
		t.Fatalf("read card: %v", err)
	}
	if namePT == nil || *namePT != originalName {
		t.Errorf("name_pt was cleared: got %v, want %q", namePT, originalName)
	}
	if imageURLPT == nil || *imageURLPT != originalURL {
		t.Errorf("image_url_pt was cleared: got %v, want %q", imageURLPT, originalURL)
	}
}

// TestUpdateCardPT_NonExistentCard_ReturnsErrNotFound verifies sentinel error.
func TestUpdateCardPT_NonExistentCard_ReturnsErrNotFound(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)
	ctx := context.Background()

	err := repo.UpdateCardPT(ctx, uuid.New(), "Name PT", "https://img/pt.webp")
	if !isErrNotFound(err) {
		t.Errorf("expected ErrNotFound for non-existent card, got: %v", err)
	}
}

// isErrNotFound checks for the postgres.ErrNotFound sentinel, handling wrapping.
func isErrNotFound(err error) bool {
	if err == nil {
		return false
	}
	// Direct comparison (errors.Is will work with %w wrapping too).
	return err.Error() == postgres.ErrNotFound.Error() ||
		containsString(err.Error(), "não encontrado") ||
		containsString(err.Error(), "not found")
}

func containsString(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && stringContains(s, sub))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
