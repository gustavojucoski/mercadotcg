package handler

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/gustavojucoski/mercadotcg/backend/internal/auth"
	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/card"
	"github.com/gustavojucoski/mercadotcg/backend/internal/repository/postgres"
	"github.com/gustavojucoski/mercadotcg/backend/internal/upload"
)

// AdminCatalogHandler expõe operações de curadoria de catálogo restritas a platform_admin.
type AdminCatalogHandler struct {
	cards   *postgres.CardRepo
	uploads upload.Provider
	mw      *auth.Middleware
	log     zerolog.Logger
}

// NewAdminCatalogHandler cria o handler.
func NewAdminCatalogHandler(cards *postgres.CardRepo, uploads upload.Provider, mw *auth.Middleware, log zerolog.Logger) *AdminCatalogHandler {
	return &AdminCatalogHandler{cards: cards, uploads: uploads, mw: mw, log: log}
}

// Routes registra as rotas no router já montado no prefixo /api/v1.
// Todas as rotas exigem platform_admin (aplicado via r.With).
func (h *AdminCatalogHandler) Routes(r chi.Router) {
	admin := r.With(h.mw.RequirePlatformAdmin)

	// Sets
	admin.Get("/admin/sets", h.listSets)
	admin.Post("/admin/sets", h.createSet)
	admin.Get("/admin/sets/{id}", h.getSet)
	admin.Patch("/admin/sets/{id}", h.patchSet)
	admin.Post("/admin/sets/{id}/image", h.uploadSetImage)
	admin.Post("/admin/sets/{id}/symbol", h.uploadSetSymbol)
	admin.Delete("/admin/sets/{id}", h.deleteSet)

	// Cards
	admin.Post("/admin/cards", h.createCard)
	admin.Get("/admin/cards/{id}", h.getCard)
	admin.Patch("/admin/cards/{id}", h.patchCard)
	admin.Post("/admin/cards/{id}/image", h.uploadCardImage)
	admin.Post("/admin/cards/{id}/image-pt", h.uploadCardImagePT)
	admin.Delete("/admin/cards/{id}", h.deleteCard)
	admin.Get("/admin/cards/{id}/variants", h.listCardVariants)
	admin.Post("/admin/cards/{id}/variants", h.createVariant)

	// Variants
	admin.Patch("/admin/variants/{id}", h.patchVariant)
	admin.Delete("/admin/variants/{id}", h.deleteVariant)
}

// ---- Audit ------------------------------------------------------------------

func (h *AdminCatalogHandler) audit(r *http.Request, action, entity, entityID string) {
	u, _ := auth.UserFromCtx(r.Context())
	h.log.Info().
		Str("action", action).
		Str("entity", entity).
		Str("entity_id", entityID).
		Str("admin_user_id", u.ID.String()).
		Msg("catalog_audit")
}

// ---- Requests ---------------------------------------------------------------

type createSetReq struct {
	Code         string     `json:"code"`
	Name         string     `json:"name"`
	NamePT       string     `json:"name_pt,omitempty"`
	NameEN       string     `json:"name_en,omitempty"`
	SeriesID     *uuid.UUID `json:"series_id,omitempty"`
	Language     string     `json:"language"`
	TCG          string     `json:"tcg"`
	ReleaseDate  *time.Time `json:"release_date,omitempty"`
	TotalCards   int        `json:"total_cards,omitempty"`
	PrintedTotal int        `json:"printed_total,omitempty"`
}

type patchSetReq struct {
	Name         *string    `json:"name,omitempty"`
	NamePT       *string    `json:"name_pt,omitempty"`
	NameEN       *string    `json:"name_en,omitempty"`
	SeriesID     *uuid.UUID `json:"series_id,omitempty"`
	ReleaseDate  *time.Time `json:"release_date,omitempty"`
	TotalCards   *int       `json:"total_cards,omitempty"`
	PrintedTotal *int       `json:"printed_total,omitempty"`
}

type deleteSetReq struct {
	ConfirmCode string `json:"confirm_code"`
}

