package handler

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/gustavojucoski/mercadotcg/backend/internal/auth"
	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/user"
	"github.com/gustavojucoski/mercadotcg/backend/internal/repository/postgres"
)

// AuthHandler expõe os endpoints de autenticação.
type AuthHandler struct {
	svc      *auth.Service
	tokenSvc *auth.TokenService
	mw       *auth.Middleware
	userRepo *postgres.UserRepo
	cfg      AuthHandlerConfig
}

// AuthHandlerConfig contém parâmetros de runtime para o handler.
type AuthHandlerConfig struct {
	FrontendBaseURL string
}

func NewAuthHandler(svc *auth.Service, tokenSvc *auth.TokenService, mw *auth.Middleware, userRepo *postgres.UserRepo, cfg AuthHandlerConfig) *AuthHandler {
	return &AuthHandler{svc: svc, tokenSvc: tokenSvc, mw: mw, userRepo: userRepo, cfg: cfg}
}

func (h *AuthHandler) Routes(r chi.Router) {
	r.Post("/auth/register", h.register)
	r.Post("/auth/login", h.login)
	r.Get("/auth/google", h.googleRedirect)
	r.Get("/auth/google/callback", h.googleCallback)
	r.Post("/auth/verify-email", h.verifyEmail)
	r.Post("/auth/forgot-password", h.forgotPassword)
	r.Post("/auth/reset-password", h.resetPassword)
	r.Post("/auth/refresh", h.refresh)
	r.Post("/auth/logout", h.logout)
	r.With(h.mw.RequireAuth).Get("/auth/me", h.me)
}

// ----------------------------------------------------------------------------
// POST /auth/register
// ----------------------------------------------------------------------------

type registerReq struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

func (h *AuthHandler) register(w http.ResponseWriter, r *http.Request) {
	var req registerReq
	if err := decodeJSON(r, &req); err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	if req.Email == "" || req.Password == "" || req.DisplayName == "" {
		writeBadRequest(w, "email, password e display_name são obrigatórios")
		return
	}
	if len(req.Password) < 8 {
		writeBadRequest(w, "senha deve ter pelo menos 8 caracteres")
		return
	}

	u, err := h.svc.RegisterWithEmail(r.Context(), req.Email, req.Password, req.DisplayName)
	if err != nil {
		if errors.Is(err, postgres.ErrAlreadyExists) {
			writeJSON(w, http.StatusConflict, errorBody{Error: "email já cadastrado"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"message": "verifique seu email para ativar a conta",
		"user_id": u.ID,
	})
}

// ----------------------------------------------------------------------------
// POST /auth/login
// ----------------------------------------------------------------------------

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type tokenResp struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresIn    int       `json:"expires_in"`
	User         user.User `json:"user"`
}

func (h *AuthHandler) login(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := decodeJSON(r, &req); err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	if req.Email == "" || req.Password == "" {
		writeBadRequest(w, "email e password são obrigatórios")
		return
	}

	access, refresh, u, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidCredentials):
			writeJSON(w, http.StatusUnauthorized, errorBody{Error: "credenciais inválidas"})
		case errors.Is(err, auth.ErrEmailNotVerified):
			writeJSON(w, http.StatusForbidden, errorBody{Error: "email não verificado"})
		case errors.Is(err, auth.ErrAccountInactive):
			writeJSON(w, http.StatusForbidden, errorBody{Error: "conta desativada"})
		default:
			writeErr(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, tokenResp{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int(h.tokenSvc.AccessTTL().Seconds()),
		User:         *u,
	})
}

// ----------------------------------------------------------------------------
// GET /auth/google
// ----------------------------------------------------------------------------

func (h *AuthHandler) googleRedirect(w http.ResponseWriter, r *http.Request) {
	oauthSvc := h.svc.OAuth()
	if !oauthSvc.IsConfigured() {
		writeJSON(w, http.StatusNotImplemented, errorBody{Error: "Google OAuth não configurado"})
		return
	}
	url, _, err := oauthSvc.AuthCodeURL()
	if err != nil {
		writeErr(w, err)
		return
	}
	http.Redirect(w, r, url, http.StatusFound)
}

