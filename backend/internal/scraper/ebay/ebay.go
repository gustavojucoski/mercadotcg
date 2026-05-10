// Package ebay implements scraper.Source for eBay using Scrydex as data source.
//
// Strategy:
//  1. Requires ExternalID in the Query — the pokemontcg.io card ID (e.g. "me2pt5-290").
//  2. GET https://scrydex.com/pokemon/cards/x/{card-id}
//     The slug segment is ignored by Scrydex; the card ID is what matters.
//  3. Parses recent eBay sold prices embedded as data attributes in the HTML:
//     data-company, data-grade, data-price, data-currency, data-sold-at.
//  4. Groups by company+grade (e.g. "PSA 10", "BGS 9.5") and returns the
//     lowest sold price per group, ordered by grade descending.
//
// No credentials required — Scrydex is a public price tracker.
package ebay

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/pricing"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper"
)

const (
	scrydexBase = "https://scrydex.com"
	userAgent   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
	rowSep      = `data-sales-filter-target="row"`
)

var (
	reCompany  = regexp.MustCompile(`data-company="([^"]*)"`)
	reGrade    = regexp.MustCompile(`data-grade="([^"]*)"`)
	rePrice    = regexp.MustCompile(`data-price="([^"]*)"`)
	reCurrency = regexp.MustCompile(`data-currency="([^"]*)"`)
	reSoldAt   = regexp.MustCompile(`data-sold-at="(\d+)"`)
	reEbayURL  = regexp.MustCompile(`href="(https://www\.ebay\.com/itm/[^"]+)"`)
	reLinkText = regexp.MustCompile(`href="https://www\.ebay\.com/itm/[^"]*"[^>]*>([^<]+)</a>`)
)

// Client é o scraper eBay via Scrydex.
type Client struct {
	http    *http.Client
	baseURL string
}

// New cria o client. Não requer credenciais.
func New(timeout time.Duration) *Client {
	return &Client{
		http:    &http.Client{Timeout: timeout},
		baseURL: scrydexBase,
	}
}

// Name implementa scraper.Source.
func (c *Client) Name() pricing.Source { return pricing.SourceEbay }

// Search implementa scraper.Source.
//
// Requer q.ExternalID com o pokemontcg.io card ID (ex: "me2pt5-290").
// Sem ExternalID retorna lista vazia — sem ID não há como buscar no Scrydex.
func (c *Client) Search(ctx context.Context, q scraper.Query) ([]scraper.Result, error) {
	if q.ExternalID == "" {
		return []scraper.Result{}, nil
	}

	pageURL := fmt.Sprintf("%s/pokemon/cards/x/%s", c.baseURL, q.ExternalID)
	body, err := c.fetchBody(ctx, pageURL)
	if err != nil {
		return nil, fmt.Errorf("ebay: scrydex: %w", err)
	}

	sales := parseSales(body)
	results := cheapestPerGrade(sales, q.Name, pageURL)
	return results, nil
}

// ─── HTTP ─────────────────────────────────────────────────────────────────────

func (c *Client) fetchBody(ctx context.Context, target string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

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

// ─── Parse ────────────────────────────────────────────────────────────────────

type saleEntry struct {
	company  string
	grade    string
	price    decimal.Decimal
	currency pricing.Currency
	soldAt   int64  // milliseconds unix
	ebayURL  string
	title    string
}

// parseSales extrai todas as entradas de vendas recentes do HTML do Scrydex.
func parseSales(html string) []saleEntry {
	parts := strings.Split(html, rowSep)
	if len(parts) < 2 {
		return nil
	}

	entries := make([]saleEntry, 0, len(parts)-1)
	for _, chunk := range parts[1:] {
		company := extract(reCompany, chunk)
		grade := extract(reGrade, chunk)
		priceStr := extract(rePrice, chunk)
		currStr := extract(reCurrency, chunk)
		soldAtStr := extract(reSoldAt, chunk)

		if company == "" || grade == "" || priceStr == "" {
			continue
		}

		price, err := decimal.NewFromString(priceStr)
		if err != nil || price.IsZero() {
			continue
		}

		soldAt, _ := strconv.ParseInt(soldAtStr, 10, 64)
		currency := parseCurrency(currStr)
		ebayURL := extract(reEbayURL, chunk)
		title := extract(reLinkText, chunk)

		entries = append(entries, saleEntry{
			company:  company,
			grade:    grade,
			price:    price,
			currency: currency,
			soldAt:   soldAt,
			ebayURL:  ebayURL,
			title:    title,
		})
	}
	return entries
}

// cheapestPerGrade agrupa por company+grade e devolve o menor preço de cada grupo.
func cheapestPerGrade(sales []saleEntry, cardName, fallbackURL string) []scraper.Result {
	type key struct{ company, grade string }
	best := make(map[key]saleEntry)

	for _, s := range sales {
		k := key{s.company, s.grade}
		if prev, ok := best[k]; !ok || s.price.LessThan(prev.price) {
			best[k] = s
		}
	}

	results := make([]scraper.Result, 0, len(best))
	for k, s := range best {
		rawCond := k.company + " " + k.grade
		itemURL := s.ebayURL
		if itemURL == "" {
			itemURL = fallbackURL
		}
		title := s.title
		if title == "" {
			title = buildTitle(cardName, rawCond)
		}

		results = append(results, scraper.Result{
			Title:        title,
			URL:          itemURL,
			Price:        s.price,
			Currency:     s.currency,
			Kind:         pricing.KindSale,
			Condition:    gradeToCondition(k.grade),
			RawCondition: rawCond,
		})
	}

	// Ordena por company asc, depois grade desc (melhor grau primeiro).
	sort.Slice(results, func(i, j int) bool {
		ci := results[i].RawCondition
		cj := results[j].RawCondition
		pi := companyPriority(results[i])
		pj := companyPriority(results[j])
		if pi != pj {
			return pi < pj
		}
		gi := gradeValue(strings.Fields(ci)[1])
		gj := gradeValue(strings.Fields(cj)[1])
		return gi > gj // grade maior primeiro
	})

	return results
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func extract(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

func parseCurrency(s string) pricing.Currency {
	switch strings.ToUpper(s) {
	case "EUR":
		return pricing.CurrencyEUR
	case "BRL":
		return pricing.CurrencyBRL
	case "JPY":
		return pricing.CurrencyJPY
	}
	return pricing.CurrencyUSD
}

// gradeToCondition mapeia o número de grade para a condição aproximada.
func gradeToCondition(grade string) string {
	g := gradeValue(grade)
	switch {
	case g >= 9:
		return string(pricing.ConditionNearMint)
	case g >= 7.5:
		return string(pricing.ConditionLightlyPlayed)
	case g >= 6:
		return string(pricing.ConditionModeratelyPlayed)
	case g >= 4:
		return string(pricing.ConditionHeavilyPlayed)
	case g > 0:
		return string(pricing.ConditionDamaged)
	}
	return ""
}

func gradeValue(grade string) float64 {
	g, _ := strconv.ParseFloat(grade, 64)
	return g
}

var companyOrder = map[string]int{"PSA": 0, "BGS": 1, "CGC": 2, "ACE": 3, "TAG": 4}

func companyPriority(r scraper.Result) int {
	company := strings.Fields(r.RawCondition)[0]
	if p, ok := companyOrder[company]; ok {
		return p
	}
	return 99
}

func buildTitle(cardName, rawCond string) string {
	if cardName == "" {
		return "eBay · " + rawCond
	}
	return cardName + " · " + rawCond
}
