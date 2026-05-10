package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/user"
)

var (
	ErrTokenExpired = errors.New("auth: token expirado")
	ErrTokenInvalid = errors.New("auth: token inválido")
)

type AccessClaims struct {
	jwt.RegisteredClaims
	Email        string           `json:"email"`
	PlatformRole user.PlatformRole `json:"platform_role"`
}

type TokenService struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func NewTokenService(secret string, accessTTL, refreshTTL time.Duration) *TokenService {
	return &TokenService{
		secret:     []byte(secret),
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
	}
}

func (s *TokenService) IssueAccessToken(u *user.User) (string, error) {
	now := time.Now()
	claims := AccessClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   u.ID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.accessTTL)),
		},
		Email:        u.Email,
		PlatformRole: u.PlatformRole,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("assinar jwt: %w", err)
	}
	return signed, nil
}

func (s *TokenService) ParseAccessToken(tokenStr string) (*AccessClaims, error) {
	tok, err := jwt.ParseWithClaims(tokenStr, &AccessClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("método de assinatura inesperado: %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrTokenInvalid
	}
	claims, ok := tok.Claims.(*AccessClaims)
	if !ok || !tok.Valid {
		return nil, ErrTokenInvalid
	}
	return claims, nil
}

// GenerateRefreshToken gera um token opaco de 32 bytes.
// Retorna (plainToken, sha256Hash, error).
// Só o hash é armazenado no banco; o plain é enviado ao cliente.
func GenerateRefreshToken() (plain string, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", fmt.Errorf("gerar refresh token: %w", err)
	}
	plain = hex.EncodeToString(b)
	hash = hashToken(plain)
	return plain, hash, nil
}

// RefreshTokenRecord é o registro de um refresh token armazenado no banco.
type RefreshTokenRecord struct {
	UserID    uuid.UUID
	ExpiresAt time.Time
	RevokedAt *time.Time
}

func (s *TokenService) AccessTTL() time.Duration {
	return s.accessTTL
}

func (s *TokenService) RefreshTTL() time.Duration {
	return s.refreshTTL
}

// hashToken computa o SHA-256 do token em hex.
func hashToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// UserIDFromClaims parseia o subject do JWT como uuid.UUID.
func UserIDFromClaims(c *AccessClaims) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Subject)
	if err != nil {
		return uuid.Nil, fmt.Errorf("subject inválido no jwt: %w", err)
	}
	return id, nil
}
