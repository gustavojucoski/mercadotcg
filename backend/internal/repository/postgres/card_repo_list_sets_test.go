// card_repo_list_sets_test.go
//
// Integration tests for ListSetsByTCG — the search/filter path added in
// feat/sets-public-search (PRD-004).
//
// These tests require a live PostgreSQL instance with all migrations applied.
// They skip gracefully when DATABASE_URL is not set.
//
// Run locally:
//
//	DATABASE_URL="postgres://..." go test ./internal/repository/postgres/... -run TestListSetsByTCG -v
package postgres_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/card"
	"github.com/gustavojucoski/mercadotcg/backend/internal/repository/postgres"
)

// ----------------------------------------------------------------------------
// Fixture helpers (scoped to this file — unique per test run to avoid collisions)
// ----------------------------------------------------------------------------

// listSetsFixture holds the DB state created for a test group.
// It is isolated by a unique timestamp prefix so parallel runs on the same DB
// are safe.
type listSetsFixture struct {
	prefix string // e.g. "lss42314"
}

func newListSetsFixture(t *testing.T) *listSetsFixture {
	t.Helper()
	return &listSetsFixture{
		prefix: fmt.Sprintf("lss%d", time.Now().UnixNano()%1_000_000),
	}
}

// code returns a unique set code derived from the fixture prefix and a short
// discriminator.  Set codes are VARCHAR(16) in the schema.
func (f *listSetsFixture) code(discriminator string) string {
	raw := f.prefix + discriminator
	if len(raw) > 16 {
		raw = raw[:16]
	}
	return raw
}

// seed inserts a minimal set for ListSetsByTCG testing and registers cleanup.
// It returns the inserted card.Set (with ID populated by UpsertSet).
func (f *listSetsFixture) seed(
	t *testing.T,
	repo *postgres.CardRepo,
	discriminator string,
	tcg string,
	name string,
) card.Set {
	t.Helper()

	seriesID := mustUpsertSeries(t, repo, f.prefix+"-series-"+discriminator)

	code := f.code(discriminator)
	s := card.Set{
		Code:         code,
		Name:         name,
		SeriesID:     &seriesID,
		TCG:          tcg,
		Language:     card.LanguageEnglish,
		ImportSource: "scrydex",
	}
	mustUpsertSet(t, repo, &s)

	t.Cleanup(func() {
		pool := openTestDB(t)
		defer pool.Close()
		cleanupSet(t, pool, code)
	})

	return s
}

// ----------------------------------------------------------------------------
// TestListSetsByTCG_EmptyQ — q="" returns all sets for the TCG
// ----------------------------------------------------------------------------

// TestListSetsByTCG_EmptyQ verifies that passing an empty q string returns
// every set belonging to the given TCG without applying any text filter.
//
// This is the base-case regression: if this fails the public sets page would
// show nothing on first load.
func TestListSetsByTCG_EmptyQ(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)

	f := newListSetsFixture(t)
	f.seed(t, repo, "a", "pokemon", "Alpha Set")
	f.seed(t, repo, "b", "pokemon", "Beta Set")

	ctx := context.Background()
	sets, total, err := repo.ListSetsByTCG(ctx, "pokemon", nil, "", 1, 500)
	if err != nil {
		t.Fatalf("ListSetsByTCG: %v", err)
	}

	// At least our two seeds must be present.
	if total < 2 {
		t.Errorf("expected total >= 2, got %d", total)
	}
	if len(sets) < 2 {
		t.Errorf("expected >= 2 sets returned, got %d", len(sets))
	}
}

// ----------------------------------------------------------------------------
// TestListSetsByTCG_TextSearch — q="base" matches by name
// ----------------------------------------------------------------------------

