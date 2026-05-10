package handler

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/pricing"
	"github.com/gustavojucoski/mercadotcg/backend/internal/pokemontcgio"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper"
)

// tcgPricesFromCatalog converte os preços da pokemontcg.io em resultados do scraper.
// Usado quando o product ID do TCGPlayer não pôde ser resolvido via redirect.
func tcgPricesFromCatalog(prices map[string]pokemontcgio.TCGPriceRange, cardURL string) []scraper.Result {
	var results []scraper.Result
	for printType, p := range prices {
		if p.Market == nil {
			continue
		}
		market := decimal.NewFromFloat(*p.Market)
		if market.IsZero() {
			continue
		}
		results = append(results, scraper.Result{
			Title:        fmt.Sprintf("TCGPlayer Market — %s", printType),
			URL:          cardURL,
			Price:        market,
			Currency:     pricing.CurrencyUSD,
			Kind:         pricing.KindListing,
			RawCondition: printType,
		})
	}
	return results
}

// cmConditionMultipliers mapeia condições do Cardmarket sobre o trendPrice (NM base).
// NM→NM, EX→LP, GD→MP, LP→HP, PO→DMG — nomenclatura CM europeia.
var cmConditionMultipliers = []struct {
	condition string  // nossa constante Condition
	rawCM     string  // label original do Cardmarket
	factor    float64 // multiplicador sobre o trendPrice (NM = 100%)
}{
	{string(pricing.ConditionNearMint), "NM", 1.00},
	{string(pricing.ConditionLightlyPlayed), "EX", 0.70},
	{string(pricing.ConditionModeratelyPlayed), "GD", 0.45},
	{string(pricing.ConditionHeavilyPlayed), "LP", 0.25},
	{string(pricing.ConditionDamaged), "PO", 0.10},
}

// cardmarketResultsFromCatalog deriva preços por condição a partir do trendPrice do
// pokemontcg.io, aplicando os multiplicadores padrão do Cardmarket.
// Usado como fallback quando o scraper ao vivo não retorna resultados.
func cardmarketResultsFromCatalog(p *pokemontcgio.CardmarketPriceRange, name, cardURL string) []scraper.Result {
	if p == nil {
		return nil
	}
	// Usa trendPrice como base NM; cai em averageSellPrice ou lowPrice se ausente.
	var nmPrice decimal.Decimal
	for _, v := range []*float64{p.TrendPrice, p.AverageSellPrice, p.LowPrice} {
		if v != nil && *v > 0 {
			nmPrice = decimal.NewFromFloat(*v)
			break
		}
	}
	if nmPrice.IsZero() {
		return nil
	}

	results := make([]scraper.Result, 0, len(cmConditionMultipliers))
	for _, cond := range cmConditionMultipliers {
		price := nmPrice.Mul(decimal.NewFromFloat(cond.factor)).Round(2)
		title := "Cardmarket"
		if name != "" {
			title = name + " · " + cond.rawCM
		}
		results = append(results, scraper.Result{
			Title:        title,
			URL:          cardURL,
			Price:        price,
			Currency:     pricing.CurrencyEUR,
			Kind:         pricing.KindListing,
			Condition:    cond.condition,
			RawCondition: cond.rawCM,
		})
	}
	return results
}

// cardResolver resolve nome e IDs externos de uma carta a partir do ptcgoCode + número.
type cardResolver interface {
	FindCard(ctx context.Context, ptcgoCode, number string) (pokemontcgio.CardInfo, error)
}

// ExternalHandler expõe a busca ao vivo agregando múltiplas fontes.
type ExternalHandler struct {
	sources          []scraper.Source
	perSourceTimeout time.Duration
	sourceTimeouts   map[pricing.Source]time.Duration // override por fonte
	catalog          cardResolver // opcional — quando presente, resolve name e TCGPlayer ID
}

// NewExternalHandler monta o handler com as fontes registradas.
func NewExternalHandler(sources ...scraper.Source) *ExternalHandler {
	return &ExternalHandler{
		sources:          sources,
		perSourceTimeout: 12 * time.Second,
	}
}

// WithCatalog injeta o client da pokemontcg.io para resolução transparente de
// nome da carta e product IDs por fonte, a partir de number+set apenas.
func (h *ExternalHandler) WithCatalog(c cardResolver) *ExternalHandler {
	h.catalog = c
	return h
}

// WithSourceTimeout define timeout específico para uma fonte.
// Útil quando uma fonte (ex: Cardmarket via FlareSolverr) precisa de mais tempo.
func (h *ExternalHandler) WithSourceTimeout(src pricing.Source, d time.Duration) *ExternalHandler {
	if h.sourceTimeouts == nil {
		h.sourceTimeouts = make(map[pricing.Source]time.Duration)
	}
	h.sourceTimeouts[src] = d
	return h
}

// Routes monta a rota.
func (h *ExternalHandler) Routes(r chi.Router) {
	r.Get("/external-search", h.search)
}

