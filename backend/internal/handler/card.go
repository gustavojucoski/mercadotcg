package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gustavojucoski/mercadotcg/backend/internal/auth"
	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/card"
	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/pricing"
	"github.com/gustavojucoski/mercadotcg/backend/internal/repository/postgres"
	"github.com/gustavojucoski/mercadotcg/backend/internal/service/pricesignal"
)

// CardHandler agrupa endpoints de cartas/variantes.
type CardHandler struct {
	cards      *postgres.CardRepo
	priceDaily *postgres.PriceDailyRepo
	signal     *pricesignal.Service
	mw         *auth.Middleware
}

// NewCardHandler cria o handler.
func NewCardHandler(cards *postgres.CardRepo, priceDaily *postgres.PriceDailyRepo, signal *pricesignal.Service, mw *auth.Middleware) *CardHandler {
	return &CardHandler{cards: cards, priceDaily: priceDaily, signal: signal, mw: mw}
}

// Routes monta as rotas no router.
func (h *CardHandler) Routes(r chi.Router) {
	// Rotas existentes.
	r.Get("/cards/search", h.search)
	r.Get("/cards/lookup", h.lookup)
	r.Get("/cards/{id}", h.getByID)
	r.Get("/cards/{id}/variants", h.listVariants)

	// Rotas públicas de catálogo — sem autenticação.
	r.Get("/series", h.listSeriesPublic)
	r.Get("/sets/{tcg}", h.listSetsByTCG)
	r.Get("/sets/{tcg}/{code}", h.getSetByCode)
	r.Get("/sets/{tcg}/{code}/cards", h.listCardsBySet)
	// /cards/autocomplete deve ser registrado ANTES de /cards/{slug} para o chi
	// não capturar "autocomplete" como valor do parâmetro {slug}.
	r.Get("/cards/autocomplete", h.autocomplete)
	r.Get("/cards/{slug}", h.getCardBySlug)

	r.Route("/admin/series", func(r chi.Router) {
		r.Use(h.mw.RequirePlatformAdmin)
		r.Get("/", h.listSeries)
		r.Patch("/{id}/name-pt", h.updateSeriesNamePT)
	})
	r.Route("/admin/sets", func(r chi.Router) {
		r.Use(h.mw.RequirePlatformAdmin)
		r.Patch("/{id}/name-pt", h.updateSetNamePT)
	})
	r.Route("/admin/cards", func(r chi.Router) {
		r.Use(h.mw.RequirePlatformAdmin)
		r.Patch("/{id}/name-pt", h.updateCardNamePT)
	})
}

// ----------------------------------------------------------------------------
// GET /cards/search?q=charizard&limit=20
// ----------------------------------------------------------------------------

func (h *CardHandler) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeBadRequest(w, "parâmetro q é obrigatório")
		return
	}
	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	cards, err := h.cards.SearchCardsByName(r.Context(), q, limit)
	if err != nil {
		writeErr(w, err)
		return
	}
	if cards == nil {
		cards = []card.Card{}
	}
	writeJSON(w, http.StatusOK, cards)
}

// ----------------------------------------------------------------------------
// GET /cards/lookup
//
// Busca rica que aceita filtros opcionais combinados:
//
//	?name=charizard          → busca tolerante a typo (pg_trgm)
//	?number=199/191          → match exato no número ou collector_number da carta
//	?set=sv8                 → match exato no code do set
//	?with_signal=true        → enriquece cada variante com matriz condition×source
//	?window=30               → janela do signal em dias (default 30)
//	?limit=20                → máximo de cartas retornadas (default 20, max 100)
//
// Pelo menos um de name/number/set é obrigatório (evita scan da tabela inteira).
// ----------------------------------------------------------------------------

// lookupVariantBlock é uma variante enriquecida (opcionalmente) com sinais
// agrupados por condição.
type lookupVariantBlock struct {
	Variant            card.Variant                   `json:"variant"`
	SignalsByCondition *pricesignal.SignalsByCondition `json:"signals_by_condition,omitempty"`
}

type lookupResultItem struct {
	Card     card.Card            `json:"card"`
	Set      card.Set             `json:"set"`
	Variants []lookupVariantBlock `json:"variants"`
}

type lookupResponse struct {
	Query   lookupEcho         `json:"query"`
	Matches []lookupResultItem `json:"matches"`
}

type lookupEcho struct {
	Name       string `json:"name,omitempty"`
	Number     string `json:"number,omitempty"`
	SetCode    string `json:"set,omitempty"`
	WithSignal bool   `json:"with_signal"`
	WindowDays int    `json:"window_days,omitempty"`
}

