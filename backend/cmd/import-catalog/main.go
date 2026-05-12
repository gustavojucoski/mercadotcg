// Command import-catalog popula card_sets + cards a partir da Pokemon TCG API
// (https://pokemontcg.io/), que é grátis e cobre todas as cartas oficiais
// do TCG (~30k entries).
//
// Uso:
//
//	import-catalog                    # importa todos os sets + cards
//	import-catalog --set sv8          # importa só um set específico
//	import-catalog --recent 5         # importa só os 5 sets mais recentes
//	import-catalog --download-images  # baixa imagens para o storage provider
//
// Idempotente: cards/sets já existentes são pulados (UNIQUE conflict tratado).
//
// Variáveis de ambiente:
//
//	DATABASE_URL           (obrigatório)
//	POKEMON_TCG_API_KEY    (opcional) — aumenta rate limit de 1k/dia para 20k/dia
//	STORAGE_BACKEND        "local" (default) | "s3"
//	STORAGE_LOCAL_PATH     diretório local (default: ./data/images); usado com STORAGE_BACKEND=local
//	STORAGE_LOCAL_BASE_URL URL base pública (ex: http://localhost:8080/uploads); obrigatório com STORAGE_BACKEND=local
//	S3_BUCKET              nome do bucket (obrigatório com STORAGE_BACKEND=s3)
//	S3_REGION              região AWS (default: us-east-1); usado com STORAGE_BACKEND=s3
//	STORAGE_S3_CUSTOM_URL  URL customizada (CloudFront etc.); opcional com STORAGE_BACKEND=s3
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/card"
	"github.com/gustavojucoski/mercadotcg/backend/internal/pokemontcgio"
	"github.com/gustavojucoski/mercadotcg/backend/internal/repository/postgres"
	"github.com/gustavojucoski/mercadotcg/backend/internal/upload"
)

