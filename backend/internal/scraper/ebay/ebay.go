// Package ebay implements scraper.Source for eBay using the official
// Browse API (OAuth2 client_credentials flow).
//
// Strategy:
//  1. POST https://api.ebay.com/identity/v1/oauth2/token  → bearer token
//  2. GET  https://api.ebay.com/buy/browse/v1/item_summary/search
//         ?q=QUERY&category_ids=183454&limit=N
//  3. Converts itemSummaries to []scraper.Result.
//
// Without clientID/certID the scraper returns ErrNotConfigured.
// Register at https://developer.ebay.com to get free credentials.
package ebay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/pricing"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper"
)

const (
	ebayTokenURL  = "https://api.ebay.com/identity/v1/oauth2/token"
	ebaySearchURL = "https://api.ebay.com/buy/browse/v1/item_summary/search"
	ebayScope     = "https://api.ebay.com/oauth/api_scope"
	pokemonCatID  = "183454" // Pokemon Individual Cards
	userAgent     = "Mozilla/5.0 (compatible; MercadoTCG/1.0)"
)

// Client é o scraper eBay via Browse API.
type Client struct {
	http     *http.Client
	clientID string
	certID   string

	mu          sync.Mutex
	cachedToken string
	tokenExpiry time.Time
}

// New monta o client. clientID e certID vêm de EBAY_CLIENT_ID / EBAY_CLIENT_SECRET.
// Se vazios, Search retorna ErrNotConfigured.
func New(timeout time.Duration, clientID, certID string) *Client {
	return &Client{
		http:     &http.Client{Timeout: timeout},
		clientID: clientID,
		certID:   certID,
	}
}

// Name implementa scraper.Source.
func (c *Client) Name() pricing.Source { return pricing.SourceEbay }

// Search implementa scraper.Source.
func (c *Client) Search(ctx context.Context, q scraper.Query) ([]scraper.Result, error) {
	if c.clientID == "" || c.certID == "" {
		return nil, scraper.ErrNotConfigured
	}
	if q.Name == "" {
		return nil, errors.New("ebay: name obrigatório")
	}

	token, err := c.getToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("ebay: autenticação: %w", err)
	}

	limit := q.Limit
	if limit <= 0 {
		limit = 10
	}

	queryStr := strings.TrimSpace(q.Name)
	if q.Number != "" {
		queryStr += " " + q.Number
	}
	if !strings.Contains(strings.ToLower(queryStr), "pokemon") {
		queryStr += " pokemon card"
	}

	params := url.Values{}
	params.Set("q", queryStr)
	params.Set("category_ids", pokemonCatID)
	params.Set("limit", fmt.Sprintf("%d", limit))
	params.Set("sort", "price")
	searchURL := ebaySearchURL + "?" + params.Encode()

	items, err := c.fetchItems(ctx, searchURL, token)
	if err != nil {
		return nil, err
	}

	results := convertItems(items)
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// ─── OAuth2 token ─────────────────────────────────────────────────────────────

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

func (c *Client) getToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cachedToken != "" && time.Now().Before(c.tokenExpiry) {
		return c.cachedToken, nil
	}

	body := url.Values{}
	body.Set("grant_type", "client_credentials")
	body.Set("scope", ebayScope)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ebayTokenURL, strings.NewReader(body.Encode()))
	if err != nil {
		return "", fmt.Errorf("build token request: %w", err)
	}
	req.SetBasicAuth(c.clientID, c.certID)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token status %d: %s", resp.StatusCode, string(b))
	}

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", fmt.Errorf("decode token: %w", err)
	}

	c.cachedToken = tr.AccessToken
	// Renova 60s antes do vencimento para evitar race conditions.
	c.tokenExpiry = time.Now().Add(time.Duration(tr.ExpiresIn-60) * time.Second)
	return c.cachedToken, nil
}

// ─── Browse API ───────────────────────────────────────────────────────────────