func (h *CardHandler) lookup(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	name := q.Get("name")
	number := q.Get("number")
	setCode := q.Get("set")

	if name == "" && number == "" && setCode == "" {
		writeBadRequest(w, "informe ao menos um de: name, number ou set")
		return
	}

	withSignal := q.Get("with_signal") == "true"
	window := atoiOrDefault(q.Get("window"), 30)
	limit := atoiOrDefault(q.Get("limit"), 20)

	cards, err := h.cards.LookupCards(r.Context(), name, number, setCode, limit)
	if err != nil {
		writeErr(w, err)
		return
	}

	resp := lookupResponse{
		Query: lookupEcho{
			Name: name, Number: number, SetCode: setCode,
			WithSignal: withSignal, WindowDays: window,
		},
		Matches: make([]lookupResultItem, 0, len(cards)),
	}

	for _, cw := range cards {
		variants, err := h.cards.ListVariantsByCard(r.Context(), cw.Card.ID)
		if err != nil {
			writeErr(w, err)
			return
		}

		blocks := make([]lookupVariantBlock, 0, len(variants))
		for _, v := range variants {
			b := lookupVariantBlock{Variant: v}
			if withSignal {
				sig, err := h.signal.ByConditions(r.Context(), v.ID, window)
				if err == nil {
					b.SignalsByCondition = &sig
				}
			}
			blocks = append(blocks, b)
		}

		resp.Matches = append(resp.Matches, lookupResultItem{
			Card:     cw.Card,
			Set:      cw.Set,
			Variants: blocks,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

// ----------------------------------------------------------------------------
// GET /cards/{id}
// ----------------------------------------------------------------------------

func (h *CardHandler) getByID(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "id inválido")
		return
	}
	c, err := h.cards.GetCardByID(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// ----------------------------------------------------------------------------
// GET /cards/{id}/variants
// ----------------------------------------------------------------------------

func (h *CardHandler) listVariants(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "id inválido")
		return
	}
	variants, err := h.cards.ListVariantsByCard(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if variants == nil {
		variants = []card.Variant{}
	}
	writeJSON(w, http.StatusOK, variants)
}

// ----------------------------------------------------------------------------
// Admin — séries, sets e cartas (PT-BR)
// ----------------------------------------------------------------------------

func (h *CardHandler) listSeries(w http.ResponseWriter, r *http.Request) {
	tcg := r.URL.Query().Get("tcg")
	series, err := h.cards.ListSeries(r.Context(), tcg)
	if err != nil {
		writeErr(w, err)
		return
	}
	if series == nil {
		series = []card.Series{}
	}
	writeJSON(w, http.StatusOK, series)
}

type updateNamePTReq struct {
	NamePT string `json:"name_pt"`
}

func (h *CardHandler) updateSeriesNamePT(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "id inválido")
		return
	}
	var req updateNamePTReq
	if err := decodeJSON(r, &req); err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	if err := h.cards.UpdateSeriesNamePT(r.Context(), id, req.NamePT); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "name_pt atualizado"})
}

func (h *CardHandler) updateSetNamePT(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "id inválido")
		return
	}
	var req updateNamePTReq
	if err := decodeJSON(r, &req); err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	if err := h.cards.UpdateSetNamePT(r.Context(), id, req.NamePT); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "name_pt atualizado"})
}

func (h *CardHandler) updateCardNamePT(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "id inválido")
		return
	}
	var req updateNamePTReq
	if err := decodeJSON(r, &req); err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	if err := h.cards.UpdateCardNamePT(r.Context(), id, req.NamePT); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "name_pt atualizado"})
}

// ----------------------------------------------------------------------------
// Public catalog endpoints
// ----------------------------------------------------------------------------

// GET /series?tcg=pokemon
func (h *CardHandler) listSeriesPublic(w http.ResponseWriter, r *http.Request) {
	tcg := r.URL.Query().Get("tcg")
	series, err := h.cards.ListSeries(r.Context(), tcg)
	if err != nil {
		writeErr(w, err)
		return
	}
	if series == nil {
		series = []card.Series{}
	}
	w.Header().Set("Cache-Control", "public, max-age=3600")
	writeJSON(w, http.StatusOK, series)
}

