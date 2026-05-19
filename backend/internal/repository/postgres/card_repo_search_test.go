// card_repo_search_test.go
//
// Integration tests for SearchCards — the paginated full-catalog search added
// in feat/search-page.
//
// These tests require a live PostgreSQL instance with all migrations applied.
// They skip gracefully when DATABASE_URL is not set.
//
// Run locally:
//
//	DATABASE_URL="postgres://..." go test ./internal/repository/postgres/... -run TestSearchCards -v
package postgres_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/card"
	"github.com/gustavojucoski/mercadotcg/backend/internal/repository/postgres"
)

// ----------------------------------------------------------------------------
// Fixture helpers scoped to search tests
// ----------------------------------------------------------------------------

type searchFixture struct {
	prefix string
}

func newSearchFixture(t *testing.T) *searchFixture {
	t.Helper()
	return &searchFixture{
		prefix: fmt.Sprintf("sc%d", time.Now().UnixNano()%1_000_000),
	}
}

// seedSearchCard inserts a minimal set + card and registers cleanup.
// Returns the card ID so tests can assert on it.
func (f *searchFixture) seedSearchCard(
	t *testing.T,
	pool *pgxpool.Pool,
	repo *postgres.CardRepo,
	discriminator string,
	cardName string,
	collectorNumber string,
	rarity string,
	tcg string,
	language card.Language,
) uuid.UUID {
	t.Helper()
	ctx := context.Background()

	// Unique set code per test run + discriminator.
	code := f.prefix + discriminator
	if len(code) > 16 {
		code = code[:16]
	}

	seriesID := mustUpsertSeries(t, repo, f.prefix+"-sr-"+discriminator)

	s := card.Set{
		Code:         code,
		Name:         f.prefix + " Search Set " + discriminator,
		SeriesID:     &seriesID,
		TCG:          tcg,
		Language:     language,
		ImportSource: "scrydex",
	}
	mustUpsertSet(t, repo, &s)

	c := card.Card{
		SetID:           s.ID,
		CollectorNumber: collectorNumber,
		Name:            cardName,
		Rarity:          rarity,
		ImportSource:    "scrydex",
	}
	if err := repo.UpsertCard(ctx, &c); err != nil {
		t.Fatalf("upsert card %q: %v", cardName, err)
	}

	t.Cleanup(func() {
		p := openTestDB(t)
		defer p.Close()
		cleanupSet(t, p, code)
	})

	return c.ID
}

// findCardInResults returns true when cardID appears in the result slice.
func findCardInResults(results []postgres.SearchCardResult, cardID uuid.UUID) bool {
	id := cardID.String()
	for _, r := range results {
		if r.ID == id {
			return true
		}
	}
	return false
}

// ----------------------------------------------------------------------------
// Happy path: q matches name, total and page returned correctly
// ----------------------------------------------------------------------------

// TestSearchCards_HappyPath verifies that a text search on q finds a card by
// name and returns the expected total + has_more=false when only one page exists.
func TestSearchCards_HappyPath(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)

	f := newSearchFixture(t)
	cardID := f.seedSearchCard(t, pool, repo, "hp", f.prefix+" Pikachu", "1", "Common", "pokemon", card.LanguageEnglish)

	ctx := context.Background()
	results, total, err := repo.SearchCards(ctx, postgres.SearchCardsParams{
		Q:     f.prefix + " Pikachu",
		Sort:  "name",
		Order: "asc",
		Page:  1,
		Limit: 24,
	})
	if err != nil {
		t.Fatalf("SearchCards: %v", err)
	}
	if total < 1 {
		t.Errorf("expected total >= 1, got %d", total)
	}
	if !findCardInResults(results, cardID) {
		t.Errorf("expected card %s in results but not found (total=%d)", cardID, total)
	}
}

// ----------------------------------------------------------------------------
// Empty q returns all cards (no text filter applied)
// ----------------------------------------------------------------------------

