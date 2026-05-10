package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/pricing"
	"github.com/gustavojucoski/mercadotcg/backend/internal/service/pricesignal"
)

// VariantHandler expõe endpoints relacionados a variantes (sinais de preço).
type VariantHandler struct {
	signal *pricesignal.Service
}

// NewVariantHandler cria o handler.
func NewVariantHandler(signal *pricesignal.Service) *VariantHandler {
	return &VariantHandler{signal: signal}
}

// Routes monta as rotas no router.
func (h *VariantHandler) Routes(r chi.Router) {
	r.Get("/variants/{id}/signal", h.signalByVariant)
}

// ----------------------------------------------------------------------------
// GET /variants/{id}/signal?condition=NM&window=30
// ----------------------------------------------------------------------------

func (h *VariantHandler) signalByVariant(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "id inválido")
		return
	}

	cond := r.URL.Query().Get("condition")
	if cond == "" {
		cond = string(pricing.ConditionNearMint)
	}
	window := 30
	if v := r.URL.Query().Get("window"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			window = n
		}
	}

	sig, err := h.signal.ForWindow(r.Context(), id, pricing.Condition(cond), window)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sig)
}
