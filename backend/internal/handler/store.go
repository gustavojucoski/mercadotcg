package handler

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/gustavojucoski/mercadotcg/backend/internal/auth"
	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/pricing"
	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/store"
	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/user"
	"github.com/gustavojucoski/mercadotcg/backend/internal/repository/postgres"
	"github.com/gustavojucoski/mercadotcg/backend/internal/service/pricesignal"
	"github.com/gustavojucoski/mercadotcg/backend/internal/upload"
)

// StoreHandler agrupa endpoints relativos a lojas e estoque.
type StoreHandler struct {
	stores  *postgres.StoreRepo
	stock   *postgres.StockRepo
	cards   *postgres.CardRepo
	signal  *pricesignal.Service
	members *postgres.StoreMemberRepo
	audit   *postgres.StoreAuditRepo
	uploads upload.Provider
	users   *postgres.UserRepo
}

// NewStoreHandler cria o handler com as dependências.
func NewStoreHandler(
	stores *postgres.StoreRepo,
	stock *postgres.StockRepo,
	cards *postgres.CardRepo,
	signal *pricesignal.Service,
	members *postgres.StoreMemberRepo,
	audit *postgres.StoreAuditRepo,
	uploads upload.Provider,
	users *postgres.UserRepo,
) *StoreHandler {
	return &StoreHandler{
		stores: stores, stock: stock, cards: cards, signal: signal,
		members: members, audit: audit, uploads: uploads,
		users: users,
	}
}