type browseResponse struct {
	Total        int           `json:"total"`
	ItemSummaries []browseItem `json:"itemSummaries"`
}

type browseItem struct {
	ItemID     string     `json:"itemId"`
	Title      string     `json:"title"`
	Price      browsePrice `json:"price"`
	ItemWebURL string     `json:"itemWebUrl"`
	Image      struct {
		ImageURL string `json:"imageUrl"`
	} `json:"image"`
	Condition   string `json:"condition"`
	ConditionID string `json:"conditionId"`
}

type browsePrice struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

func (c *Client) fetchItems(ctx context.Context, searchURL, token string) ([]browseItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("ebay: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("X-EBAY-C-MARKETPLACE-ID", "EBAY_US")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ebay: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ebay: status %d: %s", resp.StatusCode, string(b))
	}

	var br browseResponse
	if err := json.NewDecoder(resp.Body).Decode(&br); err != nil {
		return nil, fmt.Errorf("ebay: decode: %w", err)
	}
	return br.ItemSummaries, nil
}

// ─── Conversão ────────────────────────────────────────────────────────────────

func convertItems(items []browseItem) []scraper.Result {
	results := make([]scraper.Result, 0, len(items))
	seen := make(map[string]bool)

	for _, item := range items {
		if item.Title == "" || seen[item.ItemWebURL] {
			continue
		}
		price, currency, ok := parseBrowsePrice(item.Price)
		if !ok {
			continue
		}
		seen[item.ItemWebURL] = true

		externalID := extractItemID(item.ItemID)
		condition := normalizeCondition(item.Condition)

		results = append(results, scraper.Result{
			Title:        item.Title,
			URL:          item.ItemWebURL,
			ImageURL:     item.Image.ImageURL,
			Price:        price,
			Currency:     currency,
			Kind:         pricing.KindListing,
			Condition:    condition,
			RawCondition: item.Condition,
			ExternalID:   externalID,
		})
	}
	return results
}

func parseBrowsePrice(p browsePrice) (decimal.Decimal, pricing.Currency, bool) {
	if p.Value == "" {
		return decimal.Zero, "", false
	}
	d, err := decimal.NewFromString(p.Value)
	if err != nil || d.IsZero() {
		return decimal.Zero, "", false
	}
	currency := pricing.CurrencyUSD
	switch strings.ToUpper(p.Currency) {
	case "EUR":
		currency = pricing.CurrencyEUR
	case "JPY":
		currency = pricing.CurrencyJPY
	case "BRL":
		currency = pricing.CurrencyBRL
	}
	return d, currency, true
}

func normalizeCondition(raw string) string {
	r := strings.ToUpper(strings.TrimSpace(raw))
	switch {
	case strings.Contains(r, "BRAND NEW"), strings.Contains(r, "NEAR MINT"), r == "NM":
		return string(pricing.ConditionNearMint)
	case strings.Contains(r, "LIGHTLY"), strings.Contains(r, "LIGHT PLAY"), r == "LP":
		return string(pricing.ConditionLightlyPlayed)
	case strings.Contains(r, "MODERATELY"), strings.Contains(r, "MOD PLAY"), r == "MP":
		return string(pricing.ConditionModeratelyPlayed)
	case strings.Contains(r, "HEAVILY"), strings.Contains(r, "HEAVY PLAY"), r == "HP":
		return string(pricing.ConditionHeavilyPlayed)
	case strings.Contains(r, "DAMAGED"), r == "DMG", r == "POOR":
		return string(pricing.ConditionDamaged)
	case strings.Contains(r, "GOOD"), strings.Contains(r, "VERY GOOD"):
		return string(pricing.ConditionLightlyPlayed)
	}
	return ""
}

// extractItemID converte "v1|403791900150|0" → "403791900150".
func extractItemID(raw string) string {
	parts := strings.Split(raw, "|")
	if len(parts) >= 2 {
		return parts[1]
	}
	return raw
}
