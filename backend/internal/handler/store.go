package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/pricing"
	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/store"
	"github.com/gustavojucoski/mercadotcg/backend/internal/repository/postgres"
	"github.com/gustavojucoski/mercadotcg/backend/internal/service/pricesignal"
)

// StoreHandler agrupa endpoints relativos a lojas e estoque.
type StoreHandler struct {
	stores *postgres.StoreRepo
	stock  *postgres.StockRepo
	signal *pricesignal.Service
}

// NewStoreHandler cria o handler com as dependências.
func NewStoreHandler(
	stores *postgres.StoreRepo,
	stock *postgres.StockRepo,
	signal *pricesignal.Service,
) *StoreHandler {
	return &StoreHandler{stores: stores, stock: stock, signal: signal}
}

// Routes monta as rotas no router chi recebido (leitura pública).
// POST /stores foi migrado para AdminHandler.
// As rotas de escrita (purchase/sale) são registradas em main.go com middleware de auth.
func (h *StoreHandler) Routes(r chi.Router) {
	r.Get("/stores/{id}", h.GetByID)
	r.Get("/stores/{id}/stock", h.ListStock)
}

// ----------------------------------------------------------------------------
// GET /stores/{id}
// ----------------------------------------------------------------------------

func (h *StoreHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "id inválido")
		return
	}
	s, err := h.stores.GetByID(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

// ----------------------------------------------------------------------------
// GET /stores/{id}/stock
// Query params:
//   - in_stock=true        → só itens com quantity > 0
//   - with_signal=true     → enriquece cada item com price signal das fontes
//   - limit, offset        → paginação
// ----------------------------------------------------------------------------

type stockListItem struct {
	store.StockItem
	Signal *pricesignal.Signal `json:"signal,omitempty"`
}

func (h *StoreHandler) ListStock(w http.ResponseWriter, r *http.Request) {
	storeID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "id inválido")
		return
	}

	q := r.URL.Query()
	onlyInStock := q.Get("in_stock") == "true"
	withSignal := q.Get("with_signal") == "true"
	limit := atoiOrDefault(q.Get("limit"), 100)
	offset := atoiOrDefault(q.Get("offset"), 0)

	items, err := h.stock.ListItemsByStore(r.Context(), storeID, onlyInStock, limit, offset)
	if err != nil {
		writeErr(w, err)
		return
	}

	out := make([]stockListItem, 0, len(items))
	for _, it := range items {
		row := stockListItem{StockItem: it}
		if withSignal {
			sig, err := h.signal.For(r.Context(), it.VariantID, pricing.Condition(it.Condition))
			if err == nil {
				row.Signal = &sig
			}
		}
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, out)
}

// ----------------------------------------------------------------------------
// POST /stores/{id}/stock/purchase
// ----------------------------------------------------------------------------

type purchaseReq struct {
	VariantID   string `json:"variant_id"`
	Condition   string `json:"condition"`
	Language    string `json:"language"`
	Grade       string `json:"grade,omitempty"`
	Quantity    int    `json:"quantity"`
	UnitCostBRL string `json:"unit_cost_brl"`
	OccurredAt  string `json:"occurred_at,omitempty"`
	Notes       string `json:"notes,omitempty"`
}

func (h *StoreHandler) RegisterPurchase(w http.ResponseWriter, r *http.Request) {
	storeID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "id da loja inválido")
		return
	}

	var req purchaseReq
	if err := decodeJSON(r, &req); err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	if req.VariantID == "" || req.Condition == "" || req.Language == "" || req.Quantity <= 0 {
		writeBadRequest(w, "variant_id, condition, language e quantity (>0) são obrigatórios")
		return
	}
	variantID, err := parseUUID(req.VariantID)
	if err != nil {
		writeBadRequest(w, "variant_id inválido")
		return
	}
	cost, err := decimal.NewFromString(req.UnitCostBRL)
	if err != nil {
		writeBadRequest(w, "unit_cost_brl inválido")
		return
	}
	occurred, err := parseTimeOrZero(req.OccurredAt)
	if err != nil {
		writeBadRequest(w, "occurred_at deve estar em RFC3339")
		return
	}

	item, err := h.stock.RegisterPurchase(r.Context(), postgres.PurchaseInput{
		StoreID:      storeID,
		VariantID:    variantID,
		Condition:    req.Condition,
		Language:     req.Language,
		Grade:        req.Grade,
		Quantity:     req.Quantity,
		UnitCostBRL:  cost,
		OccurredAt:   occurred,
		ReferenceTyp: "manual",
		Notes:        req.Notes,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

// ----------------------------------------------------------------------------
// POST /stock-items/{id}/sale
// ----------------------------------------------------------------------------

type saleReq struct {
	Quantity     int    `json:"quantity"`
	UnitPriceBRL string `json:"unit_price_brl"`
	OccurredAt   string `json:"occurred_at,omitempty"`
	Notes        string `json:"notes,omitempty"`
}

func (h *StoreHandler) RegisterSale(w http.ResponseWriter, r *http.Request) {
	itemID, err := parseUUID(chi.URLParam(r, "itemID"))
	if err != nil {
		writeBadRequest(w, "id do stock_item inválido")
		return
	}

	var req saleReq
	if err := decodeJSON(r, &req); err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	if req.Quantity <= 0 {
		writeBadRequest(w, "quantity deve ser > 0")
		return
	}
	price, err := decimal.NewFromString(req.UnitPriceBRL)
	if err != nil {
		writeBadRequest(w, "unit_price_brl inválido")
		return
	}
	occurred, err := parseTimeOrZero(req.OccurredAt)
	if err != nil {
		writeBadRequest(w, "occurred_at deve estar em RFC3339")
		return
	}

	item, err := h.stock.RegisterSale(r.Context(), postgres.SaleInput{
		StockItemID:  itemID,
		Quantity:     req.Quantity,
		UnitPriceBRL: price,
		OccurredAt:   occurred,
		ReferenceTyp: "manual",
		Notes:        req.Notes,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

// ----------------------------------------------------------------------------
// helpers locais
// ----------------------------------------------------------------------------

func atoiOrDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return def
	}
	return n
}

func parseTimeOrZero(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, s)
}