// Routes monta as rotas no router chi recebido (leitura pública).
// As rotas de escrita (purchase/sale/profile) são registradas em main.go com middleware de auth.
func (h *StoreHandler) Routes(r chi.Router) {
	// literal route must come before {id} wildcard
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
// GET /stores/me  — requer RequireAuth (registrado em main.go)
// ----------------------------------------------------------------------------

func (h *StoreHandler) ListMyStores(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromCtx(r.Context())
	stores, err := h.stores.ListByMember(r.Context(), u.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if stores == nil {
		stores = []store.Store{}
	}
	writeJSON(w, http.StatusOK, stores)
}

// ----------------------------------------------------------------------------
// GET /stores/{id}/stock
// ----------------------------------------------------------------------------

type stockListItem struct {
	store.StockItem
	CardName      string              `json:"card_name"`
	CardNumber    string              `json:"card_number"`
	SetName       string              `json:"set_name"`
	SetCode       string              `json:"set_code"`
	Finish        string              `json:"finish"`
	VariantLabel  string              `json:"variant_label,omitempty"`
	ImageSmallURL string              `json:"image_small_url,omitempty"`
	Signal        *pricesignal.Signal `json:"signal,omitempty"`
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

	// Batch-fetch variant display data to avoid N+1 queries.
	variantIDs := make([]uuid.UUID, 0, len(items))
	for _, it := range items {
		variantIDs = append(variantIDs, it.VariantID)
	}
	displays, err := h.cards.GetVariantDisplayBatch(r.Context(), variantIDs)
	if err != nil {
		writeErr(w, err)
		return
	}

	out := make([]stockListItem, 0, len(items))
	for _, it := range items {
		d := displays[it.VariantID]
		row := stockListItem{
			StockItem:     it,
			CardName:      d.CardName,
			CardNumber:    d.CardNumber,
			SetName:       d.SetName,
			SetCode:       d.SetCode,
			Finish:        d.Finish,
			VariantLabel:  d.Label,
			ImageSmallURL: d.ImageSmallURL,
		}
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
// PATCH /stores/{id}/profile — restricted self-edit (store admin role required)
// ----------------------------------------------------------------------------

type updateProfileReq struct {
	Name                string `json:"name,omitempty"`
	Description         string `json:"description,omitempty"`
	TradeName           string `json:"trade_name,omitempty"`
	Phone               string `json:"phone,omitempty"`
	AddressZip          string `json:"address_zip,omitempty"`
	AddressStreet       string `json:"address_street,omitempty"`
	AddressNumber       string `json:"address_number,omitempty"`
	AddressComplement   string `json:"address_complement,omitempty"`
	AddressNeighborhood string `json:"address_neighborhood,omitempty"`
	AddressCity         string `json:"address_city,omitempty"`
	AddressState        string `json:"address_state,omitempty"`
}

func (h *StoreHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "id inválido")
		return
	}
	old, err := h.stores.GetByID(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}

	var req updateProfileReq
	if err := decodeJSON(r, &req); err != nil {
		writeBadRequest(w, err.Error())
		return
	}

	s := old
	if req.Name != "" {
		s.Name = req.Name
	}
	if req.Description != "" {
		s.Description = req.Description
	}
	if req.TradeName != "" {
		s.TradeName = req.TradeName
	}
	if req.Phone != "" {
		s.Phone = req.Phone
	}
	if req.AddressZip != "" {
		s.AddressZip = req.AddressZip
	}
	if req.AddressStreet != "" {
		s.AddressStreet = req.AddressStreet
	}
	if req.AddressNumber != "" {
		s.AddressNumber = req.AddressNumber
	}
	if req.AddressComplement != "" {
		s.AddressComplement = req.AddressComplement
	}
	if req.AddressNeighborhood != "" {
		s.AddressNeighborhood = req.AddressNeighborhood
	}
	if req.AddressCity != "" {
		s.AddressCity = req.AddressCity
	}
	if req.AddressState != "" {
		s.AddressState = req.AddressState
	}

	if err := h.stores.Update(r.Context(), &s); err != nil {
		writeErr(w, err)
		return
	}

	u, _ := auth.UserFromCtx(r.Context())
	_ = h.audit.Insert(r.Context(), s.ID, u.ID, "store_update",
		store.BuildDiff(old, s))

	writeJSON(w, http.StatusOK, s)
}

// ----------------------------------------------------------------------------
// GET /stores/{id}/my-role — returns authenticated user's role in the store
// Returns 403 if not a member; platform_admin always gets "admin".
// ----------------------------------------------------------------------------

func (h *StoreHandler) GetMyRole(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "id inválido")
		return
	}
	u, _ := auth.UserFromCtx(r.Context())
	if u.PlatformRole == "platform_admin" {
		writeJSON(w, http.StatusOK, map[string]string{"role": "admin"})
		return
	}
	role, err := h.members.GetMembership(r.Context(), id, u.ID)
	if err != nil {
		writeJSON(w, http.StatusForbidden, errorBody{Error: "não é membro desta loja"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"role": string(role)})
}

// ----------------------------------------------------------------------------
// GET /stores/{id}/members — list members (RequireStoreRole viewer+)
// ----------------------------------------------------------------------------

func (h *StoreHandler) StoreListMembers(w http.ResponseWriter, r *http.Request) {
	storeID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "id inválido")
		return
	}
	members, err := h.members.ListMembers(r.Context(), storeID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if members == nil {
		members = []postgres.StoreMemberRow{}
	}
	writeJSON(w, http.StatusOK, members)
}

// ----------------------------------------------------------------------------
// POST /stores/{id}/members — invite member by email (RequireStoreRole admin)
// ----------------------------------------------------------------------------

type storeAddMemberReq struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

func (h *StoreHandler) StoreAddMember(w http.ResponseWriter, r *http.Request) {
	storeID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "id inválido")
		return
	}
	var req storeAddMemberReq
	if err := decodeJSON(r, &req); err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	if req.Email == "" {
		writeBadRequest(w, "email é obrigatório")
		return
	}
	role := user.StoreRole(req.Role)
	if user.StoreRoleLevel(role) == 0 {
		writeBadRequest(w, "role inválido: use 'admin', 'stock_manager' ou 'viewer'")
		return
	}
	target, err := h.users.GetByEmail(r.Context(), req.Email)
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorBody{Error: "usuário não encontrado com este e-mail"})
		return
	}
	inviter, _ := auth.UserFromCtx(r.Context())
	invitedBy := &inviter.ID
	if err := h.members.AddMember(r.Context(), storeID, target.ID, role, invitedBy); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"message": "membro adicionado"})
}

