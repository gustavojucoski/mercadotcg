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

// cardResolver resolve nome e IDs externos de uma carta a partir do ptcgoCode + número.
type cardResolver interface {
	FindCard(ctx context.Context, ptcgoCode, number string) (pokemontcgio.CardInfo, error)
}

// ExternalHandler expõe a busca ao vivo agregando múltiplas fontes.
type ExternalHandler struct {
	sources          []scraper.Source
	perSourceTimeout time.Duration
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
//	number  — número da carta no set (ex: "276")
//	set     — ptcgoCode do set (ex: "ASC")
//	name    — nome da carta; ignorado quando catalog está disponível
//	limit   — máximo de resultados por fonte (default 10)
//
// Quando catalog está injetado e number+set são fornecidos:
//   - nome da carta é resolvido automaticamente via pokemontcg.io
//   - TCGPlayer product ID é resolvido via redirect de prices.pokemontcg.io
func (h *ExternalHandler) search(w http.ResponseWriter, r *http.Request) {
	number := r.URL.Query().Get("number")
	setCode := r.URL.Query().Get("set")
	nameParam := r.URL.Query().Get("name")
	limit := atoiOrDefault(r.URL.Query().Get("limit"), 10)

	if number == "" && setCode == "" && nameParam == "" {
		writeBadRequest(w, "informe ao menos number+set ou name")
		return
	}

	baseQuery := scraper.Query{
		Name:    nameParam,
		Number:  number,
		SetCode: setCode,
		Limit:   limit,
	}

	sourceExternalIDs := make(map[pricing.Source]string)
	var resolvedInfo *resolvedCard
	var catalogInfo *pokemontcgio.CardInfo

	// Resolução via pokemontcg.io — ativa quando catalog está injetado e temos number+set.
	// Timeout curto para não atrasar o fan-out caso a API demore ou seja rate-limitada.
	if h.catalog != nil && number != "" && setCode != "" {
		catalogCtx, catalogCancel := context.WithTimeout(r.Context(), 5*time.Second)
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
			ctx, cancel := context.WithTimeout(r.Context(), h.perSourceTimeout)
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