type createCardReq struct {
	SetID           string   `json:"set_id"`
	CollectorNumber string   `json:"collector_number"`
	Name            string   `json:"name"`
	NamePT          string   `json:"name_pt,omitempty"`
	Rarity          string   `json:"rarity,omitempty"`
	Supertype       string   `json:"supertype,omitempty"`
	Subtypes        []string `json:"subtypes,omitempty"`
	Types           []string `json:"types,omitempty"`
	HP              int      `json:"hp,omitempty"`
	Illustrator     string   `json:"illustrator,omitempty"`
}

type patchCardReq struct {
	Name            *string   `json:"name,omitempty"`
	NamePT          *string   `json:"name_pt,omitempty"`
	CollectorNumber *string   `json:"collector_number,omitempty"`
	Rarity          *string   `json:"rarity,omitempty"`
	Supertype       *string   `json:"supertype,omitempty"`
	Subtypes        *[]string `json:"subtypes,omitempty"`
	Types           *[]string `json:"types,omitempty"`
	HP              *int      `json:"hp,omitempty"`
	Illustrator     *string   `json:"illustrator,omitempty"`
}

type deleteCardReq struct {
	ConfirmCollectorNumber string `json:"confirm_collector_number"`
}

type createVariantReq struct {
	Finish  string `json:"finish"`
	Label   string `json:"label,omitempty"`
	IsPromo bool   `json:"is_promo"`
	Notes   string `json:"notes,omitempty"`
}

type patchVariantReq struct {
	Finish  *string `json:"finish,omitempty"`
	Label   *string `json:"label,omitempty"`
	IsPromo *bool   `json:"is_promo,omitempty"`
	Notes   *string `json:"notes,omitempty"`
}

type deleteVariantReq struct {
	Confirm bool `json:"confirm"`
}

// ---- Sets -------------------------------------------------------------------

// GET /admin/sets?tcg=pokemon&series_id=uuid&q=sv&page=1&limit=50
func (h *AdminCatalogHandler) listSets(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	tcg := q.Get("tcg")
	if tcg == "" {
		writeBadRequest(w, "parâmetro tcg é obrigatório")
		return
	}

	var seriesID *uuid.UUID
	if raw := q.Get("series_id"); raw != "" {
		id, err := parseUUID(raw)
		if err != nil {
			writeBadRequest(w, "series_id inválido")
			return
		}
		seriesID = &id
	}

	search := q.Get("q")
	page := atoiOrDefault(q.Get("page"), 1)
	limit := atoiOrDefault(q.Get("limit"), 50)
	if limit > 100 {
		limit = 100
	}

	sets, total, err := h.cards.ListSetsByTCGFiltered(r.Context(), tcg, seriesID, search, page, limit)
	if err != nil {
		writeErr(w, err)
		return
	}
	if sets == nil {
		sets = []postgres.SetWithSeries{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": sets,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GET /admin/sets/{id}
func (h *AdminCatalogHandler) getSet(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "id inválido")
		return
	}
	s, err := h.cards.GetSetByID(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

// POST /admin/sets
func (h *AdminCatalogHandler) createSet(w http.ResponseWriter, r *http.Request) {
	var req createSetReq
	if err := decodeJSON(r, &req); err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	if req.Code == "" || req.Name == "" || req.TCG == "" || req.Language == "" {
		writeBadRequest(w, "code, name, tcg e language são obrigatórios")
		return
	}

	s := &card.Set{
		Code:         req.Code,
		Name:         req.Name,
		NamePT:       req.NamePT,
		NameEN:       req.NameEN,
		SeriesID:     req.SeriesID,
		Language:     card.Language(req.Language),
		TCG:          req.TCG,
		ReleaseDate:  req.ReleaseDate,
		TotalCards:   req.TotalCards,
		PrintedTotal: req.PrintedTotal,
	}

	if err := h.cards.CreateSetAdmin(r.Context(), s); err != nil {
		writeErr(w, err)
		return
	}

	h.audit(r, "create", "set", s.ID.String())
	writeJSON(w, http.StatusCreated, s)
}

// PATCH /admin/sets/{id}
func (h *AdminCatalogHandler) patchSet(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "id inválido")
		return
	}
	var req patchSetReq
	if err := decodeJSON(r, &req); err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	if req.Name != nil && *req.Name == "" {
		writeBadRequest(w, "name não pode ser vazio")
		return
	}

	patch := postgres.SetPatch{
		Name:         req.Name,
		NamePT:       req.NamePT,
		NameEN:       req.NameEN,
		SeriesID:     req.SeriesID,
		ReleaseDate:  req.ReleaseDate,
		TotalCards:   req.TotalCards,
		PrintedTotal: req.PrintedTotal,
	}

	updated, err := h.cards.UpdateSet(r.Context(), id, patch)
	if err != nil {
		writeErr(w, err)
		return
	}

	h.audit(r, "patch", "set", id.String())
	writeJSON(w, http.StatusOK, updated)
}

// POST /admin/sets/{id}/image
func (h *AdminCatalogHandler) uploadSetImage(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "id inválido")
		return
	}
	url, err := h.handleImageUpload(r, w, "image", fmt.Sprintf("sets/%s/image", id))
	if err != nil {
		return
	}
	if err := h.cards.UpdateSetImageURL(r.Context(), id, url); err != nil {
		writeErr(w, err)
		return
	}
	h.audit(r, "upload_image", "set", id.String())
	writeJSON(w, http.StatusOK, map[string]string{"image_url": url})
}