// GET /sets/{tcg}?series_id=&page=1&limit=30
func (h *CardHandler) listSetsByTCG(w http.ResponseWriter, r *http.Request) {
	tcg := chi.URLParam(r, "tcg")
	q := r.URL.Query()

	var seriesID *uuid.UUID
	if raw := q.Get("series_id"); raw != "" {
		id, err := parseUUID(raw)
		if err != nil {
			writeBadRequest(w, "series_id inválido")
			return
		}
		seriesID = &id
	}

	page := atoiOrDefault(q.Get("page"), 1)
	limit := atoiOrDefault(q.Get("limit"), 30)
	if limit > 100 {
		limit = 100
	}

	sets, total, err := h.cards.ListSetsByTCG(r.Context(), tcg, seriesID, page, limit)
	if err != nil {
		writeErr(w, err)
		return
	}
	if sets == nil {
		sets = []postgres.SetWithSeries{}
	}

	w.Header().Set("Cache-Control", "public, max-age=3600")
	writeJSON(w, http.StatusOK, map[string]any{
		"tcg":   tcg,
		"total": total,
		"page":  page,
		"limit": limit,
		"sets":  sets,
	})
}

// GET /sets/{tcg}/{code}
func (h *CardHandler) getSetByCode(w http.ResponseWriter, r *http.Request) {
	tcg := chi.URLParam(r, "tcg")
	code := chi.URLParam(r, "code")

	s, err := h.cards.GetSetByCode(r.Context(), code)
	if err != nil {
		writeErr(w, err)
		return
	}
	// Garante que o set pertence ao TCG requisitado.
	if s.TCG != tcg {
		writeJSON(w, http.StatusNotFound, errorBody{Error: "registro não encontrado"})
		return
	}

	w.Header().Set("Cache-Control", "public, max-age=3600")
	writeJSON(w, http.StatusOK, s)
}

// GET /sets/{tcg}/{code}/cards?page=1&limit=60
func (h *CardHandler) listCardsBySet(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	q := r.URL.Query()

	page := atoiOrDefault(q.Get("page"), 1)
	limit := atoiOrDefault(q.Get("limit"), 60)
	if limit > 200 {
		limit = 200
	}

	cards, total, err := h.cards.ListCardsBySetCode(r.Context(), code, page, limit)
	if err != nil {
		writeErr(w, err)
		return
	}
	if cards == nil {
		cards = []postgres.CardWithVariants{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"set_code": code,
		"total":    total,
		"page":     page,
		"limit":    limit,
		"cards":    cards,
	})
}

// GET /cards/autocomplete?q=chara&tcg=pokemon&limit=8
func (h *CardHandler) autocomplete(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if len(q) < 2 {
		writeBadRequest(w, "q deve ter ao menos 2 caracteres")
		return
	}
	tcg := r.URL.Query().Get("tcg")
	results, err := h.cards.AutocompleteCards(r.Context(), q, tcg, 8)
	if err != nil {
		writeErr(w, err)
		return
	}
	if results == nil {
		results = []postgres.AutocompleteResult{}
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	writeJSON(w, http.StatusOK, results)
}

// variantWithPrice é a variante enriquecida com o resumo de preço mais recente.
type variantWithPrice struct {
	ID           uuid.UUID             `json:"id"`
	Finish       string                `json:"finish"`
	Label        string                `json:"label,omitempty"`
	IsPromo      bool                  `json:"is_promo"`
	PriceSummary *postgres.PriceSummary `json:"price_summary"`
}

// GET /cards/{slug}  — slug format: "{setCode}-{collectorNumber}"
// Collector numbers can themselves contain hyphens (e.g. "TG01-EN"), so we
// split only on the FIRST hyphen.
func (h *CardHandler) getCardBySlug(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	idx := strings.Index(slug, "-")
	if idx < 0 {
		writeBadRequest(w, "slug inválido — formato esperado: {set}-{número}")
		return
	}
	setCode := slug[:idx]
	collectorNumber := slug[idx+1:]

	c, s, err := h.cards.GetCardBySetAndNumber(r.Context(), setCode, collectorNumber)
	if err != nil {
		writeErr(w, err)
		return
	}

	variants, err := h.cards.ListVariantsByCard(r.Context(), c.ID)
	if err != nil {
		writeErr(w, err)
		return
	}

	blocks := make([]variantWithPrice, 0, len(variants))
	for _, v := range variants {
		summary, err := h.priceDaily.GetLatestByVariant(r.Context(), v.ID, string(pricing.ConditionNearMint))
		if err != nil {
			writeErr(w, err)
			return
		}
		blocks = append(blocks, variantWithPrice{
			ID:           v.ID,
			Finish:       string(v.Finish),
			Label:        v.Label,
			IsPromo:      v.IsPromo,
			PriceSummary: summary, // nil quando sem dados — serializa como null no JSON
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"card":     c,
		"set":      s,
		"variants": blocks,
	})
}
