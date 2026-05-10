// Package pokewallet implements scraper.Source for TCGPlayer and Cardmarket
// prices via the pokewallet.io API (https://api.pokewallet.io).
//
// A single API call fetches both TCGPlayer and Cardmarket prices for a card.
// New() returns two scraper.Source values (one per marketplace) that share
// the same HTTP client and a request cache, so parallel fan-out in the handler
// produces only one actual API call per card.
//
// Price resolution:
//   - TCGPlayer: highest market_price sub_type → NM/LP/MP/HP/DMG via multipliers.
//   - Cardmarket: highest trend price variant → NM/LP/MP/HP/DMG via multipliers.
//
// Returns scraper.ErrNotConfigured when no API key is set.
package pokewallet

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/pricing"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper"
)

const apiBase = "https://api.pokewallet.io"

// Client is shared between the two source wrappers.
type Client struct {
	http     *http.Client
	apiKey   string
	reqCache *requestCache
}

// New creates a pokewallet.io client and returns two scraper.Source values:
// tcg exposes TCGPlayer (USD) prices, cm exposes Cardmarket (EUR) prices.
// Both are backed by a single API call per card, deduplicated via an in-process cache.
func New(apiKey string, timeout time.Duration) (tcg, cm scraper.Source) {
	c := &Client{
		http:     &http.Client{Timeout: timeout},
		apiKey:   apiKey,
		reqCache: newRequestCache(),
	}
	return &tcgSource{c}, &cmSource{c}
}

// ─── Source wrappers ─────────────────────────────────────────────────────────

type tcgSource struct{ c *Client }

func (s *tcgSource) Name() pricing.Source { return pricing.SourceTCGPlayer }
func (s *tcgSource) Search(ctx context.Context, q scraper.Query) ([]scraper.Result, error) {
	if s.c.apiKey == "" {
		return nil, scraper.ErrNotConfigured
	}
	p, err := s.c.prices(ctx, q)
	if err != nil || p == nil {
		return []scraper.Result{}, err
	}
	return p.tcg, nil
}

type cmSource struct{ c *Client }

func (s *cmSource) Name() pricing.Source { return pricing.SourceCardmarket }
func (s *cmSource) Search(ctx context.Context, q scraper.Query) ([]scraper.Result, error) {
	if s.c.apiKey == "" {
		return nil, scraper.ErrNotConfigured
	}
	p, err := s.c.prices(ctx, q)
	if err != nil || p == nil {
		return []scraper.Result{}, err
	}
	return p.cm, nil
}

// ─── Request cache ────────────────────────────────────────────────────────────

type cardPrices struct {
	tcg []scraper.Result
	cm  []scraper.Result
}

type requestCache struct {
	mu      sync.Mutex
	entries map[string]*reqEntry
}

type reqEntry struct {
	prices *cardPrices
	err    error
	done   chan struct{}
}

func newRequestCache() *requestCache {
	return &requestCache{entries: make(map[string]*reqEntry)}
}

// get deduplicates concurrent fetches for the same key. The first caller
// executes fetch(); subsequent callers block until it completes and receive
// the same result. Entries are evicted after 60 s to cap memory.
func (rc *requestCache) get(key string, fetch func() (*cardPrices, error)) (*cardPrices, error) {
	rc.mu.Lock()
	if e, ok := rc.entries[key]; ok {
		rc.mu.Unlock()
		<-e.done
		return e.prices, e.err
	}
	e := &reqEntry{done: make(chan struct{})}
	rc.entries[key] = e
	rc.mu.Unlock()

	e.prices, e.err = fetch()
	close(e.done)

	go func() {
		time.Sleep(60 * time.Second)
		rc.mu.Lock()
		delete(rc.entries, key)
		rc.mu.Unlock()
	}()

	return e.prices, e.err
}

func (c *Client) prices(ctx context.Context, q scraper.Query) (*cardPrices, error) {
	if q.Name == "" || q.Number == "" {
		return nil, nil
	}
	key := q.SetCode + "/" + q.Number
	return c.reqCache.get(key, func() (*cardPrices, error) {
		return c.fetchPrices(ctx, q)
	})
}

// ─── API types ────────────────────────────────────────────────────────────────

type searchResponse struct {
	Results []cardResult `json:"results"`
}

type cardResult struct {
	ID       string   `json:"id"`
	CardInfo cardInfo `json:"card_info"`
	TCGPlayer struct {
		Prices []tcgPrice `json:"prices"`
		URL    string     `json:"url"`
	} `json:"tcgplayer"`
	Cardmarket struct {
		ProductURL string    `json:"product_url"`
		Prices     []cmPrice `json:"prices"`
	} `json:"cardmarket"`
}

type cardInfo struct {
	Name       string `json:"name"`
	SetName    string `json:"set_name"`
	SetCode    string `json:"set_code"`
	CardNumber string `json:"card_number"`
}

type tcgPrice struct {
	SubTypeName string  `json:"sub_type_name"`
	MarketPrice float64 `json:"market_price"`
	LowPrice    float64 `json:"low_price"`
}

type cmPrice struct {
	VariantType string  `json:"variant_type"`
	Low         float64 `json:"low"`
	Trend       float64 `json:"trend"`
	Avg         float64 `json:"avg"`
}

// ─── Fetch ────────────────────────────────────────────────────────────────────

