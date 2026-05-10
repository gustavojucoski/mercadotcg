// Package tcgplayer implements scraper.Source for TCGplayer using the
// undocumented mpapi pricepoints endpoint — no API key required.
//
// Strategy:
//  1. Requires ExternalID in the Query (the TCGplayer product ID, e.g. "676088").
//     The product ID is the numeric segment in every TCGplayer product URL:
//     https://www.tcgplayer.com/product/676088/Pokemon-...
//  2. GET https://mpapi.tcgplayer.com/v2/product/{id}/pricepoints
//  3. Returns one Result per printingType that has a non-null marketPrice.
//
// Without ExternalID the scraper returns ErrNotConfigured — TCGplayer's
// search page is a client-side SPA with no public search API.
package tcgplayer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/pricing"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper"
)

const (
	mpapiBase = "https://mpapi.tcgplayer.com"
	tcgBase   = "https://www.tcgplayer.com"
	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
)

// Client é o scraper TCGplayer.
type Client struct {
	http    *http.Client
	apiBase string
}

// New monta o client.
func New(timeout time.Duration) *Client {
	return &Client{
		http:    &http.Client{Timeout: timeout},
		apiBase: mpapiBase,
	}
}

// Name implementa scraper.Source.
func (c *Client) Name() pricing.Source { return pricing.SourceTCGPlayer }

// Search implementa scraper.Source.
//
// Requer q.ExternalID com o product ID do TCGplayer (ex: "676088").
// Sem ExternalID retorna lista vazia — o card não tem product ID disponível via pokemontcg.io.
// conditionMultipliers são os fatores padrão que o TCGPlayer aplica sobre o
// preço NM para derivar o valor em cada condição.
var conditionMultipliers = []struct {
	condition string
	factor    float64
}{
	{"NM", 1.00},
	{"LP", 0.80},
	{"MP", 0.64},
	{"HP", 0.40},
	{"DMG", 0.24},
}

func (c *Client) Search(ctx context.Context, q scraper.Query) ([]scraper.Result, error) {
	if q.ExternalID == "" {
		return []scraper.Result{}, nil
	}

	priceURL := fmt.Sprintf("%s/v2/product/%s/pricepoints", c.apiBase, q.ExternalID)
	productURL := fmt.Sprintf("%s/product/%s", tcgBase, q.ExternalID)

	points, err := c.fetchPricepoints(ctx, priceURL)
	if err != nil {
		return nil, fmt.Errorf("tcgplayer: pricepoints: %w", err)
	}

	// O marketPrice da pricepoints API representa o preço de mercado NM.
	// Derivamos LP/MP/HP/DMG aplicando os multiplicadores padrão do TCGPlayer.
	var results []scraper.Result
	for _, p := range points {
		nmPrice := pickPrice(p)
		if nmPrice.IsZero() {
			continue
		}
		for _, cond := range conditionMultipliers {
			factor := decimal.NewFromFloat(cond.factor)
			price := nmPrice.Mul(factor).Round(2)
			title := buildTitle(q.Name, p.PrintingType, cond.condition)
			results = append(results, scraper.Result{
				Title:        title,
				URL:          productURL,
				Price:        price,
				Currency:     pricing.CurrencyUSD,
				Kind:         pricing.KindListing,
				Condition:    cond.condition,
				RawCondition: cond.condition,
				ExternalID:   q.ExternalID,
			})
		}
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("tcgplayer: nenhum preço disponível para product %s", q.ExternalID)
	}

	limit := q.Limit
	if limit <= 0 || limit > len(results) {
		limit = len(results)
	}
	return results[:limit], nil
}

// ─── Pricepoints API ─────────────────────────────────────────────────────────

type pricepoint struct {
	PrintingType       string   `json:"printingType"`
	MarketPrice        *float64 `json:"marketPrice"`
	BuylistMarketPrice *float64 `json:"buylistMarketPrice"`
	ListedMedianPrice  *float64 `json:"listedMedianPrice"`
}

func (c *Client) fetchPricepoints(ctx context.Context, url string) ([]pricepoint, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", tcgBase+"/")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("status %d em %s", resp.StatusCode, url)
	}

	var points []pricepoint
	if err := json.NewDecoder(resp.Body).Decode(&points); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return points, nil
}

// pickPrice prefere marketPrice → listedMedianPrice → buylistMarketPrice.
func pickPrice(p pricepoint) decimal.Decimal {
	for _, v := range []*float64{p.MarketPrice, p.ListedMedianPrice, p.BuylistMarketPrice} {
		if v != nil && *v > 0 {
			return decimal.NewFromFloat(*v)
		}
	}
	return decimal.Zero
}

func buildTitle(name, printingType, condition string) string {
	parts := []string{}
	if name != "" {
		parts = append(parts, name)
	}
	if printingType != "" && !strings.EqualFold(printingType, "Normal") {
		parts = append(parts, printingType)
	}
	if condition != "" {
		parts = append(parts, condition)
	}
	if len(parts) == 0 {
		return "TCGplayer"
	}
	return strings.Join(parts, " · ")
}
