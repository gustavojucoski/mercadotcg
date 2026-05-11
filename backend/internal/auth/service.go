package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/user"
	"github.com/gustavojucoski/mercadotcg/backend/internal/email"
)

// Erros de domínio do AuthService.
var (
	ErrEmailNotVerified    = errors.New("auth: email não verificado")
	ErrAccountInactive     = errors.New("auth: conta desativada")
	ErrTokenNotFound       = errors.New("auth: token inválido ou expirado")
	ErrEmailAlreadyVerified = errors.New("auth: email já verificado")
)

// UserRepository é o subconjunto de persistência de usuários usado pelo AuthService.
type UserRepository interface {
	Create(ctx context.Context, u *user.User) error
	GetByID(ctx context.Context, id uuid.UUID) (user.User, error)
	GetByEmail(ctx context.Context, email string) (user.User, error)
	UpdatePasswordHash(ctx context.Context, id uuid.UUID, hash string) error
	MarkEmailVerified(ctx context.Context, id uuid.UUID) error
	CompleteRegistration(ctx context.Context, id uuid.UUID, displayName, passwordHash string) error
	GetByOAuthProvider(ctx context.Context, provider, providerUID string) (user.User, error)
	LinkOAuthProvider(ctx context.Context, userID uuid.UUID, provider, providerUID string) error
}

// TokenRepository é o subconjunto de persistência de tokens usado pelo AuthService.
type TokenRepository interface {
	CreateVerificationToken(ctx context.Context, userID uuid.UUID, hash string, expiresAt time.Time) error
	UseVerificationToken(ctx context.Context, hash string) (uuid.UUID, error)
	CreatePasswordResetToken(ctx context.Context, userID uuid.UUID, hash string, expiresAt time.Time) error
	UsePasswordResetToken(ctx context.Context, hash string) (uuid.UUID, error)
	CreateRefreshToken(ctx context.Context, userID uuid.UUID, hash string, expiresAt time.Time) error
	GetRefreshToken(ctx context.Context, hash string) (RefreshTokenRecord, error)
	RevokeRefreshToken(ctx context.Context, hash string) error
}

// ServiceConfig contém opções de runtime do AuthService.
type ServiceConfig struct {
	FrontendBaseURL string
}

// Service orquestra todos os fluxos de autenticação.
type Service struct {
	users    UserRepository
	tokens   TokenRepository
	tokenSvc *TokenService
	oauth    *OAuthService
	mailer   email.Provider
	cfg      ServiceConfig
}

func NewService(
	users UserRepository,
	tokens TokenRepository,
	tokenSvc *TokenService,
	oauth *OAuthService,
	mailer email.Provider,
	cfg ServiceConfig,
) *Service {
	return &Service{users: users, tokens: tokens, tokenSvc: tokenSvc, oauth: oauth, mailer: mailer, cfg: cfg}
}

// ---- Registro ---------------------------------------------------------------

// RegisterWithEmail cria uma conta com email e envia o link de verificação.
// O usuário só completa o cadastro (nome + senha) ao clicar no link (VerifyEmail).
func (s *Service) RegisterWithEmail(ctx context.Context, emailAddr string) (*user.User, error) {
	u := &user.User{
		ID:           uuid.New(),
		Email:        emailAddr,
		DisplayName:  emailAddr,
		PlatformRole: user.RoleUser,
		IsActive:     true,
	}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, err
	}
	if err := s.sendVerificationEmail(ctx, u); err != nil {
		log.Error().Err(err).Str("email", u.Email).Msg("enviar email de verificação")
	}
	return u, nil
}

// ResendVerificationEmail reenvia o link de verificação para contas ainda não verificadas.
// Retorna ErrEmailAlreadyVerified se a conta já foi ativada.
func (s *Service) ResendVerificationEmail(ctx context.Context, emailAddr string) error {
	u, err := s.users.GetByEmail(ctx, emailAddr)
	if err != nil {
		return nil // anti-enumeração: não revelar se o email existe
	}
	if u.IsVerified() {
		return ErrEmailAlreadyVerified
	}
	if err := s.sendVerificationEmail(ctx, &u); err != nil {
		log.Error().Err(err).Str("email", u.Email).Msg("reenviar email de verificação")
	}
	return nil
}