func (c *Client) fetchPrices(ctx context.Context, q scraper.Query) (*cardPrices, error) {
	card, err := c.findCard(ctx, q)
	if err != nil || card == nil {
		return nil, err
	}
	return &cardPrices{
		tcg: tcgResults(card, q.Name),
		cm:  cmResults(card, q.Name),
	}, nil
}

func (c *Client) findCard(ctx context.Context, q scraper.Query) (*cardResult, error) {
	query := q.Name + " " + stripSlash(q.Number)
	reqURL := apiBase + "/search?limit=20&q=" + url.QueryEscape(query)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("pokewallet: build request: %w", err)
	}
	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pokewallet: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pokewallet: status %d", resp.StatusCode)
	}

	var sr searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("pokewallet: decode: %w", err)
	}

	return pickBestMatch(sr.Results, q), nil
}

// pickBestMatch selects the result that best matches the query.
// Priority: set code + number (most precise) > set name contains + number > number only.
// The pokewallet set_name may carry a language prefix like "ME: Ascended Heroes",
// so we match by set_code first and fall back to a substring match on set_name.
func pickBestMatch(results []cardResult, q scraper.Query) *cardResult {
	if len(results) == 0 {
		return nil
	}
	setCodeUpper := strings.ToUpper(q.SetCode)
	setLower := strings.ToLower(q.SetName)
	num := stripSlash(q.Number)

	// Pass 1: exact set code + number
	for i, r := range results {
		if strings.ToUpper(r.CardInfo.SetCode) == setCodeUpper && stripSlash(r.CardInfo.CardNumber) == num {
			return &results[i]
		}
	}
	// Pass 2: set name contains + number (handles "ME: Ascended Heroes" prefix)
	for i, r := range results {
		if strings.Contains(strings.ToLower(r.CardInfo.SetName), setLower) && stripSlash(r.CardInfo.CardNumber) == num {
			return &results[i]
		}
	}
	// Pass 3: number only
	for i, r := range results {
		if stripSlash(r.CardInfo.CardNumber) == num {
			return &results[i]
		}
	}
	return nil
}

func stripSlash(s string) string {
	if i := strings.IndexByte(s, '/'); i >= 0 {
		return s[:i]
	}
	return s
}

// ─── Price mapping ────────────────────────────────────────────────────────────

// TCGPlayer multipliers per condition (ADR-013).
var tcgMultipliers = []struct {
	cond pricing.Condition
	pct  string
}{
	{pricing.ConditionNearMint, "1.00"},
	{pricing.ConditionLightlyPlayed, "0.80"},
	{pricing.ConditionModeratelyPlayed, "0.64"},
	{pricing.ConditionHeavilyPlayed, "0.40"},
	{pricing.ConditionDamaged, "0.24"},
}

// Cardmarket multipliers per condition (ADR-014).
var cmMultipliers = []struct {
	cond pricing.Condition
	pct  string
}{
	{pricing.ConditionNearMint, "1.00"},
	{pricing.ConditionLightlyPlayed, "0.70"},
	{pricing.ConditionModeratelyPlayed, "0.45"},
	{pricing.ConditionHeavilyPlayed, "0.25"},
	{pricing.ConditionDamaged, "0.10"},
}

func tcgResults(card *cardResult, cardName string) []scraper.Result {
	var best *tcgPrice
	for i := range card.TCGPlayer.Prices {
		p := &card.TCGPlayer.Prices[i]
		if p.MarketPrice <= 0 {
			continue
		}
		if best == nil || p.MarketPrice > best.MarketPrice {
			best = p
		}
	}
	if best == nil {
		return []scraper.Result{}
	}

	nmPrice := decimal.NewFromFloat(best.MarketPrice)
	title := cardName
	if best.SubTypeName != "" {
		title += " · " + best.SubTypeName
	}
	link := card.TCGPlayer.URL
	if link == "" {
		link = "https://www.tcgplayer.com"
	}

	results := make([]scraper.Result, 0, len(tcgMultipliers))
	for _, m := range tcgMultipliers {
		mult, _ := decimal.NewFromString(m.pct)
		price := nmPrice.Mul(mult).Round(2)
		if price.IsZero() {
			continue
		}
		results = append(results, scraper.Result{
			Title:     title,
			URL:       link,
			Price:     price,
			Currency:  pricing.CurrencyUSD,
			Condition: string(m.cond),
			Kind:      pricing.KindListing,
		})
	}
	return results
}

func cmResults(card *cardResult, cardName string) []scraper.Result {
	link := card.Cardmarket.ProductURL
	if link == "" {
		link = "https://www.cardmarket.com"
	}

	var results []scraper.Result
	for i := range card.Cardmarket.Prices {
		p := &card.Cardmarket.Prices[i]
		if p.Low <= 0 {
			continue
		}
		nmPrice := decimal.NewFromFloat(p.Low)
		title := cardName
		if p.VariantType != "" {
			title += " · " + p.VariantType
		}
		for _, m := range cmMultipliers {
			mult, _ := decimal.NewFromString(m.pct)
			price := nmPrice.Mul(mult).Round(2)
			if price.IsZero() {
				continue
			}
			results = append(results, scraper.Result{
				Title:     title,
				URL:       link,
				Price:     price,
				Currency:  pricing.CurrencyEUR,
				Condition: string(m.cond),
				Kind:      pricing.KindListing,
			})
		}
	}
	return results
}