// ----------------------------------------------------------------------------
// GET /auth/google/callback
// ----------------------------------------------------------------------------

func (h *AuthHandler) googleCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		writeBadRequest(w, "parâmetros ausentes no callback")
		return
	}

	access, refresh, _, err := h.svc.GoogleCallback(r.Context(), code, state)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, errorBody{Error: "autenticação Google falhou"})
		return
	}

	// Redireciona para o frontend com os tokens na query string.
	// O frontend lê e armazena os tokens na página /auth/callback.
	redirectURL := h.cfg.FrontendBaseURL + "/auth/callback?access_token=" + access + "&refresh_token=" + refresh
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// ----------------------------------------------------------------------------
// POST /auth/verify-email
// ----------------------------------------------------------------------------

type verifyEmailReq struct {
	Token string `json:"token"`
}

func (h *AuthHandler) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var req verifyEmailReq
	if err := decodeJSON(r, &req); err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	if req.Token == "" {
		writeBadRequest(w, "token é obrigatório")
		return
	}
	if err := h.svc.VerifyEmail(r.Context(), req.Token); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "token inválido ou expirado"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "email verificado com sucesso"})
}

// ----------------------------------------------------------------------------
// POST /auth/forgot-password
// ----------------------------------------------------------------------------

type forgotPasswordReq struct {
	Email string `json:"email"`
}

func (h *AuthHandler) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var req forgotPasswordReq
	if err := decodeJSON(r, &req); err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	// Sempre retorna 200 para evitar enumeração de emails.
	_ = h.svc.ForgotPassword(r.Context(), req.Email)
	writeJSON(w, http.StatusOK, map[string]string{
		"message": "se o email estiver cadastrado, você receberá um link de redefinição",
	})
}

// ----------------------------------------------------------------------------
// POST /auth/reset-password
// ----------------------------------------------------------------------------

type resetPasswordReq struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

func (h *AuthHandler) resetPassword(w http.ResponseWriter, r *http.Request) {
	var req resetPasswordReq
	if err := decodeJSON(r, &req); err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	if req.Token == "" || req.NewPassword == "" {
		writeBadRequest(w, "token e new_password são obrigatórios")
		return
	}
	if len(req.NewPassword) < 8 {
		writeBadRequest(w, "senha deve ter pelo menos 8 caracteres")
		return
	}
	if err := h.svc.ResetPassword(r.Context(), req.Token, req.NewPassword); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "token inválido ou expirado"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "senha redefinida com sucesso"})
}

// ----------------------------------------------------------------------------
// POST /auth/refresh
// ----------------------------------------------------------------------------

type refreshReq struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *AuthHandler) refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshReq
	if err := decodeJSON(r, &req); err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	if req.RefreshToken == "" {
		writeBadRequest(w, "refresh_token é obrigatório")
		return
	}
	access, u, err := h.svc.RefreshAccess(r.Context(), req.RefreshToken)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, errorBody{Error: "refresh token inválido ou expirado"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": access,
		"expires_in":   int(h.tokenSvc.AccessTTL().Seconds()),
		"user":         u,
	})
}

// ----------------------------------------------------------------------------
// POST /auth/logout
// ----------------------------------------------------------------------------

type logoutReq struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *AuthHandler) logout(w http.ResponseWriter, r *http.Request) {
	var req logoutReq
	if err := decodeJSON(r, &req); err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	_ = h.svc.Logout(r.Context(), req.RefreshToken)
	writeJSON(w, http.StatusOK, map[string]string{"message": "sessão encerrada"})
}

// ----------------------------------------------------------------------------
// GET /auth/me
// ----------------------------------------------------------------------------

func (h *AuthHandler) me(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromCtx(r.Context())
	full, err := h.userRepo.GetByID(r.Context(), u.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody{Error: "erro ao buscar usuário"})
		return
	}
	writeJSON(w, http.StatusOK, full)
}
