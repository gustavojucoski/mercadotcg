package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/gustavojucoski/mercadotcg/backend/internal/auth"
	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/card"
	"github.com/gustavojucoski/mercadotcg/backend/internal/repository/postgres"
	"github.com/gustavojucoski/mercadotcg/backend/internal/service/pricesignal"
)

// CardHandler agrupa endpoints de cartas/variantes.
type CardHandler struct {
	cards  *postgres.CardRepo
	signal *pricesignal.Service
	mw     *auth.Middleware
}

// NewCardHandler cria o handler.
func NewCardHandler(cards *postgres.CardRepo, signal *pricesignal.Service, mw *auth.Middleware) *CardHandler {
	return &CardHandler{cards: cards, signal: signal, mw: mw}
}

// Routes monta as rotas no router.
func (h *CardHandler) Routes(r chi.Router) {
	r.Get("/cards/search", h.search)
	r.Get("/cards/lookup", h.lookup)
	r.Get("/cards/{id}", h.getByID)
	r.Get("/cards/{id}/variants", h.listVariants)

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
