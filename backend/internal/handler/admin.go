package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gustavojucoski/mercadotcg/backend/internal/auth"
	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/store"
	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/user"
	"github.com/gustavojucoski/mercadotcg/backend/internal/repository/postgres"
	"github.com/gustavojucoski/mercadotcg/backend/internal/service/document"
)

// AdminHandler expõe operações restritas a platform_admin.
type AdminHandler struct {
	users        *postgres.UserRepo
	stores       *postgres.StoreRepo
	storeMembers *postgres.StoreMemberRepo
	mw           *auth.Middleware
}

func NewAdminHandler(
	users *postgres.UserRepo,
	stores *postgres.StoreRepo,
	storeMembers *postgres.StoreMemberRepo,
	mw *auth.Middleware,
) *AdminHandler {
	return &AdminHandler{users: users, stores: stores, storeMembers: storeMembers, mw: mw}
}

func (h *AdminHandler) Routes(r chi.Router) {
	r.Route("/admin", func(r chi.Router) {
		r.Use(h.mw.RequirePlatformAdmin)

		// Usuários
		r.Get("/users", h.listUsers)
		r.Post("/users", h.createUser)
		r.Patch("/users/{id}/role", h.updateUserRole)
		r.Delete("/users/{id}", h.deactivateUser)

		// Lojas — literal route before wildcard {id}
		r.Get("/stores", h.listStores)
		r.Post("/stores", h.createStore)
		r.Get("/stores/cnpj-lookup", h.cnpjLookup)
		r.Get("/stores/{id}", h.getStore)
		r.Patch("/stores/{id}", h.updateStore)
		r.Post("/stores/{id}/verify-document", h.verifyDocument)
		r.Post("/stores/{id}/members", h.addMember)
		r.Patch("/stores/{id}/members/{userId}/role", h.updateMemberRole)
		r.Delete("/stores/{id}/members/{userId}", h.removeMember)
		r.Get("/stores/{id}/members", h.listMembers)
	})
}

// ---- Usuários ---------------------------------------------------------------

func (h *AdminHandler) listUsers(w http.ResponseWriter, r *http.Request) {
	limit := atoiOrDefault(r.URL.Query().Get("limit"), 50)
	offset := atoiOrDefault(r.URL.Query().Get("offset"), 0)
	users, err := h.users.List(r.Context(), limit, offset)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, users)
}

type adminCreateUserReq struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	Role        string `json:"platform_role,omitempty"` // "user" ou "platform_admin"
}

func (h *AdminHandler) createUser(w http.ResponseWriter, r *http.Request) {
	var req adminCreateUserReq
	if err := decodeJSON(r, &req); err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	if req.Email == "" || req.Password == "" || req.DisplayName == "" {
		writeBadRequest(w, "email, password e display_name são obrigatórios")
		return
	}

	role := user.RoleUser
	if req.Role == string(user.RolePlatformAdmin) {
		role = user.RolePlatformAdmin
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeErr(w, err)
		return
	}

	// Admin cria usuário já verificado — sem necessidade de confirmação por email.
	now := time.Now()
	u := &user.User{
		ID:              uuid.New(),
		Email:           req.Email,
		DisplayName:     req.DisplayName,
		PasswordHash:    hash,
		PlatformRole:    role,
		EmailVerifiedAt: &now,
		IsActive:        true,
	}
	if err := h.users.Create(r.Context(), u); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

type updateRoleReq struct {
	Role string `json:"platform_role"`
}

func (h *AdminHandler) updateUserRole(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "id inválido")
		return
	}
	var req updateRoleReq
	if err := decodeJSON(r, &req); err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	if req.Role != string(user.RoleUser) && req.Role != string(user.RolePlatformAdmin) {
		writeBadRequest(w, "platform_role inválido: use 'user' ou 'platform_admin'")
		return
	}
	if err := h.users.UpdatePlatformRole(r.Context(), id, user.PlatformRole(req.Role)); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "role atualizado"})
}