// ----------------------------------------------------------------------------
// DELETE /stores/{id}/members/{userId} — remove member (RequireStoreRole admin)
// ----------------------------------------------------------------------------

func (h *StoreHandler) StoreRemoveMember(w http.ResponseWriter, r *http.Request) {
	storeID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "id inválido")
		return
	}
	memberID, err := parseUUID(chi.URLParam(r, "userId"))
	if err != nil {
		writeBadRequest(w, "userId inválido")
		return
	}
	// Prevent removing yourself (the owner/last admin).
	caller, _ := auth.UserFromCtx(r.Context())
	if caller.ID == memberID {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "não é possível remover a si mesmo da loja"})
		return
	}
	if err := h.members.RemoveMember(r.Context(), storeID, memberID); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "membro removido"})
}

// ----------------------------------------------------------------------------
// PATCH /stores/{id}/members/{userId}/role — update role (RequireStoreRole admin)
// ----------------------------------------------------------------------------

type storeUpdateRoleReq struct {
	Role string `json:"role"`
}

func (h *StoreHandler) StoreUpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	storeID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "id inválido")
		return
	}
	memberID, err := parseUUID(chi.URLParam(r, "userId"))
	if err != nil {
		writeBadRequest(w, "userId inválido")
		return
	}
	var req storeUpdateRoleReq
	if err := decodeJSON(r, &req); err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	role := user.StoreRole(req.Role)
	if user.StoreRoleLevel(role) == 0 {
		writeBadRequest(w, "role inválido")
		return
	}
	if err := h.members.UpdateMemberRole(r.Context(), storeID, memberID, role); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "role atualizado"})
}

// ----------------------------------------------------------------------------
// POST /stores/{id}/logo — store-facing logo upload (store admin role required)
// ----------------------------------------------------------------------------

func (h *StoreHandler) UploadLogo(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "id inválido")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 5<<20+4096)
	if err := r.ParseMultipartForm(5 << 20); err != nil {
		writeBadRequest(w, "arquivo muito grande (máx 5 MB) ou formulário inválido")
		return
	}
	file, header, err := r.FormFile("logo")
	if err != nil {
		writeBadRequest(w, "campo 'logo' ausente no formulário")
		return
	}
	defer file.Close()

	ext, mime, err := detectStoreImageType(file, header.Header.Get("Content-Type"))
	if err != nil {
		writeBadRequest(w, err.Error())
		return
	}

	key := "logos/" + uuid.New().String() + "." + ext
	logoURL, err := h.uploads.Put(r.Context(), key, file, mime)
	if err != nil {
		writeErr(w, err)
		return
	}

	old, err := h.stores.GetByID(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	s := old
	s.LogoURL = logoURL
	if err := h.stores.Update(r.Context(), &s); err != nil {
		writeErr(w, err)
		return
	}

	u, _ := auth.UserFromCtx(r.Context())
	_ = h.audit.Insert(r.Context(), s.ID, u.ID, "logo_upload",
		store.BuildDiff(old, s))

	writeJSON(w, http.StatusOK, s)
}

// detectStoreImageType mirrors detectImageType from admin.go but lives here
// so StoreHandler has no dependency on AdminHandler internals.
func detectStoreImageType(f io.ReadSeeker, headerCT string) (ext, mime string, err error) {
	if e, ok := allowedImageTypes[headerCT]; ok {
		return e, headerCT, nil
	}
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	if _, serr := f.Seek(0, io.SeekStart); serr != nil {
		return "", "", fmt.Errorf("erro ao ler arquivo")
	}
	detected := http.DetectContentType(buf[:n])
	if e, ok := allowedImageTypes[detected]; ok {
		return e, detected, nil
	}
	return "", "", fmt.Errorf("tipo de arquivo não suportado (use jpeg, png, webp ou gif)")
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
