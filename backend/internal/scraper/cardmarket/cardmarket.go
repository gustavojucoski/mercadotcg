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
	"strconv"
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
// When ExternalID is empty and FlareSolverr is configured, resolution proceeds in order:
//  1. Special-illustration rares (card number > set.printedTotal): URL is constructed
//     directly as {SetSlug}/{CardSlug}-{SetCode}{Number} — no set listing fetch needed.
//     e.g. Pikachu ex SIR 276 → "…/Ascended-Heroes/Pikachu-ex-ASC276"
//  2. Regular cards: set listing is fetched and the slug with matching number is found.
//
// When the base URL returns no article listings, Search retries automatically with
// V-number suffixes (V1…V10), stopping at the first version that yields results.
// This covers cards that Cardmarket splits into multiple product pages by version.
//
// Returns empty list when the URL cannot be found or bot protection blocks the request.
func (c *Client) Search(ctx context.Context, q scraper.Query) ([]scraper.Result, error) {
	target := q.ExternalID
	if target == "" {
		target = c.resolveTarget(ctx, q)
	}
	if target == "" {
		return []scraper.Result{}, nil
	}

	for attempt := 0; attempt <= 10; attempt++ {
		if ctx.Err() != nil {
			break
		}
		url := target
		if attempt > 0 {
			url = injectVersion(target, q.SetCode, attempt)
		}

		body, err := c.fetchBody(ctx, url)
		if err != nil {
			if attempt == 0 {
				return nil, fmt.Errorf("cardmarket: %w", err)
			}
			break // context likely exhausted
		}

		entries := parseArticles(body)
		if len(entries) > 0 {
			return cheapestPerCondition(entries, q.Name, url), nil
		}
	}
	return []scraper.Result{}, nil
}

// injectVersion inserts "-V{n}" into the card-page URL slug just before the
// "-{SETCODE}" marker. If the set code is absent from the slug (e.g. ExternalID
// from pokemontcg.io that omits set code+number), it appends "-V{n}" at the end.
//
// Examples:
//
//	".../Pikachu-ex-ASC276"  + setCode="ASC" + n=1  →  ".../Pikachu-ex-V1-ASC276"
//	".../Charizard-VMAX"     + setCode="SSH" + n=2  →  ".../Charizard-VMAX-V2"
func injectVersion(target, setCode string, n int) string {
	slash := strings.LastIndex(target, "/")
	if slash < 0 {
		return fmt.Sprintf("%s-V%d", target, n)
	}
	base := target[:slash+1]
	slug := target[slash+1:]

	if setCode != "" {
		marker := "-" + strings.ToUpper(setCode)
		if idx := strings.Index(strings.ToUpper(slug), marker); idx >= 0 {
			return base + slug[:idx] + fmt.Sprintf("-V%d", n) + slug[idx:]
		}
	}
	return base + slug + fmt.Sprintf("-V%d", n)
}

// resolveTarget picks the right Cardmarket product URL for q when ExternalID is absent.
// For SIR cards the URL is constructed directly (saves one FlareSolverr call).
// For regular cards the set listing is scraped to find the matching slug.
func (c *Client) resolveTarget(ctx context.Context, q scraper.Query) string {
	if c.flareSolverrURL == "" || q.Name == "" || q.SetName == "" {
		return ""
	}

	// SIR shortcut: cards numbered above the set's printed total are special-illustration
	// rares. Cardmarket slugs for SIRs follow {CardName}-{SetCode}{Number} with no V-prefix.
	// e.g. Pikachu ex 276/217 → "Pikachu-ex-ASC276"
	if q.SetCode != "" && q.SetPrintedTotal > 0 {
		if cn := parseCardNumber(q.Number); cn > q.SetPrintedTotal {
			numStr := q.Number
			if i := strings.IndexByte(numStr, '/'); i >= 0 {
				numStr = numStr[:i]
			}
			setSlug := toCardmarketSlug(q.SetName)
			cardSlug := toCardmarketSlug(q.Name)
			return "https://www.cardmarket.com/en/Pokemon/Products/Singles/" +
				setSlug + "/" + cardSlug + "-" + strings.ToUpper(q.SetCode) + strings.TrimSpace(numStr)
		}
	}

	return c.resolveFromSetListing(ctx, q)
}