// TestListSetsByTCG_TextSearch verifies that a non-empty q filters by name
// using a case-insensitive ILIKE match, and that non-matching sets are
// excluded.
func TestListSetsByTCG_TextSearch(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)

	f := newListSetsFixture(t)
	// "base" appears in the name — should match.
	matching := f.seed(t, repo, "bm", "pokemon", f.prefix+" Base Set")
	// Unrelated name — must not appear in results.
	_ = f.seed(t, repo, "nm", "pokemon", f.prefix+" Jungle Set")

	ctx := context.Background()
	sets, total, err := repo.ListSetsByTCG(ctx, "pokemon", nil, "base", 1, 500)
	if err != nil {
		t.Fatalf("ListSetsByTCG with q=base: %v", err)
	}

	found := false
	for _, s := range sets {
		if s.ID == matching.ID {
			found = true
		}
		// Confirm the non-matching seed does not appear.
		if s.Name == f.prefix+" Jungle Set" {
			t.Errorf("non-matching set %q appeared in results for q=base", s.Name)
		}
	}
	if !found {
		t.Errorf("expected matching set %q in results, not found (total=%d)", matching.Name, total)
	}
}

// ----------------------------------------------------------------------------
// TestListSetsByTCG_CodeSearch — q matches the code column too
// ----------------------------------------------------------------------------

// TestListSetsByTCG_CodeSearch verifies that the text filter also applies to
// cs.code, not just cs.name.  A set whose name does not contain the query
// string but whose code does must still be returned.
func TestListSetsByTCG_CodeSearch(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)

	f := newListSetsFixture(t)

	// Produce a set whose code contains "base" but whose name does not.
	// Fixture prefix is ≤9 chars (lss######), so appending "base" gives ≤13 chars < 16.
	customCode := f.prefix + "base"
	if len(customCode) > 16 {
		customCode = customCode[:16]
	}

	ctx := context.Background()
	seriesID := mustUpsertSeries(t, repo, f.prefix+"-series-bsc")
	codeSet := card.Set{
		Code:         customCode,
		Name:         f.prefix + " Unrelated Name",
		SeriesID:     &seriesID,
		TCG:          "pokemon",
		Language:     card.LanguageEnglish,
		ImportSource: "scrydex",
	}
	mustUpsertSet(t, repo, &codeSet)
	t.Cleanup(func() {
		p := openTestDB(t)
		defer p.Close()
		cleanupSet(t, p, customCode)
	})

	sets, _, err := repo.ListSetsByTCG(ctx, "pokemon", nil, "base", 1, 500)
	if err != nil {
		t.Fatalf("ListSetsByTCG with q=base (code match): %v", err)
	}

	found := false
	for _, s := range sets {
		if s.ID == codeSet.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected set matching by code to appear in results, but not found")
	}
}

// ----------------------------------------------------------------------------
// TestListSetsByTCG_LiteralPercent — q="%" is not a wildcard
// ----------------------------------------------------------------------------

// TestListSetsByTCG_LiteralPercent verifies that a q of "%" is treated as a
// literal percent sign after escapeLikePattern runs in the handler.  At the
// repo layer this test passes an already-escaped pattern ("\%") — mirroring
// what the handler delivers.
//
// If the escape were missing, passing "%" would match every row and the test
// would see all sets — a false pass.  The correct expectation is zero rows
// (assuming no set name or code literally contains a percent sign).
func TestListSetsByTCG_LiteralPercent(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)

	f := newListSetsFixture(t)
	// Seed a normal set to confirm it does NOT appear.
	_ = f.seed(t, repo, "pct", "pokemon", f.prefix+" Normal Set Pct")

	ctx := context.Background()
	// Simulate the escaped string the handler produces for raw input "%".
	escaped := `\%`
	sets, total, err := repo.ListSetsByTCG(ctx, "pokemon", nil, escaped, 1, 500)
	if err != nil {
		t.Fatalf("ListSetsByTCG with q=%%: %v", err)
	}

	// The seeded set name contains no literal "%", so it must not appear.
	for _, s := range sets {
		if s.Name == f.prefix+" Normal Set Pct" {
			t.Errorf("normal set appeared for literal-percent query (total=%d) — escape is broken", total)
		}
	}
}

// ----------------------------------------------------------------------------
// TestListSetsByTCG_LiteralUnderscore — q="_" matches only sets with "_"
// ----------------------------------------------------------------------------