func main() {
	// Flags
	setCode := flag.String("set", "", "código de um set específico (ex.: sv8). Se vazio, importa todos.")
	recent := flag.Int("recent", 0, "se > 0, importa só os N sets mais recentes.")
	downloadImages := flag.Bool("download-images", false, "baixa imagens das cartas para o storage provider")
	flag.Parse()

	// Logger em console para o CLI.
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	// Configuração mínima — não usa config.Load() para evitar exigir JWT_SECRET.
	_ = godotenv.Load() // ignora erro se .env não existir
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal().Msg("DATABASE_URL é obrigatório")
	}
	apiKey := os.Getenv("POKEMON_TCG_API_KEY") // opcional

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()

	pool, err := postgres.Connect(ctx, databaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("connect postgres")
	}
	defer pool.Close()

	repo := postgres.NewCardRepo(pool)
	client := pokemontcgio.New(30*time.Second, apiKey)

	// Regras de variante — best-effort: usa vazio se não encontrar o arquivo.
	rules, err := parseRules("variant_rules.json")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Warn().Msg("variant_rules.json não encontrado — usando apenas finishes padrão")
			rules = RuleMap{}
		} else {
			log.Fatal().Err(err).Msg("parseRules")
		}
	}

	// Provider de upload (somente se --download-images).
	var uploadProvider upload.Provider
	if *downloadImages {
		uploadProvider, err = upload.NewFromEnv()
		if err != nil {
			log.Fatal().Err(err).Msg("criar storage provider")
		}
	}

	// httpClient compartilhado entre o loop principal (logos de set) e os workers.
	httpClient := &http.Client{Timeout: 30 * time.Second}

	log.Info().Msg("==> Importando catálogo Pokemon TCG")

	sets, err := client.ListSets(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("fetch sets")
	}
	log.Info().Msgf("    %d sets encontrados na API", len(sets))

	// Filtros: --set ou --recent.
	if *setCode != "" {
		sets = filterByCode(sets, *setCode)
		if len(sets) == 0 {
			log.Fatal().Msgf("nenhum set encontrado com code=%q", *setCode)
		}
	} else if *recent > 0 {
		sets = mostRecent(sets, *recent)
	}

	// Canal para jobs de download de imagem (buffer = 200 para não bloquear o loop principal).
	type imgJob struct {
		cardID          uuid.UUID
		setID           string // code do set na API (ex: "sv8pt5")
		collectorNumber string // collector_number formatado (ex: "1/198", "SWSH001")
		smallURL        string
		largeURL        string
	}
	imgJobs := make(chan imgJob, 200)

	// workerCtx é independente do ctx principal (que tem deadline de 60min para
	// importação de sets). Downloads de imagem podem durar mais — e não devem ser
	// cancelados só porque o loop de sets chegou perto do timeout.
	workerCtx, workerCancel := context.WithCancel(context.Background())

	// Worker pool de imagens: só inicia se --download-images.
	var wg sync.WaitGroup
	if *downloadImages {
		const numWorkers = 10
		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				imgHTTP := &http.Client{Timeout: 30 * time.Second}
				for job := range imgJobs {
					newSmall, newLarge, err := downloadAndStore(workerCtx, imgHTTP, uploadProvider, "pokemon", job.setID, job.collectorNumber, job.smallURL, job.largeURL)
					if err != nil {
						log.Warn().Err(err).Str("card_id", job.cardID.String()).Msg("download imagem falhou")
						continue
					}
					if err := repo.UpdateCardImages(workerCtx, job.cardID, newSmall, newLarge); err != nil {
						log.Warn().Err(err).Str("card_id", job.cardID.String()).Msg("UpdateCardImages falhou")
					}
				}
			}()
		}
	}

	imported, skipped, cardsTotal := 0, 0, 0
	for i, s := range sets {
		log.Info().Msgf("[%d/%d] %s — %s (%d cards)", i+1, len(sets), s.ID, s.Name, s.Total)

		dbSet, setErr := upsertSet(ctx, repo, s)
		if setErr != nil {
			if errors.Is(setErr, postgres.ErrAlreadyExists) {
				skipped++
				// Mesmo que o set já exista, continua para importar cards novos.
			} else {
				log.Error().Err(setErr).Str("set", s.ID).Msg("erro no set — pulando cards")
				continue
			}
		} else {
			imported++
		}

		// Download inline do logo do set (1 arquivo por set, não justifica worker pool).
		if *downloadImages && dbSet.ImageURL != "" {
			ext := filepath.Ext(path.Base(dbSet.ImageURL))
			if ext == "" {
				ext = ".png"
			}
			key := fmt.Sprintf("pokemon/sets/%s%s", s.ID, ext)
			exists, err := uploadProvider.Exists(ctx, key)
			if err != nil {
				log.Warn().Err(err).Str("set", s.ID).Msg("exists check logo falhou")
			} else if !exists {
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, dbSet.ImageURL, nil)
				if err == nil {
					resp, err := httpClient.Do(req)
					if err == nil && resp.StatusCode == http.StatusOK {
						publicURL, err := uploadProvider.Put(ctx, key, resp.Body, "image/png")
						resp.Body.Close()
						if err != nil {
							log.Warn().Err(err).Str("set", s.ID).Msg("upload logo falhou")
						} else {
							if err := repo.UpdateSetImageURL(ctx, dbSet.ID, publicURL); err != nil {
								log.Warn().Err(err).Str("set", s.ID).Msg("UpdateSetImageURL falhou")
							}
						}
					}
				}
			}
		}

		cards, err := client.ListCardsBySet(ctx, s.ID)
		if err != nil {
			log.Error().Err(err).Str("set", s.ID).Msg("erro nos cards")
			continue
		}

		var inserted, conflicts int
		for _, c := range cards {
			cardID, err := upsertCard(ctx, repo, dbSet, s, c, rules)
			if err != nil {
				if errors.Is(err, postgres.ErrAlreadyExists) {
					conflicts++
					continue
				}
				log.Error().Err(err).Str("number", c.Number).Msg("erro na carta")
				continue
			}
			inserted++

			if *downloadImages && cardID != (uuid.UUID{}) {
				imgJobs <- imgJob{
					cardID:          cardID,
					setID:           s.ID,
					collectorNumber: buildCollectorNumber(c.Number, s.PrintedTotal),
					smallURL:        c.SmallURL,
					largeURL:        c.LargeURL,
				}
			}
		}
		log.Info().Msgf("    %d cards novos, %d já existiam", inserted, conflicts)
		cardsTotal += inserted

		// Pausa conservadora para respeitar o rate limit da API.
		time.Sleep(200 * time.Millisecond)
	}

	// Sinaliza workers de imagem que não há mais jobs e aguarda conclusão.
	close(imgJobs)
	wg.Wait()
	workerCancel() // libera o contexto dos workers após todos terminarem

	log.Info().Msgf("==> Concluído: %d sets novos, %d sets já existiam, %d cards novos",
		imported, skipped, cardsTotal)
}