// TestSearchCards_EmptyQ verifies that an empty q applies no text filter and
// returns cards — important for the /search landing state.
func TestSearchCards_EmptyQ(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)

	f := newSearchFixture(t)
	cardID := f.seedSearchCard(t, pool, repo, "eq", f.prefix+" Eevee Empty", "2", "Uncommon", "pokemon", card.LanguageEnglish)

	ctx := context.Background()
	results, total, err := repo.SearchCards(ctx, postgres.SearchCardsParams{
		Q:     "", // no filter
		Sort:  "name",
		Order: "asc",
		Page:  1,
		Limit: 24,
	})
	if err != nil {
		t.Fatalf("SearchCards empty q: %v", err)
	}
	if total < 1 {
		t.Errorf("expected total >= 1 for empty q, got %d", total)
	}
	if !findCardInResults(results, cardID) {
		t.Errorf("card %s not found in empty-q results (got %d results, total=%d)", cardID, len(results), total)
	}
}

// ----------------------------------------------------------------------------
// ILIKE escape: "%" in q is a literal percent, not a wildcard
// ----------------------------------------------------------------------------

// TestSearchCards_LiteralPercent verifies that escapeLikePattern (called by the
// handler before invoking SearchCards) prevents "%" from acting as a SQL wildcard.
//
// The repo receives a pre-escaped pattern ("\%"). If escaping were absent, "%"
// would match every card name and the seeded card's distinct name would appear —
// but it must not appear since it contains no literal "%".
func TestSearchCards_LiteralPercent(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)

	f := newSearchFixture(t)
	cardID := f.seedSearchCard(t, pool, repo, "lp", f.prefix+" Percent Card", "3", "Rare", "pokemon", card.LanguageEnglish)

	ctx := context.Background()
	// Simulate what the handler sends after escapeLikePattern("%").
	results, _, err := repo.SearchCards(ctx, postgres.SearchCardsParams{
		Q:     `\%`, // handler-escaped literal percent
		Sort:  "name",
		Order: "asc",
		Page:  1,
		Limit: 24,
	})
	if err != nil {
		t.Fatalf("SearchCards with escaped percent: %v", err)
	}
	if findCardInResults(results, cardID) {
		t.Errorf("card without literal %% in name appeared in \\%% query — escape is broken")
	}
}

// ----------------------------------------------------------------------------
// ILIKE escape: "_" in q is a literal underscore, not a single-char wildcard
// ----------------------------------------------------------------------------

func TestSearchCards_LiteralUnderscore(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)

	f := newSearchFixture(t)
	cardID := f.seedSearchCard(t, pool, repo, "lu", f.prefix+" Underscore Card", "4", "Rare", "pokemon", card.LanguageEnglish)

	ctx := context.Background()
	results, _, err := repo.SearchCards(ctx, postgres.SearchCardsParams{
		Q:     `\_`, // handler-escaped literal underscore
		Sort:  "name",
		Order: "asc",
		Page:  1,
		Limit: 24,
	})
	if err != nil {
		t.Fatalf("SearchCards with escaped underscore: %v", err)
	}
	if findCardInResults(results, cardID) {
		t.Errorf("card without literal _ in name appeared in \\_ query — escape is broken")
	}
}

// ----------------------------------------------------------------------------
// TCG filter isolates results
// ----------------------------------------------------------------------------

// TestSearchCards_TCGFilter verifies that passing tcg="pokemon-pocket" excludes
// cards belonging to tcg="pokemon".
func TestSearchCards_TCGFilter(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)

	f := newSearchFixture(t)
	pokemonCard := f.seedSearchCard(t, pool, repo, "tpk", f.prefix+" TCG Card", "5", "Common", "pokemon", card.LanguageEnglish)
	pocketCard := f.seedSearchCard(t, pool, repo, "tpo", f.prefix+" TCG Card", "5", "Common", "pokemon-pocket", card.LanguageEnglish)

	ctx := context.Background()

	// Filter to pokemon-pocket only.
	results, _, err := repo.SearchCards(ctx, postgres.SearchCardsParams{
		Q:     f.prefix + " TCG Card",
		Sort:  "name",
		Order: "asc",
		TCG:   "pokemon-pocket",
		Page:  1,
		Limit: 24,
	})
	if err != nil {
		t.Fatalf("SearchCards TCG filter: %v", err)
	}
	if findCardInResults(results, pokemonCard) {
		t.Error("pokemon card appeared in pokemon-pocket query — TCG filter is broken")
	}
	if !findCardInResults(results, pocketCard) {
		t.Error("pokemon-pocket card did not appear in pokemon-pocket query")
	}
}

// ----------------------------------------------------------------------------
// Language filter
// ----------------------------------------------------------------------------