func (s *Service) sendVerificationEmail(ctx context.Context, u *user.User) error {
	plain, hash, err := generateToken()
	if err != nil {
		return err
	}
	expiresAt := time.Now().Add(24 * time.Hour)
	if err := s.tokens.CreateVerificationToken(ctx, u.ID, hash, expiresAt); err != nil {
		return fmt.Errorf("criar token de verificação: %w", err)
	}
	verifyURL := s.cfg.FrontendBaseURL + "/auth/verify-email?token=" + plain
	log.Info().Str("email", u.Email).Str("verify_url", verifyURL).Msg("[dev] link de verificação")
	msg := email.VerificationEmail(u.Email, u.DisplayName, verifyURL)
	return s.mailer.Send(ctx, msg)
}

// ---- Verificação de email ---------------------------------------------------

// VerifyEmail consome o token de verificação, define nome e senha, e emite tokens de sessão.
func (s *Service) VerifyEmail(ctx context.Context, plainToken, password, displayName string) (accessToken, plainRefresh string, u *user.User, err error) {
	hash := hashToken(plainToken)
	userID, err := s.tokens.UseVerificationToken(ctx, hash)
	if err != nil {
		return "", "", nil, ErrTokenNotFound
	}
	pwHash, err := HashPassword(password)
	if err != nil {
		return "", "", nil, err
	}
	if err := s.users.CompleteRegistration(ctx, userID, displayName, pwHash); err != nil {
		return "", "", nil, fmt.Errorf("completar registro: %w", err)
	}
	fetchedUser, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return "", "", nil, fmt.Errorf("buscar usuário após registro: %w", err)
	}
	return s.issueTokens(ctx, &fetchedUser)
}

// ---- Login ------------------------------------------------------------------

// Login valida credenciais e emite tokens de acesso.
// Retorna (accessToken, plainRefreshToken, user, error).
func (s *Service) Login(ctx context.Context, emailAddr, password string) (string, string, *user.User, error) {
	u, err := s.users.GetByEmail(ctx, emailAddr)
	if err != nil {
		// Não revelar se o email existe — mesma mensagem para email inválido e senha errada.
		return "", "", nil, ErrInvalidCredentials
	}
	if !u.IsActive {
		return "", "", nil, ErrAccountInactive
	}
	if !u.IsVerified() {
		return "", "", nil, ErrEmailNotVerified
	}
	if u.PasswordHash == "" {
		// Conta criada via OAuth; sem senha configurada.
		return "", "", nil, ErrInvalidCredentials
	}
	if err := CheckPassword(u.PasswordHash, password); err != nil {
		return "", "", nil, ErrInvalidCredentials
	}
	return s.issueTokens(ctx, &u)
}

// ---- Google OAuth -----------------------------------------------------------

// GoogleCallback processa o retorno do Google OAuth.
// Upserta o usuário (cria ou vincula provider) e emite tokens.
func (s *Service) GoogleCallback(ctx context.Context, code, state string) (string, string, *user.User, error) {
	if err := s.oauth.ValidateState(state); err != nil {
		return "", "", nil, fmt.Errorf("state oauth inválido: %w", err)
	}
	profile, err := s.oauth.Exchange(ctx, code)
	if err != nil {
		return "", "", nil, err
	}

	// Tentar achar o usuário pelo provider.
	u, err := s.users.GetByOAuthProvider(ctx, "google", profile.Sub)
	if err != nil {
		// Usuário não existe pelo provider — verificar se existe pelo email.
		existing, emailErr := s.users.GetByEmail(ctx, profile.Email)
		if emailErr == nil {
			// Vincular provider ao usuário existente.
			_ = s.users.LinkOAuthProvider(ctx, existing.ID, "google", profile.Sub)
			u = existing
		} else {
			// Criar novo usuário.
			now := time.Now()
			newUser := &user.User{
				ID:              uuid.New(),
				Email:           profile.Email,
				DisplayName:     profile.Name,
				AvatarURL:       profile.Picture,
				PlatformRole:    user.RoleUser,
				EmailVerifiedAt: &now, // Google já verificou o email.
				IsActive:        true,
			}
			if err := s.users.Create(ctx, newUser); err != nil {
				return "", "", nil, fmt.Errorf("criar usuário google: %w", err)
			}
			_ = s.users.LinkOAuthProvider(ctx, newUser.ID, "google", profile.Sub)
			u = *newUser
		}
	}

	if !u.IsActive {
		return "", "", nil, ErrAccountInactive
	}
	return s.issueTokens(ctx, &u)
}