// TestListSetsByTCG_LiteralUnderscore verifies that "_" in the query is
// treated as a literal underscore, not as a single-char LIKE wildcard.
func TestListSetsByTCG_LiteralUnderscore(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)

	f := newListSetsFixture(t)
	_ = f.seed(t, repo, "usc", "pokemon", f.prefix+" UnderscoreSet")

	ctx := context.Background()
	// Handler escapes "_" → "\_".
	escaped := `\_`
	sets, _, err := repo.ListSetsByTCG(ctx, "pokemon", nil, escaped, 1, 500)
	if err != nil {
		t.Fatalf("ListSetsByTCG with q=_: %v", err)
	}

	// Our seeded set name contains no literal "_", so it must not appear.
	for _, s := range sets {
		if s.Name == f.prefix+" UnderscoreSet" {
			t.Errorf("set without underscore appeared for literal-underscore query — escape is broken")
		}
	}
}

// ----------------------------------------------------------------------------
// TestListSetsByTCG_LiteralBackslash — q="\" matches only sets containing "\"
// ----------------------------------------------------------------------------

// TestListSetsByTCG_LiteralBackslash verifies that a backslash in the search
// input does not corrupt the ESCAPE clause of the LIKE predicate.
// The handler converts "\" to "\\" before passing to the repo.
func TestListSetsByTCG_LiteralBackslash(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)

	f := newListSetsFixture(t)
	_ = f.seed(t, repo, "bsl", "pokemon", f.prefix+" BackslashSet")

	ctx := context.Background()
	// Handler escapes "\" → "\\".
	escaped := `\\`
	sets, _, err := repo.ListSetsByTCG(ctx, "pokemon", nil, escaped, 1, 500)
	if err != nil {
		t.Fatalf("ListSetsByTCG with q=backslash: %v", err)
	}

	// Our seeded set name contains no literal "\", so it must not appear.
	for _, s := range sets {
		if s.Name == f.prefix+" BackslashSet" {
			t.Errorf("set without backslash appeared for literal-backslash query — escape is broken")
		}
	}
}

// ----------------------------------------------------------------------------
// TestListSetsByTCG_Pagination — page/limit slicing and accurate total
// ----------------------------------------------------------------------------

// TestListSetsByTCG_Pagination verifies that:
//   - page=1 limit=2 returns exactly 2 rows when 3+ exist,
//   - total reflects the full count, not the page size,
//   - page=2 limit=2 returns the remaining rows,
//   - the two pages together cover all seeded IDs without duplicates.
func TestListSetsByTCG_Pagination(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)

	f := newListSetsFixture(t)
	s1 := f.seed(t, repo, "p1", "pokemon", f.prefix+" Page Set One")
	s2 := f.seed(t, repo, "p2", "pokemon", f.prefix+" Page Set Two")
	s3 := f.seed(t, repo, "p3", "pokemon", f.prefix+" Page Set Three")
	seededIDs := map[string]bool{
		s1.ID.String(): true,
		s2.ID.String(): true,
		s3.ID.String(): true,
	}

	ctx := context.Background()

	// Use a prefix-scoped q so only our 3 seeds match, independent of other
	// sets already in the database.
	q := f.prefix + " Page Set"

	page1, total1, err := repo.ListSetsByTCG(ctx, "pokemon", nil, q, 1, 2)
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if total1 != 3 {
		t.Errorf("expected total=3, got %d", total1)
	}
	if len(page1) != 2 {
		t.Errorf("expected 2 sets on page 1, got %d", len(page1))
	}

	page2, total2, err := repo.ListSetsByTCG(ctx, "pokemon", nil, q, 2, 2)
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if total2 != 3 {
		t.Errorf("expected total=3 on page 2, got %d", total2)
	}
	if len(page2) != 1 {
		t.Errorf("expected 1 set on page 2, got %d", len(page2))
	}

	// Confirm no duplicates across pages.
	seen := map[string]bool{}
	for _, s := range append(page1, page2...) {
		id := s.ID.String()
		if seen[id] {
			t.Errorf("duplicate set ID across pages: %s", id)
		}
		seen[id] = true
		delete(seededIDs, id)
	}
	for id := range seededIDs {
		t.Errorf("seeded set %s missing from paginated results", id)
	}
}

// ----------------------------------------------------------------------------
// TestListSetsByTCG_TCGIsolation — pokemon query never returns magic sets
// ----------------------------------------------------------------------------