// POST /admin/sets/{id}/symbol
func (h *AdminCatalogHandler) uploadSetSymbol(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "id inválido")
		return
	}
	url, err := h.handleImageUpload(r, w, "image", fmt.Sprintf("sets/%s/symbol", id))
	if err != nil {
		return
	}
	if err := h.cards.UpdateSetSymbolURL(r.Context(), id, url); err != nil {
		writeErr(w, err)
		return
	}
	h.audit(r, "upload_symbol", "set", id.String())
	writeJSON(w, http.StatusOK, map[string]string{"symbol_url": url})
}

// DELETE /admin/sets/{id}
func (h *AdminCatalogHandler) deleteSet(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "id inválido")
		return
	}

	var req deleteSetReq
	if err := decodeJSON(r, &req); err != nil {
		writeBadRequest(w, err.Error())
		return
	}

	s, err := h.cards.GetSetByID(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if req.ConfirmCode != s.Code {
		writeBadRequest(w, "confirm_code não corresponde ao code do set")
		return
	}

	err = h.cards.DeleteSetWithCards(r.Context(), id)
	var blocked postgres.ErrDeleteBlocked
	if errors.As(err, &blocked) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "set contém cartas em uso",
			"blocked_by": map[string]int{
				"cards_with_stock":    blocked.Stock,
				"cards_with_listings": blocked.Listings,
			},
		})
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}

	h.audit(r, "delete", "set", id.String())
	w.WriteHeader(http.StatusNoContent)
}

// ---- Cards ------------------------------------------------------------------