// ----------------------------------------------------------------------------
// Persistência
// ----------------------------------------------------------------------------

// upsertSet garante que o set existe no banco. Retorna o set (existente ou novo)
// e ErrAlreadyExists se já existia (para contabilização), ou nil se foi criado.
func upsertSet(ctx context.Context, repo *postgres.CardRepo, s pokemontcgio.SetInfo) (card.Set, error) {
	// Upsert da série primeiro, para ter o series_id disponível.
	var seriesID *uuid.UUID
	if s.Series != "" {
		tcg := "pokemon"
		ser, err := repo.UpsertSeries(ctx, s.Series, tcg)
		if err != nil {
			return card.Set{}, fmt.Errorf("upsert series: %w", err)
		}
		seriesID = &ser.ID
	}

	existing, err := repo.GetSetByCode(ctx, s.ID)
	if err == nil {
		// Set já existe — retorna com ErrAlreadyExists para o caller contabilizar.
		return existing, postgres.ErrAlreadyExists
	}
	if !errors.Is(err, postgres.ErrNotFound) {
		return card.Set{}, fmt.Errorf("get set by code: %w", err)
	}

	releaseDate := parseAPIDate(s.ReleaseDate)
	dbSet := card.Set{
		Code:        s.ID,
		Name:        s.Name,
		Series:      s.Series,
		SeriesID:    seriesID,
		TCG:         "pokemon",
		Language:    card.LanguageEnglish,
		ReleaseDate: releaseDate,
		TotalCards:  s.Total,
		ImageURL:    s.LogoURL,
	}
	if err := repo.CreateSet(ctx, &dbSet); err != nil {
		// Race condition: outro processo inseriu o set entre o GetSetByCode e o CreateSet.
		// Busca o existente para devolver um set com ID válido.
		if errors.Is(err, postgres.ErrAlreadyExists) {
			existing, lookupErr := repo.GetSetByCode(ctx, s.ID)
			if lookupErr != nil {
				return card.Set{}, fmt.Errorf("upsertSet get existing after race: %w", lookupErr)
			}
			return existing, postgres.ErrAlreadyExists
		}
		return card.Set{}, fmt.Errorf("create set: %w", err)
	}
	return dbSet, nil
}

// buildCollectorNumber devolve o número de colecionador sem o total do set.
// O display "274/217" é construído no frontend combinando collector_number + "/" + set.total_cards;
// armazenar o total aqui geraria slugs com "/" que quebram URLs.
func buildCollectorNumber(number string, _ int) string {
	return number
}

// upsertCard cria uma carta e suas variantes de acordo com as regras.
// Retorna o ID interno da carta (uuid.UUID) para uso no download de imagens.
// Retorna ErrAlreadyExists se a carta já existia.
func upsertCard(
	ctx context.Context,
	repo *postgres.CardRepo,
	dbSet card.Set,
	s pokemontcgio.SetInfo,
	c pokemontcgio.CatalogCard,
	rules RuleMap,
) (cardID uuid.UUID, err error) {
	isPromo := strings.EqualFold(c.Rarity, "Promo")

	dbCard := card.Card{
		SetID:           dbSet.ID,
		Number:          c.Number,
		CollectorNumber: buildCollectorNumber(c.Number, s.PrintedTotal),
		Name:            c.Name,
		Rarity:          c.Rarity,
		Supertype:       c.Supertype,
		Subtypes:        c.Subtypes,
		Types:           c.Types,
		HP:              atoiOrZero(c.HP),
		Illustrator:     c.Artist,
		ImageSmallURL:   c.SmallURL,
		ImageLargeURL:   c.LargeURL,
		ExternalIDs: map[string]string{
			"pokemon_tcg_io": c.ID,
		},
	}
	if err := repo.CreateCard(ctx, &dbCard); err != nil {
		return uuid.UUID{}, err
	}

	// Resolve os finishes usando as regras.
	finishes := resolveFinishes("pokemon", s.Series, c.Rarity, rules)

	// Verifica se a carta tem número acima do printedTotal sem variante especial —
	// pode indicar que a regra está faltando.
	if s.PrintedTotal > 0 {
		cardNum := atoiOrZero(c.Number)
		if cardNum > s.PrintedTotal {
			hasSpecial := false
			for _, f := range finishes {
				if f == "master_ball_mirror" || f == "poke_ball_mirror" {
					hasSpecial = true
					break
				}
			}
			if !hasSpecial {
				log.Warn().Msgf("[WARN] carta %s: number=%s > printedTotal=%d mas nenhuma variante especial foi criada (rarity=%q)",
					c.ID, c.Number, s.PrintedTotal, c.Rarity)
			}
		}
	}

	// Cria uma variante para cada finish resolvido.
	for _, finishStr := range finishes {
		v := card.Variant{
			CardID:  dbCard.ID,
			Finish:  card.Finish(finishStr),
			IsPromo: isPromo,
		}
		if varErr := repo.CreateVariant(ctx, &v); varErr != nil && !errors.Is(varErr, postgres.ErrAlreadyExists) {
			// Variante duplicada é aceitável em re-runs — erros reais logamos mas não abortamos.
			log.Warn().Err(varErr).
				Str("card", c.ID).
				Str("finish", finishStr).
				Msg("criar variante falhou")
		}
	}

	return dbCard.ID, nil
}