// resolveFromSetListing finds the Cardmarket card URL by fetching the set listing
// page and searching for a link that matches the card name.
// Only works when FlareSolverr is configured. Returns "" on any failure.
//
// When multiple versions of the same card exist in the set (e.g. regular + SIR
// Pikachu ex), it uses q.Number and q.SetPrintedTotal to pick the right one:
//   - card number > printed total → special rare (SIR/IR/etc.) → highest V-number
//   - card number ≤ printed total → regular → lowest V-number (V1)
//
// If q.SetPrintedTotal is 0 and there are multiple matches, returns "" to avoid
// returning the wrong version.
func (c *Client) resolveFromSetListing(ctx context.Context, q scraper.Query) string {
	if c.flareSolverrURL == "" || q.Name == "" || q.SetName == "" {
		return ""
	}

	setSlug := toCardmarketSlug(q.SetName)
	setURL := "https://www.cardmarket.com/en/Pokemon/Products/Singles/" + setSlug

	html, err := c.fetchViaFlareSolverr(ctx, setURL)
	if err != nil || html == "" {
		return ""
	}

	cardSlug := strings.ToLower(toCardmarketSlug(q.Name))
	linkPrefix := "/en/Pokemon/Products/Singles/" + setSlug + "/"

	// Collect distinct slugs (lowercase) matching the card name prefix.
	// The same href appears many times in the listing HTML, so deduplicate.
	seen := make(map[string]struct{})
	idx := 0
	for {
		pos := strings.Index(html[idx:], linkPrefix)
		if pos < 0 {
			break
		}
		pos += idx
		start := pos + len(linkPrefix)
		end := strings.IndexByte(html[start:], '"')
		if end < 0 {
			break
		}
		slug := html[start : start+end]
		if strings.HasPrefix(strings.ToLower(slug), cardSlug) {
			seen[strings.ToLower(slug)] = struct{}{}
		}
		idx = pos + 1
	}

	// Priority 1: exact card-number match.
	// Cardmarket slugs encode the official card number: "Pikachu-ex-V3-ASC276".
	// Matching "ASC276" directly is far more reliable than any V-number heuristic.
	var chosen string
	if cardNum := parseCardNumber(q.Number); cardNum > 0 {
		for slug := range seen {
			if slugContainsNumber(slug, q.SetCode, cardNum) {
				chosen = slug
				break
			}
		}
	}

	// Priority 2: fallback when the exact number isn't on this page.
	if chosen == "" {
		switch len(seen) {
		case 0:
			return ""
		case 1:
			for s := range seen {
				chosen = s
			}
			// Single match but not the exact number we want — could be a different
			// version of the card (e.g. regular V1 while the SIR V3 is on a later page).
			// If the card is a special rare and what we found is V1, skip it to avoid
			// returning the cheap regular version instead of the SIR.
			if q.SetPrintedTotal > 0 && extractVNumber(chosen) == 1 {
				if cn := parseCardNumber(q.Number); cn > q.SetPrintedTotal {
					return ""
				}
			}
		default:
			chosen = pickByVNumber(seen, q.Number, q.SetPrintedTotal)
			if chosen == "" {
				return ""
			}
		}
	}

	orig := findSlugOrigCase(html, linkPrefix, chosen)
	return "https://www.cardmarket.com" + linkPrefix + orig
}