func (h *AdminHandler) deactivateUser(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "id inválido")
		return
	}
	if err := h.users.SetActive(r.Context(), id, false); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "usuário desativado"})
}

// ---- Lojas ------------------------------------------------------------------

func (h *AdminHandler) listStores(w http.ResponseWriter, r *http.Request) {
	limit := atoiOrDefault(r.URL.Query().Get("limit"), 50)
	offset := atoiOrDefault(r.URL.Query().Get("offset"), 0)
	stores, err := h.stores.List(r.Context(), limit, offset)
	if err != nil {
		writeErr(w, err)
		return
	}
	if stores == nil {
		stores = []store.Store{}
	}
	writeJSON(w, http.StatusOK, stores)
}

func (h *AdminHandler) getStore(w http.ResponseWriter, r *http.Request) {
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

type adminCreateStoreReq struct {
	OwnerID        string `json:"owner_id"`
	Name           string `json:"name"`
	Slug           string `json:"slug"`
	Description    string `json:"description,omitempty"`
	LogoURL        string `json:"logo_url,omitempty"`
	DocumentType   string `json:"document_type,omitempty"`   // "cpf" | "cnpj"
	DocumentNumber string `json:"document_number,omitempty"` // raw, with or without mask
}

func (h *AdminHandler) createStore(w http.ResponseWriter, r *http.Request) {
	var req adminCreateStoreReq
	if err := decodeJSON(r, &req); err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	if req.OwnerID == "" || req.Name == "" || req.Slug == "" {
		writeBadRequest(w, "owner_id, name e slug são obrigatórios")
		return
	}
	ownerID, err := parseUUID(req.OwnerID)
	if err != nil {
		writeBadRequest(w, "owner_id inválido")
		return
	}

	s := &store.Store{
		OwnerID:     ownerID,
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
		LogoURL:     req.LogoURL,
		IsActive:    true,
	}

	// Validate and resolve document when provided.
	if req.DocumentType != "" || req.DocumentNumber != "" {
		if req.DocumentType != string(store.DocumentTypeCPF) && req.DocumentType != string(store.DocumentTypeCNPJ) {
			writeBadRequest(w, "document_type deve ser 'cpf' ou 'cnpj'")
			return
		}
		dt := store.DocumentType(req.DocumentType)
		s.DocumentType = &dt

		var digits string
		switch dt {
		case store.DocumentTypeCNPJ:
			digits, err = document.ValidateCNPJ(req.DocumentNumber)
			if err != nil {
				writeJSON(w, 422, errorBody{Error: err.Error()})
				return
			}
			// Attempt ReceitaWS lookup; set status based on result.
			info, lerr := document.LookupCNPJ(r.Context(), digits)
			if lerr == nil {
				s.LegalName = &info.LegalName
				if info.Situation == "ATIVA" {
					s.DocumentStatus = store.DocumentStatusAutoVerified
				} else {
					s.DocumentStatus = store.DocumentStatusPending
				}
			} else {
				// Rate limit or network error: still save the document, leave pending.
				s.DocumentStatus = store.DocumentStatusPending
			}

		case store.DocumentTypeCPF:
			digits, err = document.ValidateCPF(req.DocumentNumber)
			if err != nil {
				writeJSON(w, 422, errorBody{Error: err.Error()})
				return
			}
			s.DocumentStatus = store.DocumentStatusPending
		}
		s.DocumentNumber = &digits
	}

	if err := h.stores.Create(r.Context(), s); err != nil {
		writeErr(w, err)
		return
	}

	// Adiciona o owner como admin da loja automaticamente.
	_ = h.storeMembers.AddMember(r.Context(), s.ID, ownerID, user.StoreRoleAdmin, nil)

	writeJSON(w, http.StatusCreated, s)
}

type adminUpdateStoreReq struct {
	Name        string `json:"name,omitempty"`
	Slug        string `json:"slug,omitempty"`
	Description string `json:"description,omitempty"`
	IsActive    *bool  `json:"is_active,omitempty"`
}

func (h *AdminHandler) updateStore(w http.ResponseWriter, r *http.Request) {
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

	var req adminUpdateStoreReq
	if err := decodeJSON(r, &req); err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	if req.Name != "" {
		s.Name = req.Name
	}
	if req.Slug != "" {
		s.Slug = req.Slug
	}
	if req.Description != "" {
		s.Description = req.Description
	}
	if req.IsActive != nil {
		s.IsActive = *req.IsActive
	}

	if err := h.stores.Update(r.Context(), &s); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *AdminHandler) verifyDocument(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "id inválido")
		return
	}
	admin, _ := auth.UserFromCtx(r.Context())
	if err := h.stores.SetDocumentVerified(r.Context(), id, admin.ID, store.DocumentStatusManuallyVerified); err != nil {
		writeErr(w, err)
		return
	}
	s, err := h.stores.GetByID(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *AdminHandler) cnpjLookup(w http.ResponseWriter, r *http.Request) {
	cnpj := r.URL.Query().Get("cnpj")
	if cnpj == "" {
		writeBadRequest(w, "parâmetro 'cnpj' é obrigatório")
		return
	}
	digits, err := document.ValidateCNPJ(cnpj)
	if err != nil {
		writeJSON(w, 422, errorBody{Error: err.Error()})
		return
	}
	info, err := document.LookupCNPJ(r.Context(), digits)
	if err != nil {
		if err == document.ErrRateLimit {
			writeJSON(w, http.StatusTooManyRequests, errorBody{Error: "limite de consultas atingido, tente em alguns segundos"})
			return
		}
		writeJSON(w, 422, errorBody{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"legal_name": info.LegalName,
		"situation":  info.Situation,
	})
}

// ---- Membros de loja --------------------------------------------------------

type addMemberReq struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

func (h *AdminHandler) addMember(w http.ResponseWriter, r *http.Request) {
	storeID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "store id inválido")
		return
	}
	var req addMemberReq
	if err := decodeJSON(r, &req); err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	userID, err := parseUUID(req.UserID)
	if err != nil {
		writeBadRequest(w, "user_id inválido")
		return
	}
	role := user.StoreRole(req.Role)
	if user.StoreRoleLevel(role) == 0 {
		writeBadRequest(w, "role inválido: use 'admin', 'stock_manager' ou 'viewer'")
		return
	}

	adminUser, _ := auth.UserFromCtx(r.Context())
	invitedBy := &adminUser.ID

	if err := h.storeMembers.AddMember(r.Context(), storeID, userID, role, invitedBy); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"message": "membro adicionado"})
}