// TestSearchCards_LangFilter verifies that lang="ja" excludes English cards.
func TestSearchCards_LangFilter(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)

	f := newSearchFixture(t)
	enCard := f.seedSearchCard(t, pool, repo, "lfe", f.prefix+" Lang Card", "6", "Common", "pokemon", card.LanguageEnglish)
	jaCard := f.seedSearchCard(t, pool, repo, "lfj", f.prefix+" Lang Card", "6", "Common", "pokemon", card.LanguageJapanese)

	ctx := context.Background()

	results, _, err := repo.SearchCards(ctx, postgres.SearchCardsParams{
		Q:     f.prefix + " Lang Card",
		Sort:  "name",
		Order: "asc",
		Lang:  "ja",
		Page:  1,
		Limit: 24,
	})
	if err != nil {
		t.Fatalf("SearchCards lang filter: %v", err)
	}
	if findCardInResults(results, enCard) {
		t.Error("EN card appeared when lang=ja filter applied")
	}
	if !findCardInResults(results, jaCard) {
		t.Error("JA card did not appear when lang=ja filter applied")
	}
}

// ----------------------------------------------------------------------------
// Rarity filter (ILIKE with pre-escaped pattern)
// ----------------------------------------------------------------------------

// TestSearchCards_RarityFilter verifies that a rarity filter returns only
// cards matching that rarity substring.
func TestSearchCards_RarityFilter(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)

	f := newSearchFixture(t)
	rareCard := f.seedSearchCard(t, pool, repo, "rf1", f.prefix+" Rarity Card A", "7", "Double Rare", "pokemon", card.LanguageEnglish)
	commonCard := f.seedSearchCard(t, pool, repo, "rf2", f.prefix+" Rarity Card B", "8", "Common", "pokemon", card.LanguageEnglish)

	ctx := context.Background()
	// "Double Rare" after escapeLikePattern is "Double Rare" (no specials), so
	// repo receives it as-is and wraps it with % in the ILIKE: %Double Rare%.
	results, _, err := repo.SearchCards(ctx, postgres.SearchCardsParams{
		Q:      f.prefix + " Rarity Card",
		Sort:   "name",
		Order:  "asc",
		Rarity: "Double Rare",
		Page:   1,
		Limit:  24,
	})
	if err != nil {
		t.Fatalf("SearchCards rarity filter: %v", err)
	}
	if !findCardInResults(results, rareCard) {
		t.Error("Double Rare card did not appear with rarity=Double Rare filter")
	}
	if findCardInResults(results, commonCard) {
		t.Error("Common card appeared when rarity=Double Rare filter applied")
	}
}

// ----------------------------------------------------------------------------
// Sort by collector_number — numeric sort for alphanumeric values
// ----------------------------------------------------------------------------

// TestSearchCards_SortCollectorNumber_Numeric verifies that cards with numeric
// collector_numbers are sorted numerically, not lexicographically.
// "2" < "10" numerically but "10" < "2" lexicographically.
func TestSearchCards_SortCollectorNumber_Numeric(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)

	f := newSearchFixture(t)
	// Seed two cards in the same set to ensure set ordering does not interfere.
	// They use the same set but different collector_numbers.
	ctx := context.Background()
	code := f.prefix + "cn"
	if len(code) > 16 {
		code = code[:16]
	}

	seriesID := mustUpsertSeries(t, repo, f.prefix+"-series-cn")
	s := card.Set{
		Code:         code,
		Name:         f.prefix + " CN Sort Set",
		SeriesID:     &seriesID,
		TCG:          "pokemon",
		Language:     card.LanguageEnglish,
		ImportSource: "scrydex",
	}
	mustUpsertSet(t, repo, &s)
	t.Cleanup(func() {
		p := openTestDB(t)
		defer p.Close()
		cleanupSet(t, p, code)
	})

	cardNumbers := []struct {
		num  string
		name string
	}{
		{"10", f.prefix + " CN Sort Card Ten"},
		{"2", f.prefix + " CN Sort Card Two"},
		{"1", f.prefix + " CN Sort Card One"},
	}
	for _, cn := range cardNumbers {
		c := card.Card{
			SetID:           s.ID,
			CollectorNumber: cn.num,
			Name:            cn.name,
			ImportSource:    "scrydex",
		}
		if err := repo.UpsertCard(ctx, &c); err != nil {
			t.Fatalf("upsert card %s: %v", cn.num, err)
		}
	}

	results, _, err := repo.SearchCards(ctx, postgres.SearchCardsParams{
		Q:     f.prefix + " CN Sort Card",
		Sort:  "collector_number",
		Order: "asc",
		Page:  1,
		Limit: 24,
	})
	if err != nil {
		t.Fatalf("SearchCards collector_number sort: %v", err)
	}
	if len(results) < 3 {
		t.Fatalf("expected >= 3 results, got %d", len(results))
	}

	// Find our three seeded cards in the result and verify order: 1, 2, 10.
	var found []string
	for _, r := range results {
		switch r.CollectorNumber {
		case "1", "2", "10":
			if r.Set.Code == code {
				found = append(found, r.CollectorNumber)
			}
		}
	}
	if len(found) != 3 {
		t.Fatalf("expected 3 seeded cards in results, got %d: %v", len(found), found)
	}
	want := []string{"1", "2", "10"}
	for i, w := range want {
		if found[i] != w {
			t.Errorf("position %d: got collector_number=%q, want %q (order is lexicographic, not numeric)", i, found[i], w)
		}
	}
}