// ---- Forgot / Reset password ------------------------------------------------

// ForgotPassword envia email de reset. Não revela se o email está cadastrado.
func (s *Service) ForgotPassword(ctx context.Context, emailAddr string) error {
	u, err := s.users.GetByEmail(ctx, emailAddr)
	if err != nil {
		// Conta não encontrada — retorna sem erro (anti-enumeração).
		return nil
	}
	plain, hash, err := generateToken()
	if err != nil {
		return err
	}
	expiresAt := time.Now().Add(time.Hour)
	if err := s.tokens.CreatePasswordResetToken(ctx, u.ID, hash, expiresAt); err != nil {
		return fmt.Errorf("criar token de reset: %w", err)
	}
	resetURL := s.cfg.FrontendBaseURL + "/auth/reset-password?token=" + plain
	msg := email.PasswordResetEmail(u.Email, u.DisplayName, resetURL)
	return s.mailer.Send(ctx, msg)
}

// ResetPassword valida o token e atualiza a senha.
func (s *Service) ResetPassword(ctx context.Context, plainToken, newPassword string) error {
	hash := hashToken(plainToken)
	userID, err := s.tokens.UsePasswordResetToken(ctx, hash)
	if err != nil {
		return ErrTokenNotFound
	}
	newHash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	return s.users.UpdatePasswordHash(ctx, userID, newHash)
}

// ---- Token lifecycle --------------------------------------------------------

// RefreshAccess valida o refresh token e emite novo access token.
func (s *Service) RefreshAccess(ctx context.Context, plainRefreshToken string) (string, *user.User, error) {
	hash := hashToken(plainRefreshToken)
	rec, err := s.tokens.GetRefreshToken(ctx, hash)
	if err != nil {
		return "", nil, ErrTokenNotFound
	}
	if rec.RevokedAt != nil {
		return "", nil, ErrTokenNotFound
	}
	if time.Now().After(rec.ExpiresAt) {
		return "", nil, ErrTokenNotFound
	}
	u, err := s.users.GetByID(ctx, rec.UserID)
	if err != nil {
		return "", nil, fmt.Errorf("buscar usuário para refresh: %w", err)
	}
	if !u.IsActive {
		return "", nil, ErrAccountInactive
	}
	accessToken, err := s.tokenSvc.IssueAccessToken(&u)
	if err != nil {
		return "", nil, err
	}
	return accessToken, &u, nil
}

// Logout revoga o refresh token.
func (s *Service) Logout(ctx context.Context, plainRefreshToken string) error {
	hash := hashToken(plainRefreshToken)
	return s.tokens.RevokeRefreshToken(ctx, hash)
}

// OAuthService expõe o OAuthService para uso no handler (gerar URL de redirect).
func (s *Service) OAuth() *OAuthService {
	return s.oauth
}

// ---- helpers internos -------------------------------------------------------

// issueTokens gera access + refresh token e persiste o refresh no DB.
func (s *Service) issueTokens(ctx context.Context, u *user.User) (accessToken, plainRefresh string, _ *user.User, err error) {
	accessToken, err = s.tokenSvc.IssueAccessToken(u)
	if err != nil {
		return "", "", nil, err
	}
	plain, hash, err := GenerateRefreshToken()
	if err != nil {
		return "", "", nil, err
	}
	expiresAt := time.Now().Add(s.tokenSvc.RefreshTTL())
	if err := s.tokens.CreateRefreshToken(ctx, u.ID, hash, expiresAt); err != nil {
		return "", "", nil, fmt.Errorf("persistir refresh token: %w", err)
	}
	return accessToken, plain, u, nil
}

// generateToken gera 32 bytes aleatórios como hex e devolve (plain, sha256hash).
func generateToken() (plain, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", fmt.Errorf("gerar token: %w", err)
	}
	plain = hex.EncodeToString(b)
	hash = hashToken(plain)
	return plain, hash, nil
}