// TestListSetsByTCG_TCGIsolation verifies the WHERE cs.tcg = $1 predicate:
// sets belonging to a different TCG must not bleed into results even when
// their names match the search query.
func TestListSetsByTCG_TCGIsolation(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)

	f := newListSetsFixture(t)
	// Seed the same name prefix in two different TCGs.
	pokemonSet := f.seed(t, repo, "tpi", "pokemon", f.prefix+" Shared Name Set")
	magicSet := f.seed(t, repo, "tmi", "magic", f.prefix+" Shared Name Set")

	ctx := context.Background()
	q := f.prefix + " Shared Name"

	sets, _, err := repo.ListSetsByTCG(ctx, "pokemon", nil, q, 1, 500)
	if err != nil {
		t.Fatalf("ListSetsByTCG TCG isolation: %v", err)
	}

	for _, s := range sets {
		if s.ID == magicSet.ID {
			t.Error("magic set appeared in pokemon query — TCG isolation is broken")
		}
	}

	found := false
	for _, s := range sets {
		if s.ID == pokemonSet.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("pokemon set did not appear in pokemon query — TCG filter is over-filtering")
	}
}

// ----------------------------------------------------------------------------
// TestListSetsByTCG_PageBeyondLast — empty result when page exceeds data
// ----------------------------------------------------------------------------

// TestListSetsByTCG_PageBeyondLast verifies that requesting a page beyond the
// last page returns an empty slice (not an error).  total must still reflect
// the actual count so the caller knows further pages do not exist.
func TestListSetsByTCG_PageBeyondLast(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)

	f := newListSetsFixture(t)
	_ = f.seed(t, repo, "oob", "pokemon", f.prefix+" OOB Set")

	ctx := context.Background()
	q := f.prefix + " OOB"

	sets, total, err := repo.ListSetsByTCG(ctx, "pokemon", nil, q, 999, 10)
	if err != nil {
		t.Fatalf("ListSetsByTCG page-beyond-last: %v", err)
	}
	if len(sets) != 0 {
		t.Errorf("expected empty result for page 999, got %d sets", len(sets))
	}
	// total should still be 1 (the one seeded set).
	if total != 1 {
		t.Errorf("expected total=1, got %d", total)
	}
}

// ----------------------------------------------------------------------------
// TestListSetsByTCG_CaseInsensitive — ILIKE is truly case-insensitive
// ----------------------------------------------------------------------------

// TestListSetsByTCG_CaseInsensitive verifies that the ILIKE predicate matches
// regardless of case in the query string.
func TestListSetsByTCG_CaseInsensitive(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)

	f := newListSetsFixture(t)
	matched := f.seed(t, repo, "ci", "pokemon", f.prefix+" Base Set CI")

	ctx := context.Background()
	for _, q := range []string{"BASE", "base", "Base", "bAsE"} {
		sets, _, err := repo.ListSetsByTCG(ctx, "pokemon", nil, q, 1, 500)
		if err != nil {
			t.Fatalf("ListSetsByTCG q=%q: %v", q, err)
		}
		found := false
		for _, s := range sets {
			if s.ID == matched.ID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("q=%q: expected set to appear but it did not", q)
		}
	}
}

// ----------------------------------------------------------------------------
// TestListSetsByTCG_NoMatch — no matching sets returns empty slice, not error
// ----------------------------------------------------------------------------

// TestListSetsByTCG_NoMatch verifies that a query matching no rows returns an
// empty (non-nil) slice and zero total — never an error.  The handler wraps
// nil to [] before responding, but the repo itself must not error.
func TestListSetsByTCG_NoMatch(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()
	repo := postgres.NewCardRepo(pool)

	ctx := context.Background()
	// Use a query string that is astronomically unlikely to match any row.
	q := "xyzzy_no_such_set_12345_zzzz"
	sets, total, err := repo.ListSetsByTCG(ctx, "pokemon", nil, q, 1, 30)
	if err != nil {
		t.Fatalf("ListSetsByTCG no-match: unexpected error: %v", err)
	}
	if total != 0 {
		t.Errorf("expected total=0, got %d", total)
	}
	if len(sets) != 0 {
		t.Errorf("expected empty result, got %d sets", len(sets))
	}
}