// TestSearchCards_SortCollectorNumber_Alphanumeric verifies that alphanumeric
// collector_numbers like "TG01" and "GG70" sort correctly.
// The regex_replace strips non-digits: "TG01" → 1, "GG70" → 70.
// So TG01 sorts before GG70 in ascending numeric order.
func TestSearchCards_SortCollectorNumber_Alphanumeric(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)

	f := newSearchFixture(t)
	ctx := context.Background()
	code := f.prefix + "an"
	if len(code) > 16 {
		code = code[:16]
	}

	seriesID := mustUpsertSeries(t, repo, f.prefix+"-series-an")
	s := card.Set{
		Code:         code,
		Name:         f.prefix + " Alpha Sort Set",
		SeriesID:     &seriesID,
		TCG:          "pokemon",
		Language:     card.LanguageEnglish,
		ImportSource: "scrydex",
	}
	mustUpsertSet(t, repo, &s)
	t.Cleanup(func() {
		p := openTestDB(t)
		defer p.Close()
		cleanupSet(t, p, code)
	})

	for _, cn := range []struct{ num, name string }{
		{"GG70", f.prefix + " Alpha Card GG70"},
		{"TG01", f.prefix + " Alpha Card TG01"},
		{"TG15", f.prefix + " Alpha Card TG15"},
	} {
		c := card.Card{
			SetID:           s.ID,
			CollectorNumber: cn.num,
			Name:            cn.name,
			ImportSource:    "scrydex",
		}
		if err := repo.UpsertCard(ctx, &c); err != nil {
			t.Fatalf("upsert card %s: %v", cn.num, err)
		}
	}

	results, _, err := repo.SearchCards(ctx, postgres.SearchCardsParams{
		Q:     f.prefix + " Alpha Card",
		Sort:  "collector_number",
		Order: "asc",
		Page:  1,
		Limit: 24,
	})
	if err != nil {
		t.Fatalf("SearchCards alphanumeric sort: %v", err)
	}

	var found []string
	for _, r := range results {
		if r.Set.Code == code {
			found = append(found, r.CollectorNumber)
		}
	}
	if len(found) != 3 {
		t.Fatalf("expected 3 seeded cards, got %d: %v", len(found), found)
	}
	// numeric part: TG01→1, TG15→15, GG70→70 → ascending: TG01, TG15, GG70
	want := []string{"TG01", "TG15", "GG70"}
	for i, w := range want {
		if found[i] != w {
			t.Errorf("position %d: got %q, want %q (alphanumeric collector_number sort broken)", i, found[i], w)
		}
	}
}

// ----------------------------------------------------------------------------
// Descending sort
// ----------------------------------------------------------------------------