type updateMemberRoleReq struct {
	Role string `json:"role"`
}

func (h *AdminHandler) updateMemberRole(w http.ResponseWriter, r *http.Request) {
	storeID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "store id inválido")
		return
	}
	memberID, err := parseUUID(chi.URLParam(r, "userId"))
	if err != nil {
		writeBadRequest(w, "userId inválido")
		return
	}
	var req updateMemberRoleReq
	if err := decodeJSON(r, &req); err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	role := user.StoreRole(req.Role)
	if user.StoreRoleLevel(role) == 0 {
		writeBadRequest(w, "role inválido")
		return
	}
	if err := h.storeMembers.UpdateMemberRole(r.Context(), storeID, memberID, role); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "role atualizado"})
}

func (h *AdminHandler) removeMember(w http.ResponseWriter, r *http.Request) {
	storeID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "store id inválido")
		return
	}
	memberID, err := parseUUID(chi.URLParam(r, "userId"))
	if err != nil {
		writeBadRequest(w, "userId inválido")
		return
	}
	if err := h.storeMembers.RemoveMember(r.Context(), storeID, memberID); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "membro removido"})
}

func (h *AdminHandler) listMembers(w http.ResponseWriter, r *http.Request) {
	storeID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeBadRequest(w, "store id inválido")
		return
	}
	members, err := h.storeMembers.ListMembers(r.Context(), storeID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, members)
}