// parseCardNumber converts "276" or "276/217" to 276.
func parseCardNumber(s string) int {
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

// slugContainsNumber reports whether the Cardmarket slug (lowercase) encodes the
// given card number for the given set code.
// e.g. slugContainsNumber("pikachu-ex-v3-asc276", "ASC", 276) → true
//      slugContainsNumber("pikachu-ex-v1-asc057", "ASC", 276) → false
// Handles zero-padded slugs ("asc057" matches cardNum=57).
func slugContainsNumber(slug, setCode string, cardNum int) bool {
	prefix := strings.ToLower(setCode)
	idx := strings.LastIndex(slug, prefix)
	if idx < 0 {
		return false
	}
	n, err := strconv.Atoi(slug[idx+len(prefix):])
	return err == nil && n == cardNum
}

// pickByVNumber selects the slug with the appropriate Cardmarket V-number:
//   - special rare (cardNumber > printedTotal) → highest V (rarest version)
//   - regular card (cardNumber ≤ printedTotal) → lowest V (most common version)
//
// Returns "" if printedTotal is 0 (unknown) or no slugs have parseable V-numbers.
func pickByVNumber(seen map[string]struct{}, cardNumber string, printedTotal int) string {
	if printedTotal == 0 {
		return ""
	}
	cardNum := parseCardNumber(cardNumber)
	if cardNum == 0 {
		return ""
	}

	specialRare := cardNum > printedTotal

	best := ""
	bestV := 0
	for slug := range seen {
		v := extractVNumber(slug)
		if v == 0 {
			continue
		}
		if best == "" {
			best = slug
			bestV = v
			continue
		}
		if specialRare && v > bestV {
			best = slug
			bestV = v
		} else if !specialRare && v < bestV {
			best = slug
			bestV = v
		}
	}
	return best
}

// extractVNumber parses the V-number from a Cardmarket slug.
// e.g. "pikachu-ex-v3-asc276" → 3, "mega-dragonite-ex-v1-asc152" → 1.
func extractVNumber(slug string) int {
	for _, part := range strings.Split(slug, "-") {
		if len(part) >= 2 && (part[0] == 'v' || part[0] == 'V') {
			if n, err := strconv.Atoi(part[1:]); err == nil && n > 0 {
				return n
			}
		}
	}
	return 0
}

// findSlugOrigCase returns the original-cased slug from the HTML that lowercases to lowerSlug.
func findSlugOrigCase(html, linkPrefix, lowerSlug string) string {
	idx := 0
	for {
		pos := strings.Index(html[idx:], linkPrefix)
		if pos < 0 {
			break
		}
		pos += idx
		start := pos + len(linkPrefix)
		end := strings.IndexByte(html[start:], '"')
		if end < 0 {
			break
		}
		slug := html[start : start+end]
		if strings.ToLower(slug) == lowerSlug {
			return slug
		}
		idx = pos + 1
	}
	return lowerSlug
}

// toCardmarketSlug converts a name like "Ascended Heroes" to the Cardmarket
// URL slug "Ascended-Heroes". Handles apostrophes, ampersands, and accented chars.
func toCardmarketSlug(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == ' ':
			b.WriteRune('-')
		case r == '\'', r == '’': // apostrophes
			// skip
		case r == '&':
			// skip
		case r == 'é':
			b.WriteRune('e')
		case r == 'ü':
			b.WriteRune('u')
		default:
			b.WriteRune(r)
		}
	}
	// collapse consecutive hyphens
	result := b.String()
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}
	return strings.Trim(result, "-")
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
	// Treat 404/503 as "not found" — same as fetchDirect does for 403/503.
	// A constructed SIR URL for a card not yet listed on CM returns 404; callers
	// handle empty body as empty results.
	s := fsResp.Solution.Status
	if s == 404 || s == 503 {
		return "", nil
	}
	if s < 200 || s >= 300 {
		return "", fmt.Errorf("flaresolverr: target status %d", s)
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
//
// Confirmed against live Cardmarket HTML (2025-05):
//   <div id="articleRow{ID}" class="row g-0 article-row">
//     …
//     <a class="article-condition condition-nm me-1" …>
//       <span class="badge ">NM</span>
//     </a>
//     …
//     <div class="price-container …">
//       <span class="color-primary … fw-bold ">1,00 €</span>
//     </div>
//   </div>
var strategies = []selectorStrategy{
	// Cardmarket 2024+ Bootstrap UI (confirmed live)
	{"div.article-row", "a.article-condition span.badge", "div.price-container span.color-primary"},
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