// TestSearchCards_SortNameDesc verifies that order=desc reverses the name ordering.
func TestSearchCards_SortNameDesc(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)

	f := newSearchFixture(t)
	ctx := context.Background()
	code := f.prefix + "nd"
	if len(code) > 16 {
		code = code[:16]
	}

	seriesID := mustUpsertSeries(t, repo, f.prefix+"-series-nd")
	s := card.Set{
		Code:         code,
		Name:         f.prefix + " Desc Sort Set",
		SeriesID:     &seriesID,
		TCG:          "pokemon",
		Language:     card.LanguageEnglish,
		ImportSource: "scrydex",
	}
	mustUpsertSet(t, repo, &s)
	t.Cleanup(func() {
		p := openTestDB(t)
		defer p.Close()
		cleanupSet(t, p, code)
	})

	names := []string{
		f.prefix + " Desc Card A",
		f.prefix + " Desc Card B",
		f.prefix + " Desc Card C",
	}
	for i, name := range names {
		c := card.Card{
			SetID:           s.ID,
			CollectorNumber: fmt.Sprintf("%d", i+1),
			Name:            name,
			ImportSource:    "scrydex",
		}
		if err := repo.UpsertCard(ctx, &c); err != nil {
			t.Fatalf("upsert card %q: %v", name, err)
		}
	}

	results, _, err := repo.SearchCards(ctx, postgres.SearchCardsParams{
		Q:     f.prefix + " Desc Card",
		Sort:  "name",
		Order: "desc",
		Page:  1,
		Limit: 24,
	})
	if err != nil {
		t.Fatalf("SearchCards name desc: %v", err)
	}

	var found []string
	for _, r := range results {
		if r.Set.Code == code {
			found = append(found, r.Name)
		}
	}
	if len(found) != 3 {
		t.Fatalf("expected 3 cards, got %d: %v", len(found), found)
	}
	// DESC: C > B > A
	want := []string{
		f.prefix + " Desc Card C",
		f.prefix + " Desc Card B",
		f.prefix + " Desc Card A",
	}
	for i, w := range want {
		if found[i] != w {
			t.Errorf("position %d: got %q, want %q", i, found[i], w)
		}
	}
}

// ----------------------------------------------------------------------------
// Pagination: page/limit slicing + has_more semantics
// ----------------------------------------------------------------------------

// TestSearchCards_Pagination verifies that page 1 returns limit rows, total
// reflects the full count, and page 2 returns the remainder.
func TestSearchCards_Pagination(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)

	f := newSearchFixture(t)
	ctx := context.Background()
	code := f.prefix + "pg"
	if len(code) > 16 {
		code = code[:16]
	}

	seriesID := mustUpsertSeries(t, repo, f.prefix+"-series-pg")
	s := card.Set{
		Code:         code,
		Name:         f.prefix + " Pager Set",
		SeriesID:     &seriesID,
		TCG:          "pokemon",
		Language:     card.LanguageEnglish,
		ImportSource: "scrydex",
	}
	mustUpsertSet(t, repo, &s)
	t.Cleanup(func() {
		p := openTestDB(t)
		defer p.Close()
		cleanupSet(t, p, code)
	})

	// Seed 5 cards for pagination test.
	for i := 1; i <= 5; i++ {
		c := card.Card{
			SetID:           s.ID,
			CollectorNumber: fmt.Sprintf("%d", i),
			Name:            fmt.Sprintf("%s Pager Card %d", f.prefix, i),
			ImportSource:    "scrydex",
		}
		if err := repo.UpsertCard(ctx, &c); err != nil {
			t.Fatalf("upsert card %d: %v", i, err)
		}
	}

	q := f.prefix + " Pager Card"

	// Page 1, limit 3.
	page1, total1, err := repo.SearchCards(ctx, postgres.SearchCardsParams{
		Q: q, Sort: "name", Order: "asc", Page: 1, Limit: 3,
	})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if total1 != 5 {
		t.Errorf("expected total=5, got %d", total1)
	}
	if len(page1) != 3 {
		t.Errorf("expected 3 results on page 1, got %d", len(page1))
	}

	// Page 2, limit 3.
	page2, total2, err := repo.SearchCards(ctx, postgres.SearchCardsParams{
		Q: q, Sort: "name", Order: "asc", Page: 2, Limit: 3,
	})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if total2 != 5 {
		t.Errorf("expected total=5 on page 2, got %d", total2)
	}
	if len(page2) != 2 {
		t.Errorf("expected 2 results on page 2, got %d", len(page2))
	}

	// No duplicate IDs across pages.
	seen := map[string]bool{}
	for _, r := range append(page1, page2...) {
		if seen[r.ID] {
			t.Errorf("duplicate result ID across pages: %s", r.ID)
		}
		seen[r.ID] = true
	}
}