// GET /admin/cards/{id}
func (h *AdminCatalogHandler) getCard(w http.ResponseWriter, r *http.Request) {
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

// POST /admin/cards
func (h *AdminCatalogHandler) createCard(w http.ResponseWriter, r *http.Request) {
	var req createCardReq
	if err := decodeJSON(r, &req); err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	if req.SetID == "" || req.CollectorNumber == "" || req.Name == "" {
		writeBadRequest(w, "set_id, collector_number e name são obrigatórios")
		return
	}
	setID, err := parseUUID(req.SetID)
	if err != nil {
		writeBadRequest(w, "set_id inválido")
		return
	}

	c := &card.Card{
		SetID:           setID,
		CollectorNumber: req.CollectorNumber,
		Name:            req.Name,
		NamePT:          req.NamePT,
		Rarity:          req.Rarity,
		Supertype:       req.Supertype,
		Subtypes:        req.Subtypes,
		Types:           req.Types,
		HP:              req.HP,
		Illustrator:     req.Illustrator,
	}

	if err := h.cards.CreateCardAdmin(r.Context(), c); err != nil {
		writeErr(w, err)
		return
	}

	h.audit(r, "create", "card", c.ID.String())
	writeJSON(w, http.StatusCreated, c)
}

// PATCH /admin/cards/{id}
func (h *AdminCatalogHandler) patchCard(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "id inválido")
		return
	}
	var req patchCardReq
	if err := decodeJSON(r, &req); err != nil {
		writeBadRequest(w, err.Error())
		return
	}

	patch := postgres.CardPatch{
		Name:            req.Name,
		NamePT:          req.NamePT,
		CollectorNumber: req.CollectorNumber,
		Rarity:          req.Rarity,
		Supertype:       req.Supertype,
		Subtypes:        req.Subtypes,
		Types:           req.Types,
		HP:              req.HP,
		Illustrator:     req.Illustrator,
	}

	updated, err := h.cards.UpdateCard(r.Context(), id, patch)
	if err != nil {
		writeErr(w, err)
		return
	}

	h.audit(r, "patch", "card", id.String())
	writeJSON(w, http.StatusOK, updated)
}

// POST /admin/cards/{id}/image
func (h *AdminCatalogHandler) uploadCardImage(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "id inválido")
		return
	}
	url, err := h.handleImageUpload(r, w, "image", fmt.Sprintf("cards/%s/image_en", id))
	if err != nil {
		return
	}
	if err := h.cards.UpdateCardImages(r.Context(), id, url, url); err != nil {
		writeErr(w, err)
		return
	}
	h.audit(r, "upload_image_en", "card", id.String())
	writeJSON(w, http.StatusOK, map[string]string{
		"image_small_url": url,
		"image_large_url": url,
	})
}

// POST /admin/cards/{id}/image-pt
func (h *AdminCatalogHandler) uploadCardImagePT(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "id inválido")
		return
	}
	url, err := h.handleImageUpload(r, w, "image", fmt.Sprintf("cards/%s/image_pt", id))
	if err != nil {
		return
	}
	if err := h.cards.UpdateCardImagePT(r.Context(), id, url); err != nil {
		writeErr(w, err)
		return
	}
	h.audit(r, "upload_image_pt", "card", id.String())
	writeJSON(w, http.StatusOK, map[string]string{"image_url_pt": url})
}

// DELETE /admin/cards/{id}
func (h *AdminCatalogHandler) deleteCard(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "id inválido")
		return
	}

	var req deleteCardReq
	if err := decodeJSON(r, &req); err != nil {
		writeBadRequest(w, err.Error())
		return
	}

	c, err := h.cards.GetCardByID(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if req.ConfirmCollectorNumber != c.CollectorNumber {
		writeBadRequest(w, "confirm_collector_number não corresponde ao collector_number da carta")
		return
	}

	err = h.cards.DeleteCard(r.Context(), id)
	var blocked postgres.ErrDeleteBlocked
	if errors.As(err, &blocked) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "carta em uso",
			"blocked_by": map[string]int{
				"stock_items": blocked.Stock,
				"listings":    blocked.Listings,
			},
		})
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}

	h.audit(r, "delete", "card", id.String())
	w.WriteHeader(http.StatusNoContent)
}

// GET /admin/cards/{id}/variants
func (h *AdminCatalogHandler) listCardVariants(w http.ResponseWriter, r *http.Request) {
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

// POST /admin/cards/{id}/variants
func (h *AdminCatalogHandler) createVariant(w http.ResponseWriter, r *http.Request) {
	cardID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "id inválido")
		return
	}

	var req createVariantReq
	if err := decodeJSON(r, &req); err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	if !isValidFinish(req.Finish) {
		writeBadRequest(w, "finish inválido — valores: normal, holo, reverse_holo, master_ball_mirror, poke_ball_mirror, cosmos_holo, galaxy_holo, textured, gold_etched, first_edition, shadowless, unlimited")
		return
	}

	v := &card.Variant{
		CardID:  cardID,
		Finish:  card.Finish(req.Finish),
		Label:   req.Label,
		IsPromo: req.IsPromo,
		Notes:   req.Notes,
	}

	if err := h.cards.CreateVariant(r.Context(), v); err != nil {
		writeErr(w, err)
		return
	}

	h.audit(r, "create", "variant", v.ID.String())
	writeJSON(w, http.StatusCreated, v)
}

