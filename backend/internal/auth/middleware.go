package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/user"
)

type ctxKeyUser struct{}

// UserFromCtx recupera o usuário autenticado do contexto.
func UserFromCtx(ctx context.Context) (user.User, bool) {
	u, ok := ctx.Value(ctxKeyUser{}).(user.User)
	return u, ok
}

// StoreMemberRepository é o subconjunto usado pelo middleware para verificar roles.
type StoreMemberRepository interface {
	GetMembership(ctx context.Context, storeID, userID uuid.UUID) (user.StoreRole, error)
}

// Middleware contém os middlewares chi de autenticação e autorização.
type Middleware struct {
	tokenSvc     *TokenService
	storeMembers StoreMemberRepository
}

func NewMiddleware(tokenSvc *TokenService, storeMembers StoreMemberRepository) *Middleware {
	return &Middleware{tokenSvc: tokenSvc, storeMembers: storeMembers}
}

// RequireAuth extrai o Bearer token, valida e injeta o User no contexto.
// Responde 401 se ausente ou inválido.
func (m *Middleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := m.extractUser(w, r)
		if !ok {
			return
		}
		ctx := context.WithValue(r.Context(), ctxKeyUser{}, u)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequirePlatformAdmin requer platform_role == platform_admin.
// Encadeia RequireAuth implicitamente.
func (m *Middleware) RequirePlatformAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := m.extractUser(w, r)
		if !ok {
			return
		}
		if u.PlatformRole != user.RolePlatformAdmin {
			writeUnauthorized(w, "acesso restrito a administradores da plataforma")
			return
		}
		ctx := context.WithValue(r.Context(), ctxKeyUser{}, u)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireStoreRole gera um middleware que exige o usuário ter pelo menos minRole
// na loja identificada pelo parâmetro de URL {id}.
// Platform admins bypassam a verificação de membro.
func (m *Middleware) RequireStoreRole(minRole user.StoreRole) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, ok := m.extractUser(w, r)
			if !ok {
				return
			}
			// Platform admins têm acesso irrestrito a todas as lojas.
			if u.PlatformRole == user.RolePlatformAdmin {
				ctx := context.WithValue(r.Context(), ctxKeyUser{}, u)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			storeIDStr := chi.URLParam(r, "id")
			storeID, err := uuid.Parse(storeIDStr)
			if err != nil {
				writeForbidden(w, "loja não encontrada")
				return
			}

			role, err := m.storeMembers.GetMembership(r.Context(), storeID, u.ID)
			if err != nil {
				writeForbidden(w, "acesso negado")
				return
			}
			if user.StoreRoleLevel(role) < user.StoreRoleLevel(minRole) {
				writeForbidden(w, "permissão insuficiente para esta operação")
				return
			}

			ctx := context.WithValue(r.Context(), ctxKeyUser{}, u)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ---- helpers internos -------------------------------------------------------

func (m *Middleware) extractUser(w http.ResponseWriter, r *http.Request) (user.User, bool) {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		writeUnauthorized(w, "token de autenticação ausente")
		return user.User{}, false
	}
	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
	claims, err := m.tokenSvc.ParseAccessToken(tokenStr)
	if err != nil {
		writeUnauthorized(w, "token inválido ou expirado")
		return user.User{}, false
	}
	id, err := UserIDFromClaims(claims)
	if err != nil {
		writeUnauthorized(w, "token corrompido")
		return user.User{}, false
	}
	return user.User{
		ID:           id,
		Email:        claims.Email,
		PlatformRole: claims.PlatformRole,
	}, true
}

func writeUnauthorized(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"` + msg + `"}`))
}

func writeForbidden(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(`{"error":"` + msg + `"}`))
}