// ----------------------------------------------------------------------------
// Download de imagens
// ----------------------------------------------------------------------------

// downloadAndStore baixa smallURL e largeURL, salva no provider e retorna as
// novas URLs públicas.
// Chave gerada: {tcg}/cards/{setCode}/{collectorNumberSafe}_{size}.{ext}
func downloadAndStore(
	ctx context.Context,
	imgHTTP *http.Client,
	provider upload.Provider,
	tcg, setCode, collectorNumber, smallURL, largeURL string,
) (newSmall, newLarge string, err error) {
	collSafe := strings.ReplaceAll(collectorNumber, "/", "_")
	newSmall, err = downloadOne(ctx, imgHTTP, provider, tcg, setCode, collSafe+"_small", smallURL)
	if err != nil {
		return "", "", fmt.Errorf("small: %w", err)
	}
	newLarge, err = downloadOne(ctx, imgHTTP, provider, tcg, setCode, collSafe+"_large", largeURL)
	if err != nil {
		return "", "", fmt.Errorf("large: %w", err)
	}
	return newSmall, newLarge, nil
}

// downloadOne baixa uma imagem e a armazena via provider.
// A chave no provider é "{tcg}/cards/{setCode}/{filename}.{ext}".
// Se o arquivo já existir no provider, pula o download e retorna a URL existente
// (idempotência em re-runs com --download-images).
func downloadOne(
	ctx context.Context,
	imgHTTP *http.Client,
	provider upload.Provider,
	tcg, setCode, filename, srcURL string,
) (string, error) {
	if srcURL == "" {
		return "", nil
	}

	ext := filepath.Ext(path.Base(srcURL))
	if ext == "" {
		ext = ".png"
	}
	key := fmt.Sprintf("%s/cards/%s/%s%s", tcg, setCode, filename, ext)

	exists, err := provider.Exists(ctx, key)
	if err != nil {
		return "", fmt.Errorf("exists check %s: %w", key, err)
	}
	if exists {
		return provider.PublicURL(key), nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srcURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := imgHTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: status %d", srcURL, resp.StatusCode)
	}

	// Detecta content type pela extensão — mais confiável que o header da origem.
	contentType := "image/png"
	if ext == ".jpg" || ext == ".jpeg" {
		contentType = "image/jpeg"
	}

	publicURL, err := provider.Put(ctx, key, io.Reader(resp.Body), contentType)
	if err != nil {
		return "", fmt.Errorf("put %s: %w", key, err)
	}
	return publicURL, nil
}

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

func filterByCode(sets []pokemontcgio.SetInfo, code string) []pokemontcgio.SetInfo {
	var out []pokemontcgio.SetInfo
	for _, s := range sets {
		if s.ID == code {
			out = append(out, s)
		}
	}
	return out
}

func mostRecent(sets []pokemontcgio.SetInfo, n int) []pokemontcgio.SetInfo {
	sort.SliceStable(sets, func(i, j int) bool {
		return sets[i].ReleaseDate > sets[j].ReleaseDate
	})
	if n > len(sets) {
		n = len(sets)
	}
	return sets[:n]
}

func parseAPIDate(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse("2006/01/02", s)
	if err != nil {
		return nil
	}
	return &t
}

func atoiOrZero(s string) int {
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}
