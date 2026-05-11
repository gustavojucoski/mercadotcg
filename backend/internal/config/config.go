// Package config carrega configuração via env vars. Tipado e validado no boot.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config concentra toda configuração da aplicação. Campos são preenchidos
// uma vez no boot e tratados como imutáveis depois.
type Config struct {
	Env             string
	HTTPPort        int
	DatabaseURL     string
	ShutdownTimeout time.Duration
	LogLevel        string

	// Auth — JWT e Google OAuth.
	JWTSecret          string        // obrigatório
	JWTAccessTTL       time.Duration // padrão 15min
	JWTRefreshTTL      time.Duration // padrão 30 dias
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string
	OAuthStateHMACKey  string // chave hex para HMAC do state param
	FrontendBaseURL    string // ex: "http://localhost:3000"

	// Email — Resend.
	ResendAPIKey     string
	EmailFromAddress string // ex: "noreply@mercadotcg.com.br"

	// Upload de arquivos.
	UploadsDir     string // diretório local de uploads (dev)
	UploadsBaseURL string // URL pública base, ex: "http://localhost:8080/uploads"

	// Credenciais de fontes externas. Todas opcionais — se vazias, o scraper
	// correspondente devolve scraper.ErrNotConfigured e o handler segue o
	// fan-out sem essa fonte.
	TCGPlayerPublicKey  string
	TCGPlayerPrivateKey string
	EbayClientID        string
	EbayClientSecret    string
	PokemonTCGAPIKey    string // opcional, aumenta rate limit do importador.
	PokeWalletAPIKey    string // pokewallet.io — TCGPlayer + Cardmarket via API.
	FlareSolverrURL     string // ex: "http://localhost:8191". Vazio = sem bypass.
}

// Load lê variáveis de ambiente (e .env se presente) e devolve uma Config
// validada. Falha rápido se algo obrigatório estiver faltando.
func Load() (Config, error) {
	_ = godotenv.Load() // .env é opcional em produção

	cfg := Config{
		Env:             getOr("APP_ENV", "development"),
		LogLevel:        getOr("LOG_LEVEL", "info"),
		ShutdownTimeout: 15 * time.Second,
	}

	port, err := strconv.Atoi(getOr("HTTP_PORT", "8080"))
	if err != nil {
		return Config{}, fmt.Errorf("HTTP_PORT inválido: %w", err)
	}
	cfg.HTTPPort = port

	cfg.DatabaseURL = os.Getenv("DATABASE_URL")
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL é obrigatória")
	}

	// Auth.
	cfg.JWTSecret = os.Getenv("JWT_SECRET")
	if cfg.JWTSecret == "" {
		return Config{}, fmt.Errorf("JWT_SECRET é obrigatório")
	}
	cfg.JWTAccessTTL = parseDurationOr("JWT_ACCESS_TTL", 15*time.Minute)
	cfg.JWTRefreshTTL = parseDurationOr("JWT_REFRESH_TTL", 30*24*time.Hour)
	cfg.GoogleClientID = os.Getenv("GOOGLE_CLIENT_ID")
	cfg.GoogleClientSecret = os.Getenv("GOOGLE_CLIENT_SECRET")
	cfg.GoogleRedirectURL = getOr("GOOGLE_REDIRECT_URL", "http://localhost:8080/api/v1/auth/google/callback")
	cfg.OAuthStateHMACKey = os.Getenv("OAUTH_STATE_HMAC_KEY")
	cfg.FrontendBaseURL = getOr("FRONTEND_BASE_URL", "http://localhost:3000")

	// Email.
	cfg.ResendAPIKey = os.Getenv("RESEND_API_KEY")
	cfg.EmailFromAddress = getOr("EMAIL_FROM_ADDRESS", "noreply@mercadotcg.com.br")

	// Uploads.
	cfg.UploadsDir = getOr("UPLOADS_DIR", "./uploads")
	cfg.UploadsBaseURL = getOr("UPLOADS_BASE_URL", "http://localhost:8080/uploads")

	// Credenciais externas — todas opcionais.
	cfg.TCGPlayerPublicKey = os.Getenv("TCGPLAYER_PUBLIC_KEY")
	cfg.TCGPlayerPrivateKey = os.Getenv("TCGPLAYER_PRIVATE_KEY")
	cfg.EbayClientID = os.Getenv("EBAY_CLIENT_ID")
	cfg.EbayClientSecret = os.Getenv("EBAY_CLIENT_SECRET")
	cfg.PokemonTCGAPIKey = os.Getenv("POKEMON_TCG_API_KEY")
	cfg.PokeWalletAPIKey = os.Getenv("POKEWALLET_API_KEY")
	cfg.FlareSolverrURL = os.Getenv("FLARESOLVERR_URL")

	return cfg, nil
}

func getOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseDurationOr(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
