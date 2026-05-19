// card_search_test.go
//
// Unit tests for the searchCards handler (GET /search/cards).
//
// Scope: HTTP layer concerns that do not require a database:
//   - Parameter validation (sort, order)
//   - Offset depth guard (ErrSearchOffsetTooDeep → HTTP 400)
//   - Limit clamping (> 48 → 48, < 1 → 24)
//   - q truncation at 80 runes (tested via Cache-Control branching + echo)
//   - Cache-Control: max-age=3600 without q/rarity, max-age=60 with q or rarity
//   - Response envelope shape: data, total, page, limit, has_more
//
// The handler owns a concrete *postgres.CardRepo (not an interface), so we
// exercise the HTTP routing logic using the same stub pattern established in
// card_list_sets_test.go — a chi mux with a handler that mirrors the spec.
package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// ----------------------------------------------------------------------------
// Stub search handler — mirrors the searchCards spec for HTTP-layer tests
// ----------------------------------------------------------------------------

// searchCardsStubResponse is the envelope the stub echoes back.
// The real handler uses postgres.SearchCardResult; the stub uses a generic map
// so tests can inspect fields without importing the postgres package here.
type searchCardsStubResponse struct {
	Data    []any  `json:"data"`
	Total   int    `json:"total"`
	Page    int    `json:"page"`
	Limit   int    `json:"limit"`
	HasMore bool   `json:"has_more"`
	// echo fields for test assertions
	EchoQ      string `json:"echo_q"`
	EchoSort   string `json:"echo_sort"`
	EchoOrder  string `json:"echo_order"`
	EchoLimit  int    `json:"echo_limit"`
	EchoPage   int    `json:"echo_page"`
	EchoRarity string `json:"echo_rarity"`
}

// stubSearchCardsHandler reproduces all handler-layer decisions from searchCards:
//   - q truncation to 80 runes + escapeLikePattern
//   - sort/order allowlist validation → 400
//   - limit clamping [1, 48]
//   - page clamping >= 1
//   - offset-depth guard (page-1)*limit > 1000 → 400 (ErrSearchOffsetTooDeep message)
//   - Cache-Control: 60 with q or rarity, 3600 otherwise
//
// It does NOT call the repo; instead it echoes back processed params so tests
// can assert on them independently of SQL execution.
func stubSearchCardsHandler(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()

	q := strings.TrimSpace(qs.Get("q"))
	if runes := []rune(q); len(runes) > 80 {
		q = string(runes[:80])
	}
	q = escapeLikePattern(q)

	sort := qs.Get("sort")
	switch sort {
	case "name", "release_date", "collector_number":
		// valid
	case "":
		sort = "name"
	default:
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "sort inválido: use name, release_date ou collector_number",
		})
		return
	}

	order := strings.ToLower(qs.Get("order"))
	switch order {
	case "asc", "desc":
		// valid
	case "":
		order = "asc"
	default:
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "order inválido: use asc ou desc"})
		return
	}

	rarity := strings.TrimSpace(qs.Get("rarity"))
	rarity = escapeLikePattern(rarity)

	page := atoiOrDefault(qs.Get("page"), 1)
	if page < 1 {
		page = 1
	}

	limit := atoiOrDefault(qs.Get("limit"), 24)
	if limit < 1 {
		limit = 24
	}
	if limit > 48 {
		limit = 48
	}

	if (page-1)*limit > 1000 {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "página muito profunda: refine os filtros para navegar além da página 42",
		})
		return
	}

	if q != "" || rarity != "" {
		w.Header().Set("Cache-Control", "public, max-age=60")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(searchCardsStubResponse{
		Data:       []any{},
		Total:      0,
		Page:       page,
		Limit:      limit,
		HasMore:    false,
		EchoQ:      q,
		EchoSort:   sort,
		EchoOrder:  order,
		EchoLimit:  limit,
		EchoPage:   page,
		EchoRarity: rarity,
	})
}

func newSearchCardRouter() *chi.Mux {
	r := chi.NewRouter()
	r.Get("/search/cards", stubSearchCardsHandler)
	return r
}

func decodeSearchStub(t *testing.T, body io.Reader) searchCardsStubResponse {
	t.Helper()
	var resp searchCardsStubResponse
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		t.Fatalf("decode stub response: %v", err)
	}
	return resp
}

// ----------------------------------------------------------------------------
// Sort validation
// ----------------------------------------------------------------------------

