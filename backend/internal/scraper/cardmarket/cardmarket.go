// Package cardmarket implements scraper.Source for Cardmarket
// (https://www.cardmarket.com) via HTML scraping.
//
// Strategy:
//  1. Requires ExternalID — the full Cardmarket card URL from pokemontcg.io
//     (e.g. "https://www.cardmarket.com/en/Pokemon/Products/Singles/...").
//  2. Fetches the card product page.
//     - When FlareSolverr is configured: routes through FlareSolverr to bypass
//       Cloudflare's Managed Challenge (JS-based bot detection).
//     - Without FlareSolverr: direct HTTP, returns empty on 403.
//  3. Parses the article listing table for per-condition prices.
//     Cardmarket conditions: NM, EX, GD, LP, PO → mapped to NM/LP/MP/HP/DMG.
//  4. Returns cheapest listing per condition, ordered NM→LP→MP→HP→DMG.
//
// The handler falls back to pokemontcg.io trendPrice + condition multipliers
// when this scraper returns empty (see handler/external.go ADR-014).
package cardmarket

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/shopspring/decimal"

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/pricing"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper"
)

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

// Client is the Cardmarket scraper.
type Client struct {
	http            *http.Client
	flareSolverrURL string // e.g. "http://localhost:8191" — empty = direct HTTP
}

// New creates a client that fetches Cardmarket directly (will 403 on Cloudflare).
func New(timeout time.Duration) *Client {
	return &Client{http: &http.Client{Timeout: timeout}}
}

// NewWithFlareSolverr creates a client that routes requests through FlareSolverr
// to bypass Cloudflare. flareSolverrURL is e.g. "http://localhost:8191".
// The timeout applies to the FlareSolverr HTTP call (should be ≥ 65s to match
// FlareSolverr's 60s maxTimeout).
func NewWithFlareSolverr(timeout time.Duration, flareSolverrURL string) *Client {
	return &Client{
		http:            &http.Client{Timeout: timeout},
		flareSolverrURL: strings.TrimRight(flareSolverrURL, "/"),
	}
}

// Name implements scraper.Source.
func (c *Client) Name() pricing.Source { return pricing.SourceCardmarket }

// Search implements scraper.Source.
//
// ExternalID must be the full Cardmarket card URL (pokemontcg.io cardmarket.url).
// Returns empty list when ExternalID is empty or bot protection blocks the request.
func (c *Client) Search(ctx context.Context, q scraper.Query) ([]scraper.Result, error) {
	if q.ExternalID == "" {
		return []scraper.Result{}, nil
	}

	body, err := c.fetchBody(ctx, q.ExternalID)
	if err != nil {
		return nil, fmt.Errorf("cardmarket: %w", err)
	}

	entries := parseArticles(body)
	return cheapestPerCondition(entries, q.Name, q.ExternalID), nil
}

// ─── HTTP ─────────────────────────────────────────────────────────────────────

// fetchBody fetches the target page via FlareSolverr (when configured) or
// direct HTTP. Returns ("", nil) on 403/503 so callers treat it as empty.
func (c *Client) fetchBody(ctx context.Context, target string) (string, error) {
	if c.flareSolverrURL != "" {
		return c.fetchViaFlareSolverr(ctx, target)
	}
	return c.fetchDirect(ctx, target)
}

func (c *Client) fetchDirect(ctx context.Context, target string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "identity")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 403 || resp.StatusCode == 503 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return "", nil // Cloudflare block → empty, no error
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return "", fmt.Errorf("status %d em %s", resp.StatusCode, target)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ler body: %w", err)
	}
	return string(b), nil
}

// flareSolverrRequest é o corpo do POST para o endpoint /v1 do FlareSolverr.
type flareSolverrRequest struct {
	Cmd        string `json:"cmd"`
	URL        string `json:"url"`
	MaxTimeout int    `json:"maxTimeout"` // ms
}

// flareSolverrResponse é a resposta do FlareSolverr.
type flareSolverrResponse struct {
	Status   string `json:"status"`
	Solution struct {
		Status   int    `json:"status"`
		Response string `json:"response"`
	} `json:"solution"`
}

