// Command translate-set-names translates non-EN set names using the DeepL API
// and stores the result in card_sets.name_en.
//
// It targets sets whose language is 'ja', 'ko', or 'zh-tw' and whose name_en
// column is NULL or empty. Re-running is idempotent: already-translated sets
// are skipped unless --force is passed.
//
// Usage:
//
//	translate-set-names                  # translate all untranslated non-EN sets
//	translate-set-names --lang ja        # translate only Japanese sets
//	translate-set-names --force          # re-translate even sets with name_en set
//	translate-set-names --dry-run        # print translations without saving
//	translate-set-names --rate 1.0       # requests per second (default 0.5)
//
// Environment variables:
//
//	DATABASE_URL    (required)
//	DEEPL_API_KEY   (required) — free tier key ends with ":fx"
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	_ = godotenv.Load()

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "15:04:05"})

	langFlag := flag.String("lang", "", "filter by language: ja, ko, zh-tw (default: all non-EN)")
	force := flag.Bool("force", false, "re-translate sets that already have name_en")
	dryRun := flag.Bool("dry-run", false, "print translations without saving to DB")
	ratePerSec := flag.Float64("rate", 0.5, "max requests per second sent to DeepL (default 0.5 = 1 req/2s)")
	flag.Parse()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal().Msg("DATABASE_URL é obrigatório")
	}
	deeplKey := os.Getenv("DEEPL_API_KEY")
	if deeplKey == "" {
		log.Fatal().Msg("DEEPL_API_KEY é obrigatório")
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatal().Err(err).Msg("conectar ao banco")
	}
	defer pool.Close()

	sets, err := fetchSetsToTranslate(ctx, pool, *langFlag, *force)
	if err != nil {
		log.Fatal().Err(err).Msg("buscar sets")
	}
	log.Info().Int("total", len(sets)).Msg("sets a traduzir")

	if len(sets) == 0 {
		log.Info().Msg("nenhum set para traduzir")
		return
	}

	// Enforce a hard rate ceiling via ticker. Default 0.5 req/s = 1 req every 2s,
	// well within DeepL free tier limits. Increase only if needed.
	interval := time.Duration(float64(time.Second) / *ratePerSec)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Info().Dur("interval", interval).Msg("rate limiter configurado")

	client := &deeplClient{
		apiKey:     deeplKey,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}

	translated := 0
	for i, s := range sets {
		// Block until the rate limiter allows the next request.
		<-ticker.C

		translation, err := translateWithBackoff(ctx, client, s.name)
		if err != nil {
			log.Error().Err(err).Str("set", s.code).Str("name", s.name).Msg("tradução falhou — pulando")
			continue
		}

		log.Info().
			Str("code", s.code).
			Str("lang", s.lang).
			Str("original", s.name).
			Str("translated", translation).
			Msgf("[%d/%d]", i+1, len(sets))

		if *dryRun {
			continue
		}

		if err := updateSetNameEN(ctx, pool, s.id, translation); err != nil {
			log.Error().Err(err).Str("set", s.code).Msg("salvar tradução")
			continue
		}
		translated++
	}

	if !*dryRun {
		log.Info().Int("saved", translated).Msg("concluído")
	} else {
		log.Info().Msg("dry-run: nenhuma alteração salva")
	}
}

// translateWithBackoff retries on 429 with exponential backoff.
// Waits are 10s, 20s, 40s, 80s before the fifth (final) attempt.
func translateWithBackoff(ctx context.Context, client *deeplClient, text string) (string, error) {
	backoff := 10 * time.Second
	for attempt := 1; attempt <= 5; attempt++ {
		translation, err := client.translate(ctx, text, "EN")
		if err == nil {
			return translation, nil
		}
		if attempt == 5 {
			return "", err
		}
		log.Warn().
			Err(err).
			Int("attempt", attempt).
			Dur("backoff", backoff).
			Msg("rate limited — aguardando")
		time.Sleep(backoff)
		backoff *= 2
	}
	return "", fmt.Errorf("esgotou tentativas")
}

// ---------------------------------------------------------------------------
// DB helpers
// ---------------------------------------------------------------------------

type setRow struct {
	id   uuid.UUID
	code string
	name string
	lang string
}

const fetchSetsSQL = `
SELECT id, code, name, language
FROM card_sets
WHERE language = ANY($1)
  AND ($2 OR name_en IS NULL OR name_en = '')
ORDER BY language, code`

func fetchSetsToTranslate(ctx context.Context, pool *pgxpool.Pool, lang string, force bool) ([]setRow, error) {
	langs := []string{"ja", "ko", "zh-tw"}
	if lang != "" {
		langs = []string{lang}
	}

	rows, err := pool.Query(ctx, fetchSetsSQL, langs, force)
	if err != nil {
		return nil, fmt.Errorf("query sets: %w", err)
	}
	defer rows.Close()

	var out []setRow
	for rows.Next() {
		var s setRow
		if err := rows.Scan(&s.id, &s.code, &s.name, &s.lang); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

const updateNameENSQL = `
UPDATE card_sets SET name_en = $2, updated_at = now() WHERE id = $1`

func updateSetNameEN(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, nameEN string) error {
	_, err := pool.Exec(ctx, updateNameENSQL, id, nameEN)
	return err
}

// ---------------------------------------------------------------------------
// DeepL client
// ---------------------------------------------------------------------------

type deeplClient struct {
	apiKey     string
	httpClient *http.Client
}

type deeplRequest struct {
	Text       []string `json:"text"`
	TargetLang string   `json:"target_lang"`
}

type deeplResponse struct {
	Translations []struct {
		DetectedSourceLanguage string `json:"detected_source_language"`
		Text                   string `json:"text"`
	} `json:"translations"`
}

func (c *deeplClient) translate(ctx context.Context, text, targetLang string) (string, error) {
	body, _ := json.Marshal(deeplRequest{Text: []string{text}, TargetLang: targetLang})

	// Free tier key ends with ":fx" → use api-free.deepl.com
	host := "https://api.deepl.com"
	if strings.HasSuffix(c.apiKey, ":fx") {
		host = "https://api-free.deepl.com"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, host+"/v2/translate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "DeepL-Auth-Key "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("deepl HTTP %d: %s", resp.StatusCode, b)
	}

	var result deeplResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decodificar resposta DeepL: %w", err)
	}
	if len(result.Translations) == 0 {
		return "", fmt.Errorf("resposta DeepL sem traduções")
	}
	return result.Translations[0].Text, nil
}