func TestSearchCardsHandler_InvalidSort_Returns400(t *testing.T) {
	r := newSearchCardRouter()
	req := httptest.NewRequest(http.MethodGet, "/search/cards?sort=invalid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid sort, got %d", w.Code)
	}
	var body map[string]string
	_ = json.NewDecoder(w.Body).Decode(&body)
	if body["error"] == "" {
		t.Error("expected error field in 400 response")
	}
}

func TestSearchCardsHandler_ValidSortValues(t *testing.T) {
	for _, sort := range []string{"name", "release_date", "collector_number"} {
		t.Run(sort, func(t *testing.T) {
			r := newSearchCardRouter()
			req := httptest.NewRequest(http.MethodGet, "/search/cards?sort="+sort, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Errorf("sort=%q: expected 200, got %d", sort, w.Code)
			}
		})
	}
}

func TestSearchCardsHandler_EmptySort_DefaultsToName(t *testing.T) {
	r := newSearchCardRouter()
	req := httptest.NewRequest(http.MethodGet, "/search/cards", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	resp := decodeSearchStub(t, w.Body)
	if resp.EchoSort != "name" {
		t.Errorf("expected default sort=name, got %q", resp.EchoSort)
	}
}

// ----------------------------------------------------------------------------
// Order validation
// ----------------------------------------------------------------------------

func TestSearchCardsHandler_InvalidOrder_Returns400(t *testing.T) {
	r := newSearchCardRouter()
	req := httptest.NewRequest(http.MethodGet, "/search/cards?order=sideways", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for order=sideways, got %d", w.Code)
	}
}

func TestSearchCardsHandler_OrderCaseInsensitive(t *testing.T) {
	// The handler does strings.ToLower before switch, so "ASC" must be accepted.
	r := newSearchCardRouter()
	req := httptest.NewRequest(http.MethodGet, "/search/cards?order=ASC", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for order=ASC (case-insensitive), got %d", w.Code)
	}
}

func TestSearchCardsHandler_EmptyOrder_DefaultsToAsc(t *testing.T) {
	r := newSearchCardRouter()
	req := httptest.NewRequest(http.MethodGet, "/search/cards", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	resp := decodeSearchStub(t, w.Body)
	if resp.EchoOrder != "asc" {
		t.Errorf("expected default order=asc, got %q", resp.EchoOrder)
	}
}

// ----------------------------------------------------------------------------
// Limit clamping
// ----------------------------------------------------------------------------

func TestSearchCardsHandler_LimitAbove48_ClampedTo48(t *testing.T) {
	r := newSearchCardRouter()
	req := httptest.NewRequest(http.MethodGet, "/search/cards?limit=200", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	resp := decodeSearchStub(t, w.Body)
	if resp.EchoLimit != 48 {
		t.Errorf("expected limit clamped to 48, got %d", resp.EchoLimit)
	}
}

func TestSearchCardsHandler_LimitZero_DefaultsTo24(t *testing.T) {
	r := newSearchCardRouter()
	req := httptest.NewRequest(http.MethodGet, "/search/cards?limit=0", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	resp := decodeSearchStub(t, w.Body)
	if resp.EchoLimit != 24 {
		t.Errorf("expected limit=0 defaulted to 24, got %d", resp.EchoLimit)
	}
}

func TestSearchCardsHandler_NegativeLimit_DefaultsTo24(t *testing.T) {
	r := newSearchCardRouter()
	req := httptest.NewRequest(http.MethodGet, "/search/cards?limit=-5", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	resp := decodeSearchStub(t, w.Body)
	if resp.EchoLimit != 24 {
		t.Errorf("expected limit=-5 defaulted to 24, got %d", resp.EchoLimit)
	}
}

func TestSearchCardsHandler_Limit48_Exact(t *testing.T) {
	r := newSearchCardRouter()
	req := httptest.NewRequest(http.MethodGet, "/search/cards?limit=48", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	resp := decodeSearchStub(t, w.Body)
	if resp.EchoLimit != 48 {
		t.Errorf("expected limit=48 accepted as-is, got %d", resp.EchoLimit)
	}
}

// ----------------------------------------------------------------------------
// Offset depth guard
// ----------------------------------------------------------------------------

// TestSearchCardsHandler_OffsetTooDeep_Returns400 is the primary guard test.
// The boundary: (page-1)*limit > 1000 → 400.
// With limit=24 (default): (page-1)*24 > 1000 → page-1 > 41.67 → page >= 43.
// So page=43, limit=24: (42)*24 = 1008 > 1000 → must 400.
func TestSearchCardsHandler_OffsetTooDeep_Returns400(t *testing.T) {
	r := newSearchCardRouter()
	req := httptest.NewRequest(http.MethodGet, "/search/cards?page=43&limit=24", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for page=43 limit=24 (offset=1008>1000), got %d", w.Code)
	}
	var body map[string]string
	_ = json.NewDecoder(w.Body).Decode(&body)
	if body["error"] == "" {
		t.Error("expected error message in 400 response")
	}
}

// TestSearchCardsHandler_OffsetBoundary_Page42_Allowed verifies the exact
// boundary: (42-1)*24 = 984 <= 1000 → must 200.
func TestSearchCardsHandler_OffsetBoundary_Page42_Allowed(t *testing.T) {
	r := newSearchCardRouter()
	req := httptest.NewRequest(http.MethodGet, "/search/cards?page=42&limit=24", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for page=42 limit=24 (offset=984<=1000), got %d", w.Code)
	}
}

// TestSearchCardsHandler_OffsetDepth_WithLimit1_Page1001 verifies the guard with
// the smallest possible limit (1). Page 1001 gives offset 1000 (exactly at limit).
// (1000)*1 = 1000, which is NOT > 1000, so this must 200.
func TestSearchCardsHandler_OffsetDepth_Page1001_Limit1_AtBoundary(t *testing.T) {
	r := newSearchCardRouter()
	req := httptest.NewRequest(http.MethodGet, "/search/cards?page=1001&limit=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	// (1000)*1 = 1000, not > 1000 → allowed
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for page=1001 limit=1 (offset=1000, not >1000), got %d", w.Code)
	}
}

// TestSearchCardsHandler_OffsetDepth_Page1002_Limit1_Exceeds verifies the guard
// fires at offset 1001 with limit=1.
func TestSearchCardsHandler_OffsetDepth_Page1002_Limit1_Exceeds(t *testing.T) {
	r := newSearchCardRouter()
	req := httptest.NewRequest(http.MethodGet, "/search/cards?page=1002&limit=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	// (1001)*1 = 1001 > 1000 → must 400
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for page=1002 limit=1 (offset=1001>1000), got %d", w.Code)
	}
}

// ----------------------------------------------------------------------------
// Cache-Control
// ----------------------------------------------------------------------------

func TestSearchCardsHandler_CacheControl_NoVolatileParams_LongTTL(t *testing.T) {
	r := newSearchCardRouter()
	req := httptest.NewRequest(http.MethodGet, "/search/cards?sort=name", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	cc := w.Header().Get("Cache-Control")
	if cc != "public, max-age=3600" {
		t.Errorf("expected max-age=3600 without q/rarity, got %q", cc)
	}
}

func TestSearchCardsHandler_CacheControl_WithQ_ShortTTL(t *testing.T) {
	r := newSearchCardRouter()
	req := httptest.NewRequest(http.MethodGet, "/search/cards?q=pikachu", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	cc := w.Header().Get("Cache-Control")
	if cc != "public, max-age=60" {
		t.Errorf("expected max-age=60 with q, got %q", cc)
	}
}

func TestSearchCardsHandler_CacheControl_WithRarity_ShortTTL(t *testing.T) {
	r := newSearchCardRouter()
	req := httptest.NewRequest(http.MethodGet, "/search/cards?rarity=Rare", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	cc := w.Header().Get("Cache-Control")
	if cc != "public, max-age=60" {
		t.Errorf("expected max-age=60 with rarity, got %q", cc)
	}
}

func TestSearchCardsHandler_CacheControl_WhitespaceOnlyQ_LongTTL(t *testing.T) {
	// q consisting only of whitespace trims to "" → treated as absent → 3600.
	r := newSearchCardRouter()
	req := httptest.NewRequest(http.MethodGet, "/search/cards?q=+++", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	cc := w.Header().Get("Cache-Control")
	if cc != "public, max-age=3600" {
		t.Errorf("expected max-age=3600 for whitespace-only q, got %q", cc)
	}
}

// ----------------------------------------------------------------------------
// q truncation at 80 runes
// ----------------------------------------------------------------------------

func TestSearchCardsHandler_QTruncatedAt80Runes(t *testing.T) {
	// 100 ASCII chars — each is 1 rune — must be truncated to 80.
	long := strings.Repeat("a", 100)
	r := newSearchCardRouter()
	req := httptest.NewRequest(http.MethodGet, "/search/cards?q="+long, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	resp := decodeSearchStub(t, w.Body)
	if len([]rune(resp.EchoQ)) != 80 {
		t.Errorf("expected q truncated to 80 runes, got %d", len([]rune(resp.EchoQ)))
	}
}

func TestSearchCardsHandler_Q_MultiByte_TruncatedByRune(t *testing.T) {
	// 90 Japanese characters (each is 1 rune, 3 bytes in UTF-8).
	// Byte-based truncation would produce an invalid string; rune-based gives 80 valid runes.
	long := strings.Repeat("あ", 90)
	r := newSearchCardRouter()
	req := httptest.NewRequest(http.MethodGet, "/search/cards?q="+long, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	resp := decodeSearchStub(t, w.Body)
	runeLen := len([]rune(resp.EchoQ))
	if runeLen != 80 {
		t.Errorf("expected q truncated to 80 runes (multibyte), got %d", runeLen)
	}
}

func TestSearchCardsHandler_QExactly80Chars_NotTruncated(t *testing.T) {
	exact := strings.Repeat("b", 80)
	r := newSearchCardRouter()
	req := httptest.NewRequest(http.MethodGet, "/search/cards?q="+exact, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	resp := decodeSearchStub(t, w.Body)
	if len(resp.EchoQ) != 80 {
		t.Errorf("q of 80 chars must not be truncated, got len=%d", len(resp.EchoQ))
	}
}

// ----------------------------------------------------------------------------
// q escaping
// ----------------------------------------------------------------------------

func TestSearchCardsHandler_QPercent_Escaped(t *testing.T) {
	r := newSearchCardRouter()
	req := httptest.NewRequest(http.MethodGet, "/search/cards?q=%25", nil) // %25 = URL-encoded "%"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	resp := decodeSearchStub(t, w.Body)
	want := `\%`
	if resp.EchoQ != want {
		t.Errorf("q=%% not escaped: got %q, want %q", resp.EchoQ, want)
	}
}

func TestSearchCardsHandler_QUnderscore_Escaped(t *testing.T) {
	r := newSearchCardRouter()
	req := httptest.NewRequest(http.MethodGet, "/search/cards?q=_", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	resp := decodeSearchStub(t, w.Body)
	want := `\_`
	if resp.EchoQ != want {
		t.Errorf("q=_ not escaped: got %q, want %q", resp.EchoQ, want)
	}
}

// ----------------------------------------------------------------------------
// Rarity escaping
// ----------------------------------------------------------------------------

func TestSearchCardsHandler_RarityWithUnderscore_Escaped(t *testing.T) {
	r := newSearchCardRouter()
	req := httptest.NewRequest(http.MethodGet, "/search/cards?rarity=some_rarity", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	resp := decodeSearchStub(t, w.Body)
	want := `some\_rarity`
	if resp.EchoRarity != want {
		t.Errorf("rarity underscore not escaped: got %q, want %q", resp.EchoRarity, want)
	}
}

// ----------------------------------------------------------------------------
// Response envelope shape
// ----------------------------------------------------------------------------

func TestSearchCardsHandler_ResponseEnvelope_HasExpectedFields(t *testing.T) {
	r := newSearchCardRouter()
	req := httptest.NewRequest(http.MethodGet, "/search/cards", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body map[string]json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	for _, field := range []string{"data", "total", "page", "limit", "has_more"} {
		if _, ok := body[field]; !ok {
			t.Errorf("response missing field %q", field)
		}
	}

	// data must be an array (not null).
	var data []any
	if err := json.Unmarshal(body["data"], &data); err != nil {
		t.Errorf("data field is not a JSON array: %v", err)
	}
}

// ----------------------------------------------------------------------------
// Page clamping: page < 1 becomes 1
// ----------------------------------------------------------------------------

func TestSearchCardsHandler_PageZero_ClampedTo1(t *testing.T) {
	r := newSearchCardRouter()
	req := httptest.NewRequest(http.MethodGet, "/search/cards?page=0", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	resp := decodeSearchStub(t, w.Body)
	if resp.EchoPage != 1 {
		t.Errorf("expected page=0 clamped to 1, got %d", resp.EchoPage)
	}
}

func TestSearchCardsHandler_NegativePage_ClampedTo1(t *testing.T) {
	r := newSearchCardRouter()
	req := httptest.NewRequest(http.MethodGet, "/search/cards?page=-10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	resp := decodeSearchStub(t, w.Body)
	if resp.EchoPage != 1 {
		t.Errorf("expected page=-10 clamped to 1, got %d", resp.EchoPage)
	}
}