// ---- Variants ---------------------------------------------------------------

// PATCH /admin/variants/{id}
func (h *AdminCatalogHandler) patchVariant(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "id inválido")
		return
	}
	var req patchVariantReq
	if err := decodeJSON(r, &req); err != nil {
		writeBadRequest(w, err.Error())
		return
	}

	patch := postgres.VariantPatch{
		Label:   req.Label,
		IsPromo: req.IsPromo,
		Notes:   req.Notes,
	}
	if req.Finish != nil {
		if !isValidFinish(*req.Finish) {
			writeBadRequest(w, "finish inválido")
			return
		}
		f := card.Finish(*req.Finish)
		patch.Finish = &f
	}

	updated, err := h.cards.UpdateVariant(r.Context(), id, patch)
	if err != nil {
		writeErr(w, err)
		return
	}

	h.audit(r, "patch", "variant", id.String())
	writeJSON(w, http.StatusOK, updated)
}

// DELETE /admin/variants/{id}
func (h *AdminCatalogHandler) deleteVariant(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "id inválido")
		return
	}

	var req deleteVariantReq
	if err := decodeJSON(r, &req); err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	if !req.Confirm {
		writeBadRequest(w, "confirm deve ser true para apagar a variante")
		return
	}

	err = h.cards.DeleteVariant(r.Context(), id)
	var blocked postgres.ErrDeleteBlocked
	if errors.As(err, &blocked) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "variante em uso",
			"blocked_by": map[string]int{
				"stock_items": blocked.Stock,
				"listings":    blocked.Listings,
			},
		})
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}

	h.audit(r, "delete", "variant", id.String())
	w.WriteHeader(http.StatusNoContent)
}

// ---- Helpers ----------------------------------------------------------------

// handleImageUpload faz parse de multipart, detecta tipo e chama uploads.Put.
// Retorna a URL pública ou escreve o erro na resposta (retorna "" em caso de erro).
func (h *AdminCatalogHandler) handleImageUpload(r *http.Request, w http.ResponseWriter, field, keyBase string) (string, error) {
	r.Body = http.MaxBytesReader(w, r.Body, 5<<20+4096)
	if err := r.ParseMultipartForm(5 << 20); err != nil {
		writeBadRequest(w, "arquivo muito grande (máx 5 MB) ou formulário inválido")
		return "", err
	}
	file, header, err := r.FormFile(field)
	if err != nil {
		writeBadRequest(w, fmt.Sprintf("campo '%s' ausente no formulário", field))
		return "", err
	}
	defer file.Close()

	ext, mime, err := detectImageType(file, header.Header.Get("Content-Type"))
	if err != nil {
		writeBadRequest(w, err.Error())
		return "", err
	}

	key := keyBase + "." + ext
	url, err := h.uploads.Put(r.Context(), key, file, mime)
	if err != nil {
		writeErr(w, err)
		return "", err
	}
	return url, nil
}

// isValidFinish verifica se o valor de finish é um ENUM válido.
func isValidFinish(f string) bool {
	switch card.Finish(f) {
	case card.FinishNormal, card.FinishHolo, card.FinishReverseHolo,
		card.FinishMasterBallMirror, card.FinishPokeBallMirror,
		card.FinishCosmosHolo, card.FinishGalaxyHolo, card.FinishTextured,
		card.FinishGoldEtched, card.FinishFirstEdition,
		card.FinishShadowless, card.FinishUnlimited:
		return true
	}
	return false
}