// ----------------------------------------------------------------------------
// Page beyond last returns empty slice, not an error
// ----------------------------------------------------------------------------

func TestSearchCards_PageBeyondLast_EmptyNotError(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)

	f := newSearchFixture(t)
	f.seedSearchCard(t, pool, repo, "pb", f.prefix+" PageBeyond Card", "1", "Common", "pokemon", card.LanguageEnglish)

	ctx := context.Background()
	results, total, err := repo.SearchCards(ctx, postgres.SearchCardsParams{
		Q:     f.prefix + " PageBeyond Card",
		Sort:  "name",
		Order: "asc",
		Page:  999,
		Limit: 24,
	})
	if err != nil {
		t.Fatalf("SearchCards page-beyond-last: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected empty results for page=999, got %d", len(results))
	}
	// Total reflects the actual count, not the page size.
	if total < 1 {
		t.Errorf("expected total >= 1 (counting the seeded card), got %d", total)
	}
}

// ----------------------------------------------------------------------------
// No match returns empty slice (not nil), not an error
// ----------------------------------------------------------------------------

func TestSearchCards_NoMatch_EmptySliceNotNil(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)

	ctx := context.Background()
	results, total, err := repo.SearchCards(ctx, postgres.SearchCardsParams{
		Q:     "xyzzy_absolutely_no_such_card_9876543210_zz",
		Sort:  "name",
		Order: "asc",
		Page:  1,
		Limit: 24,
	})
	if err != nil {
		t.Fatalf("SearchCards no-match: %v", err)
	}
	if total != 0 {
		t.Errorf("expected total=0 for impossible query, got %d", total)
	}
	// SearchCards must return a non-nil empty slice, not nil, so JSON serializes
	// to [] instead of null.
	if results == nil {
		t.Error("expected non-nil empty slice on no-match, got nil")
	}
	if len(results) != 0 {
		t.Errorf("expected empty results, got %d", len(results))
	}
}

// ----------------------------------------------------------------------------
// release_date nullable: cards from sets without release_date scan correctly
// ----------------------------------------------------------------------------

// TestSearchCards_NullReleaseDate verifies that a set with no release_date
// does not cause a scan error. SetInfo.ReleaseDate is *time.Time and must be
// nil (not panic) when the column is NULL.
func TestSearchCards_NullReleaseDate(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)

	ctx := context.Background()
	f := newSearchFixture(t)
	code := f.prefix + "rd"
	if len(code) > 16 {
		code = code[:16]
	}

	seriesID := mustUpsertSeries(t, repo, f.prefix+"-series-rd")

	// Insert set with explicitly NULL release_date.
	var setID uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO card_sets (code, name, language, tcg, series_id, import_source)
		VALUES ($1, $2, 'en', 'pokemon', $3, 'scrydex')
		RETURNING id`,
		code, f.prefix+" Null Date Set", seriesID,
	).Scan(&setID)
	if err != nil {
		t.Fatalf("insert set with null release_date: %v", err)
	}
	t.Cleanup(func() {
		p := openTestDB(t)
		defer p.Close()
		cleanupSet(t, p, code)
	})

	c := card.Card{
		SetID:           setID,
		CollectorNumber: "1",
		Name:            f.prefix + " Null Date Card",
		ImportSource:    "scrydex",
	}
	if err := repo.UpsertCard(ctx, &c); err != nil {
		t.Fatalf("upsert card: %v", err)
	}

	results, _, err := repo.SearchCards(ctx, postgres.SearchCardsParams{
		Q:     f.prefix + " Null Date Card",
		Sort:  "name",
		Order: "asc",
		Page:  1,
		Limit: 24,
	})
	if err != nil {
		t.Fatalf("SearchCards with null release_date: %v", err)
	}
	if !findCardInResults(results, c.ID) {
		t.Errorf("card from set with null release_date not found in results")
	}
	// Verify that ReleaseDate is nil for this card (not a zero value or empty string).
	for _, r := range results {
		if r.ID == c.ID.String() {
			if r.Set.ReleaseDate != nil {
				t.Errorf("expected nil ReleaseDate for set without release_date, got %v", r.Set.ReleaseDate)
			}
		}
	}
}
