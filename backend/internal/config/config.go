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

	// Credenciais de fontes externas. Todas opcionais — se vazias, o scraper
	// correspondente devolve scraper.ErrNotConfigured e o handler segue o
	// fan-out sem essa fonte.
	TCGPlayerPublicKey  string
	TCGPlayerPrivateKey string
	EbayClientID        string
	EbayClientSecret    string
	PokemonTCGAPIKey    string // opcional, aumenta rate limit do importador.
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

	// Credenciais externas — todas opcionais.
	cfg.TCGPlayerPublicKey = os.Getenv("TCGPLAYER_PUBLIC_KEY")
	cfg.TCGPlayerPrivateKey = os.Getenv("TCGPLAYER_PRIVATE_KEY")
	cfg.EbayClientID = os.Getenv("EBAY_CLIENT_ID")
	cfg.EbayClientSecret = os.Getenv("EBAY_CLIENT_SECRET")
	cfg.PokemonTCGAPIKey = os.Getenv("POKEMON_TCG_API_KEY")

	return cfg, nil
}

func getOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