// fetchViaFlareSolverr fetches the target page through FlareSolverr, which
// runs an undetected browser to solve Cloudflare challenges.
func (c *Client) fetchViaFlareSolverr(ctx context.Context, target string) (string, error) {
	// Derive maxTimeout from context deadline so FlareSolverr doesn't overshoot.
	maxTimeout := 60_000 // 60s default
	if dl, ok := ctx.Deadline(); ok {
		remaining := int(time.Until(dl).Milliseconds()) - 2_000
		if remaining > 0 && remaining < maxTimeout {
			maxTimeout = remaining
		}
	}

	body, err := json.Marshal(flareSolverrRequest{
		Cmd:        "request.get",
		URL:        target,
		MaxTimeout: maxTimeout,
	})
	if err != nil {
		return "", fmt.Errorf("flaresolverr: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.flareSolverrURL+"/v1", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("flaresolverr: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("flaresolverr: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return "", fmt.Errorf("flaresolverr: status %d", resp.StatusCode)
	}

	var fsResp flareSolverrResponse
	if err := json.NewDecoder(resp.Body).Decode(&fsResp); err != nil {
		return "", fmt.Errorf("flaresolverr: decode: %w", err)
	}
	if fsResp.Status != "ok" {
		return "", fmt.Errorf("flaresolverr: status=%q", fsResp.Status)
	}
	if fsResp.Solution.Status < 200 || fsResp.Solution.Status >= 300 {
		return "", fmt.Errorf("flaresolverr: target status %d", fsResp.Solution.Status)
	}

	return fsResp.Solution.Response, nil
}

// ─── Parse ────────────────────────────────────────────────────────────────────

type articleEntry struct {
	condition    string // our Condition constant
	rawCondition string // original CM label (NM, EX, GD, LP, PO)
	price        decimal.Decimal
}

// selectorStrategy is one CSS-selector combination to find article rows.
type selectorStrategy struct {
	row, condition, price string
}

// strategies are tried in order; first one that yields any articles wins.
var strategies = []selectorStrategy{
	// Cardmarket post-2024 Vue UI
	{"div.article-row", "div.product-comments a", "div.price-container span.color-primary"},
	{"div.article-row", "div.article-condition a", "div.price-container span.color-primary"},
	// Older/alternative UI
	{"article.single-card", "span.condition-label", "span.article-unit-price"},
	{"div.single-card", "span.condition-label", "span.article-unit-price"},
	// Generic fallback: any table row that has condition text + price
	{"tr.article", "td.condition", "td.price span"},
}

func parseArticles(body string) []articleEntry {
	if body == "" {
		return nil
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil
	}

	for _, s := range strategies {
		entries := extractWithStrategy(doc, s)
		if len(entries) > 0 {
			return entries
		}
	}
	return nil
}

func extractWithStrategy(doc *goquery.Document, s selectorStrategy) []articleEntry {
	var entries []articleEntry
	doc.Find(s.row).Each(func(_ int, row *goquery.Selection) {
		condEl := row.Find(s.condition).First()

		// Prefer title/data-original-title attributes for the condition label.
		condStr, _ := condEl.Attr("data-original-title")
		if condStr == "" {
			condStr, _ = condEl.Attr("title")
		}
		if condStr == "" {
			condStr = strings.TrimSpace(condEl.Text())
		}

		priceStr := strings.TrimSpace(row.Find(s.price).First().Text())

		price, err := parseEURPrice(priceStr)
		if err != nil || price.IsZero() {
			return
		}

		cond := mapCMCondition(condStr)
		if cond == "" {
			return
		}

		entries = append(entries, articleEntry{
			condition:    cond,
			rawCondition: strings.ToUpper(strings.TrimSpace(condStr)),
			price:        price,
		})
	})
	return entries
}

// ─── cheapestPerCondition ─────────────────────────────────────────────────────

func cheapestPerCondition(entries []articleEntry, cardName, cardURL string) []scraper.Result {
	best := make(map[string]articleEntry)
	for _, e := range entries {
		if prev, ok := best[e.condition]; !ok || e.price.LessThan(prev.price) {
			best[e.condition] = e
		}
	}

	order := []string{
		string(pricing.ConditionNearMint),
		string(pricing.ConditionLightlyPlayed),
		string(pricing.ConditionModeratelyPlayed),
		string(pricing.ConditionHeavilyPlayed),
		string(pricing.ConditionDamaged),
	}

	results := make([]scraper.Result, 0, len(best))
	for _, cond := range order {
		e, ok := best[cond]
		if !ok {
			continue
		}
		title := "Cardmarket"
		if cardName != "" {
			title = cardName + " · " + e.rawCondition
		}
		results = append(results, scraper.Result{
			Title:        title,
			URL:          cardURL,
			Price:        e.price,
			Currency:     pricing.CurrencyEUR,
			Kind:         pricing.KindListing,
			Condition:    cond,
			RawCondition: e.rawCondition,
		})
	}
	return results
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// mapCMCondition maps a Cardmarket condition label to our Condition constants.
// CM conditions: MT/NM → NM, EX → LP, GD → MP, LP/PL → HP, PO → DMG.
func mapCMCondition(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "MT", "MINT", "NM", "NEAR MINT":
		return string(pricing.ConditionNearMint)
	case "EX", "EXCELLENT":
		return string(pricing.ConditionLightlyPlayed)
	case "GD", "GOOD":
		return string(pricing.ConditionModeratelyPlayed)
	case "LP", "LIGHTLY PLAYED", "PL", "PLAYED":
		return string(pricing.ConditionHeavilyPlayed)
	case "PO", "POOR":
		return string(pricing.ConditionDamaged)
	}
	return ""
}

// parseEURPrice parses European-formatted price strings like "24,80 €" or "1.234,56 €".
func parseEURPrice(s string) (decimal.Decimal, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "€", "")
	s = strings.TrimSpace(s)
	if s == "" {
		return decimal.Zero, fmt.Errorf("empty price")
	}
	// European format: thousands='.', decimal=','
	if strings.Contains(s, ",") {
		s = strings.ReplaceAll(s, ".", "")
		s = strings.ReplaceAll(s, ",", ".")
	}
	s = strings.TrimSpace(s)
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero, err
	}
	return d, nil
}