// externalSearchResponse é o JSON devolvido por /external-search.
type externalSearchResponse struct {
	Card      *resolvedCard          `json:"card,omitempty"`
	Query     scraper.Query          `json:"query"`
	FetchedAt time.Time              `json:"fetched_at"`
	Sources   []scraper.SourceResult `json:"sources"`
}

// resolvedCard é o resumo da carta encontrada via pokemontcg.io.
type resolvedCard struct {
	ID      string `json:"id"`       // pokemontcg.io card ID, ex.: "me2pt5-276"
	Name    string `json:"name"`
	Number  string `json:"number"`
	SetCode string `json:"set_code"`
	SetName string `json:"set_name"`
}

// search executa fan-out paralelo com timeout independente por fonte.
//
// Parâmetros aceitos:
//
//	number  — número da carta no set (ex: "276") — obrigatório
//	set     — ptcgoCode do set (ex: "ASC") — obrigatório
//	limit   — máximo de resultados por fonte (default 10)
//
// A carta é resolvida automaticamente via pokemontcg.io (nome, product IDs).
func (h *ExternalHandler) search(w http.ResponseWriter, r *http.Request) {
	number := r.URL.Query().Get("number")
	setCode := r.URL.Query().Get("set")
	limit := atoiOrDefault(r.URL.Query().Get("limit"), 10)

	if number == "" || setCode == "" {
		writeBadRequest(w, "informe number e set")
		return
	}

	baseQuery := scraper.Query{
		Number:  number,
		SetCode: setCode,
		Limit:   limit,
	}

	sourceExternalIDs := make(map[pricing.Source]string)
	var resolvedInfo *resolvedCard
	var catalogInfo *pokemontcgio.CardInfo

	// Resolução via pokemontcg.io: nome da carta + product IDs externos.
	if h.catalog != nil {
		catalogCtx, catalogCancel := context.WithTimeout(r.Context(), 12*time.Second)
		defer catalogCancel()
		if info, err := h.catalog.FindCard(catalogCtx, setCode, number); err == nil {
			baseQuery.Name = info.Name

			resolvedInfo = &resolvedCard{
				ID:      info.ID,
				Name:    info.Name,
				Number:  info.Number,
				SetCode: info.SetCode,
				SetName: info.SetName,
			}

			if info.TCGPlayerID != "" {
				sourceExternalIDs[pricing.SourceTCGPlayer] = info.TCGPlayerID
			}
			if info.ID != "" {
				sourceExternalIDs[pricing.SourceEbay] = info.ID
			}
			if info.CardmarketURL != "" {
				sourceExternalIDs[pricing.SourceCardmarket] = info.CardmarketURL
			}
			catalogInfo = &info
		}
	}

	// Fan-out paralelo com timeout por fonte.
	results := make([]scraper.SourceResult, len(h.sources))
	var wg sync.WaitGroup

	for i, src := range h.sources {
		wg.Add(1)
		go func(idx int, s scraper.Source) {
			defer wg.Done()

			timeout := h.perSourceTimeout
			if t, ok := h.sourceTimeouts[s.Name()]; ok {
				timeout = t
			}
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			q := baseQuery
			if eid, ok := sourceExternalIDs[s.Name()]; ok {
				q.ExternalID = eid
			}
			results[idx] = scraper.MeasureSearch(ctx, s, q)
		}(i, src)
	}
	wg.Wait()

	// Fallback: se o scraper do TCGPlayer falhou por falta de product ID, mas temos
	// os preços direto da pokemontcg.io, usa esses dados em vez de retornar erro.
	if catalogInfo != nil && len(catalogInfo.TCGPlayerPrices) > 0 {
		for i, r := range results {
			if r.Source == pricing.SourceTCGPlayer && len(r.Results) == 0 {
				synthetic := tcgPricesFromCatalog(catalogInfo.TCGPlayerPrices, catalogInfo.TCGPlayerURL)
				if len(synthetic) > 0 {
					results[i] = scraper.SourceResult{
						Source:     pricing.SourceTCGPlayer,
						DurationMS: r.DurationMS,
						Results:    synthetic,
					}
				}
			}
		}
	}

	// Cardmarket: se o scraper retornou vazio mas temos preços do catálogo,
	// injeta preços por condição estimados via multiplicadores sobre o trendPrice.
	if catalogInfo != nil && catalogInfo.CardmarketPrices != nil {
		for i, r := range results {
			if r.Source == pricing.SourceCardmarket && len(r.Results) == 0 {
				synthetic := cardmarketResultsFromCatalog(
					catalogInfo.CardmarketPrices,
					catalogInfo.Name,
					catalogInfo.CardmarketURL,
				)
				if len(synthetic) > 0 {
					results[i] = scraper.SourceResult{
						Source:     pricing.SourceCardmarket,
						DurationMS: r.DurationMS,
						Results:    synthetic,
					}
				}
			}
		}
	}

	sort.SliceStable(results, func(i, j int) bool {
		return string(results[i].Source) < string(results[j].Source)
	})

	writeJSON(w, http.StatusOK, externalSearchResponse{
		Card:      resolvedInfo,
		Query:     baseQuery,
		FetchedAt: time.Now().UTC(),
		Sources:   results,
	})
}
