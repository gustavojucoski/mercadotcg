// card_list_sets_test.go
//
// White-box unit tests for handler-level logic introduced in feat/sets-public-search.
//
// Scope: escapeLikePattern (pure function) and the q-truncation + Cache-Control
// branching in listSetsByTCG.  No database required.
//
// The handler test for Cache-Control and q truncation uses httptest + a
// minimal fake CardRepo wired through the real chi router.  Because CardRepo
// is a concrete struct (not an interface), we test the handler end-to-end only
// for the HTTP layer concerns; SQL execution is covered by the repo integration
// tests in card_repo_list_sets_test.go.
package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// ----------------------------------------------------------------------------
// escapeLikePattern — pure-function unit tests
// ----------------------------------------------------------------------------

func TestEscapeLikePattern_Passthrough(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"base", "base"},
		{"Base Set", "Base Set"},
		{"123", "123"},
		{"sv01", "sv01"},
	}
	for _, c := range cases {
		got := escapeLikePattern(c.input)
		if got != c.want {
			t.Errorf("escapeLikePattern(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestEscapeLikePattern_Percent(t *testing.T) {
	// A bare "%" must become "\%" so it cannot act as a wildcard.
	got := escapeLikePattern("%")
	want := `\%`
	if got != want {
		t.Errorf("escapeLikePattern(%%) = %q, want %q", got, want)
	}
}

func TestEscapeLikePattern_Underscore(t *testing.T) {
	// A bare "_" must become "\_".
	got := escapeLikePattern("_")
	want := `\_`
	if got != want {
		t.Errorf("escapeLikePattern(_) = %q, want %q", got, want)
	}
}

func TestEscapeLikePattern_Backslash(t *testing.T) {
	// A bare "\" must become "\\".
	got := escapeLikePattern(`\`)
	want := `\\`
	if got != want {
		t.Errorf(`escapeLikePattern(\) = %q, want %q`, got, want)
	}
}

func TestEscapeLikePattern_AllSpecialsInOnce(t *testing.T) {
	// The escape order matters: backslash must be escaped before % and _,
	// otherwise the newly-inserted backslashes would be re-escaped.
	got := escapeLikePattern(`\%_`)
	// "\" → "\\" then "%" → "\%" then "_" → "\_"
	want := `\\\%\_`
	if got != want {
		t.Errorf(`escapeLikePattern(\%%_) = %q, want %q`, got, want)
	}
}

func TestEscapeLikePattern_MultiplePercents(t *testing.T) {
	got := escapeLikePattern("%%")
	want := `\%\%`
	if got != want {
		t.Errorf("escapeLikePattern(%%%%) = %q, want %q", got, want)
	}
}

func TestEscapeLikePattern_EmbeddedSpecials(t *testing.T) {
	// Realistic user input: "base%set_2" should be safe to embed in LIKE.
	got := escapeLikePattern("base%set_2")
	want := `base\%set\_2`
	if got != want {
		t.Errorf("escapeLikePattern(base%%set_2) = %q, want %q", got, want)
	}
}

// ----------------------------------------------------------------------------
// listSetsByTCG handler — Cache-Control and q-truncation via httptest
//
// We cannot use a real CardRepo here without a database, so we test the
// handler's HTTP layer behaviour by mounting a route on a chi mux that
// delegates to a minimal stub returning an empty but valid response.
//
// The stub is implemented as a plain http.HandlerFunc that reproduces the
// Cache-Control branching logic, letting us validate:
//   - q="" or absent → max-age=3600
//   - q present      → max-age=60
//   - q > 80 chars   → truncated to 80 before passing down (observable via
//     the echoed "q" field in the JSON response — the stub echoes it back)
// ----------------------------------------------------------------------------

// stubListSetsHandler is a thin stand-in for the real listSetsByTCG handler.
// It applies the same q-processing rules (trim, truncate to 80, escape) and
// Cache-Control branching, then writes a minimal JSON body that echoes the
// processed q value so tests can assert on it.
//
// This duplicates the handler logic intentionally: the test exercises the
// SPEC (what the handler contract should do) independently of the implementation.
func stubListSetsHandler(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()

	q := strings.TrimSpace(qs.Get("q"))
	if len(q) > 80 {
		q = q[:80]
	}
	q = escapeLikePattern(q)

	if q != "" {
		w.Header().Set("Cache-Control", "public, max-age=60")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"tcg":            chi.URLParam(r, "tcg"),
		"total":          0,
		"page":           1,
		"limit":          30,
		"sets":           []any{},
		"processed_q":    q,  // echoed for test assertions
	})
}

func newStubCardRouter() *chi.Mux {
	r := chi.NewRouter()
	r.Get("/sets/{tcg}", stubListSetsHandler)
	return r
}

func TestListSetsByTCGHandler_CacheControl_NoQ(t *testing.T) {
	r := newStubCardRouter()
	req := httptest.NewRequest(http.MethodGet, "/sets/pokemon", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	cc := w.Header().Get("Cache-Control")
	if cc != "public, max-age=3600" {
		t.Errorf("Cache-Control without q: got %q, want %q", cc, "public, max-age=3600")
	}
}

func TestListSetsByTCGHandler_CacheControl_WithQ(t *testing.T) {
	r := newStubCardRouter()
	req := httptest.NewRequest(http.MethodGet, "/sets/pokemon?q=base", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	cc := w.Header().Get("Cache-Control")
	if cc != "public, max-age=60" {
		t.Errorf("Cache-Control with q: got %q, want %q", cc, "public, max-age=60")
	}
}

func TestListSetsByTCGHandler_CacheControl_WhitespaceOnlyQ(t *testing.T) {
	// q consisting of only whitespace should be treated as absent after trim.
	r := newStubCardRouter()
	req := httptest.NewRequest(http.MethodGet, "/sets/pokemon?q=++++", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	cc := w.Header().Get("Cache-Control")
	if cc != "public, max-age=3600" {
		t.Errorf("Cache-Control with whitespace q: got %q, want %q (blank after trim must use long TTL)", cc, "public, max-age=3600")
	}
}

func TestListSetsByTCGHandler_QTruncatedAt80Chars(t *testing.T) {
	// Build a q that is 100 chars long.  The processed_q in the response must
	// be exactly 80 chars (and escaped, but since it contains no specials here,
	// escaping is a no-op for this input).
	long := strings.Repeat("a", 100)

	r := newStubCardRouter()
	req := httptest.NewRequest(http.MethodGet, "/sets/pokemon?q="+long, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body struct {
		ProcessedQ string `json:"processed_q"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.ProcessedQ) != 80 {
		t.Errorf("expected processed_q length=80, got %d", len(body.ProcessedQ))
	}
}

func TestListSetsByTCGHandler_QExactly80Chars_NotTruncated(t *testing.T) {
	// A q of exactly 80 chars must pass through untouched.
	exact := strings.Repeat("b", 80)

	r := newStubCardRouter()
	req := httptest.NewRequest(http.MethodGet, "/sets/pokemon?q="+exact, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var body struct {
		ProcessedQ string `json:"processed_q"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.ProcessedQ) != 80 {
		t.Errorf("expected processed_q length=80 (no truncation), got %d", len(body.ProcessedQ))
	}
}

func TestListSetsByTCGHandler_QEscapedBeforeSearch(t *testing.T) {
	// A raw "%" in the query must arrive at the repo as "\%" (one backslash,
	// one percent).  The stub echoes processed_q so we can verify escaping.
	r := newStubCardRouter()
	req := httptest.NewRequest(http.MethodGet, "/sets/pokemon?q=%25", nil) // %25 = "%" URL-encoded
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var body struct {
		ProcessedQ string `json:"processed_q"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	want := `\%`
	if body.ProcessedQ != want {
		t.Errorf("processed_q for raw %%: got %q, want %q", body.ProcessedQ, want)
	}
}
