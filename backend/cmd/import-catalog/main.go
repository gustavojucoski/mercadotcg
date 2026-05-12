// Command import-catalog populates card_sets + cards from the TCGDex API
// (https://api.tcgdex.net/v2), which is free, requires no API key, and covers
// both the main Pokémon TCG and TCG Pocket.
//
// Usage:
//
//	import-catalog                    # imports all sets + cards
//	import-catalog --set sv01         # imports a single specific set
//	import-catalog --series sv        # imports all sets in a series
//	import-catalog --recent 5         # imports only the 5 most recently released sets
//	import-catalog --download-images  # downloads card/set images to the storage provider
//
// Idempotent: existing sets and cards are updated (UPSERT), not duplicated.
//
// Environment variables:
//
//	DATABASE_URL           (required)
//	STORAGE_BACKEND        "local" (default) | "s3"
//	STORAGE_LOCAL_PATH     local directory (default: ./data/images)
//	STORAGE_LOCAL_BASE_URL public base URL; required with STORAGE_BACKEND=local
//	S3_BUCKET              bucket name; required with STORAGE_BACKEND=s3
//	S3_REGION              AWS region (default: us-east-1)
//	STORAGE_S3_CUSTOM_URL  optional CloudFront/custom URL for S3
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/card"
	"github.com/gustavojucoski/mercadotcg/backend/internal/repository/postgres"
	"github.com/gustavojucoski/mercadotcg/backend/internal/tcgdex"
	"github.com/gustavojucoski/mercadotcg/backend/internal/upload"
)

// imgJob describes a card image to download in the background worker pool.
type imgJob struct {
	cardID   uuid.UUID
	setCode  string
	localID  string // TCGDex localId, used as the filename base
	imageURL string // base URL (no extension); append "/high.webp" for the image
	tcg      string
}

func main() {
	setCode := flag.String("set", "", "import a specific set by ID (e.g. sv01)")
	seriesFilter := flag.String("series", "", "import all sets whose ID starts with this prefix (e.g. sv)")
	recent := flag.Int("recent", 0, "if > 0, import only the N sets with the highest IDs (heuristic for recent)")
	downloadImages := flag.Bool("download-images", false, "download card and set images to the storage provider")
	flag.Parse()

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	_ = godotenv.Load()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal().Msg("DATABASE_URL is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Minute)
	defer cancel()

	pool, err := postgres.Connect(ctx, databaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("connect postgres")
	}
	defer pool.Close()

	repo := postgres.NewCardRepo(pool)
	client := tcgdex.New(30 * time.Second)

	var uploadProvider upload.Provider
	if *downloadImages {
		uploadProvider, err = upload.NewFromEnv()
		if err != nil {
			log.Fatal().Err(err).Msg("create storage provider")
		}
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}

	log.Info().Msg("==> Fetching set list from TCGDex")

	allSets, err := client.ListSets(ctx, "en")
	if err != nil {
		log.Fatal().Err(err).Msg("fetch set list")
	}
	log.Info().Msgf("    %d sets available", len(allSets))

	// Apply filters: --set, --series, --recent.
	sets := allSets
	switch {
	case *setCode != "":
		sets = filterByID(sets, *setCode)
		if len(sets) == 0 {
			log.Fatal().Msgf("no set found with id=%q", *setCode)
		}
	case *seriesFilter != "":
		sets = filterBySeries(sets, *seriesFilter)
		if len(sets) == 0 {
			log.Fatal().Msgf("no sets found for series prefix=%q", *seriesFilter)
		}
	case *recent > 0:
		sets = mostRecent(sets, *recent)
	}

	// Background worker pool for card image downloads.
	// Buffered channel allows the main import loop to proceed without blocking.
	jobs := make(chan imgJob, 500)
	workerCtx, workerCancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	if *downloadImages {
		const numWorkers = 5
		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				imgHTTP := &http.Client{Timeout: 30 * time.Second}
				for job := range jobs {
					newURL, err := downloadCardImage(workerCtx, imgHTTP, uploadProvider, job.tcg, job.setCode, job.localID, job.imageURL)
					if err != nil {
						log.Warn().Err(err).Str("card_id", job.cardID.String()).Msg("download image failed")
						continue
					}
					if newURL != "" {
						// TCGDex provides a single image per card; use it for both small and large.
						if err := repo.UpdateCardImages(workerCtx, job.cardID, newURL, newURL); err != nil {
							log.Warn().Err(err).Str("card_id", job.cardID.String()).Msg("UpdateCardImages failed")
						}
					}
				}
			}()
		}
	}

	importedSets, updatedSets, totalCards := 0, 0, 0

	for i, summary := range sets {
		log.Info().Msgf("[%d/%d] %s — %s (%d cards)", i+1, len(sets), summary.ID, summary.Name, summary.CardCount.Total)

		// Fetch full EN set (with card list) + PT-BR enrichment in one call.
		bs, err := tcgdex.EnrichSet(ctx, client, summary.ID)
		if err != nil {
			log.Error().Err(err).Str("set", summary.ID).Msg("fetch set details — skipping")
			continue
		}
		if bs == nil {
			log.Warn().Str("set", summary.ID).Msg("set not found on TCGDex EN — skipping")
			continue
		}

		tcgName := detectTCG(bs.Serie.ID)

		// Upsert the series and capture its UUID.
		ser, err := repo.UpsertSeriesWithPT(ctx, bs.Serie.Name, bs.SerieNamePT, tcgName)
		if err != nil {
			log.Error().Err(err).Str("series", bs.Serie.Name).Msg("upsert series — skipping set")
			continue
		}

		releaseDate := parseISO8601(bs.ReleaseDate)

		// cardCount.official is the printed total (e.g. 198 in sv01).
		// When 0, store NULL via UpsertSet's NULLIF($10, 0).
		dbSet := card.Set{
			Code:         bs.ID,
			Name:         bs.Name,
			NamePT:       bs.NamePT,
			Series:       bs.Serie.Name,
			SeriesID:     &ser.ID,
			TCG:          tcgName,
			Language:     card.LanguageEnglish,
			ReleaseDate:  releaseDate,
			TotalCards:   bs.CardCount.Total,
			PrintedTotal: bs.CardCount.Official,
			ImageURL:     bs.Logo,
			SymbolURL:    bs.Symbol,
		}

		if err := repo.UpsertSet(ctx, &dbSet); err != nil {
			log.Error().Err(err).Str("set", bs.ID).Msg("upsert set — skipping cards")
			continue
		}

		// Distinguish new inserts from updates: pgx returns the same created_at on
		// a new insert, and the original created_at on an update. A sub-second
		// difference signals a new row.
		age := dbSet.UpdatedAt.Sub(dbSet.CreatedAt)
		if age < time.Second {
			importedSets++
		} else {
			updatedSets++
		}

		// Download set logo and symbol (inline — 1 per set, no worker pool).
		if *downloadImages {
			downloadSetImages(ctx, httpClient, uploadProvider, repo, dbSet, tcgName, bs.ID, bs.Logo, bs.Symbol)
		}

		// Import all cards for this set using the card refs already in bs.Cards.
		// This avoids a redundant GetSet call inside importCards.
		inserted, updated, skipped := importCards(ctx, client, repo, dbSet, tcgName, bs.Cards, jobs, *downloadImages)
		log.Info().Msgf("    cards: %d new, %d updated, %d skipped", inserted, updated, skipped)
		totalCards += inserted + updated
	}

	// Signal workers that no more jobs are coming, then wait for them to finish.
	close(jobs)
	wg.Wait()
	workerCancel()

	log.Info().Msgf("==> Done: %d sets new, %d sets updated, %d cards processed",
		importedSets, updatedSets, totalCards)
}

// ----------------------------------------------------------------------------
// Card import
// ----------------------------------------------------------------------------

// importCards imports all cards from a set. cardRefs are the CardRef entries already
// fetched from EnrichSet (avoids a redundant API call). Returns (inserted, updated, skipped).
func importCards(
	ctx context.Context,
	client *tcgdex.Client,
	repo *postgres.CardRepo,
	dbSet card.Set,
	tcgName string,
	cardRefs []tcgdex.CardRef,
	imgJobs chan<- imgJob,
	downloadImages bool,
) (inserted, updated, skipped int) {
	// Only fetch PT-BR card names for TCG Pocket sets.
	// TCGDex has PT-BR translations for Pocket cards but not for main TCG cards,
	// so we skip ~20k unnecessary requests for the main set corpus.
	fetchPTBR := tcgName == "pocket"

	for _, ref := range cardRefs {
		var namePT string
		var fullCard *tcgdex.Card

		if fetchPTBR {
			// For TCG Pocket we fetch the full PT-BR card (which also has variant data).
			ptCard, ferr := client.GetCard(ctx, "pt-br", ref.ID)
			if ferr != nil {
				log.Warn().Err(ferr).Str("card", ref.ID).Msg("pt-br card fetch failed")
			} else if ptCard != nil {
				namePT = ptCard.Name
			}
			// Also fetch EN full card for Pocket to get variant flags.
			enCard, ferr := client.GetCard(ctx, "en", ref.ID)
			if ferr != nil {
				log.Warn().Err(ferr).Str("card", ref.ID).Msg("en card fetch failed")
			} else {
				fullCard = enCard
			}
		}
		// For main TCG sets we skip per-card API calls entirely (saves ~20k requests).
		// Variants default to Normal+ReverseHolo — the standard finish for most TCG cards.
		// Run `import-catalog --set <code>` later to enrich a specific set's variants.

		dbCard := card.Card{
			SetID:           dbSet.ID,
			Number:          ref.LocalID,
			CollectorNumber: ref.LocalID,
			Name:            ref.Name,
			NamePT:          namePT,
			// TCGDex image URLs are base paths; append "/high.webp" for the full image.
			// We store the base URL here and resolve to a local path if --download-images.
			ImageSmallURL: imageURL(ref.Image),
			ImageLargeURL: imageURL(ref.Image),
		}

		wasNew, err := upsertCardWithVariants(ctx, repo, dbSet, dbCard, ref, fullCard)
		if err != nil {
			log.Error().Err(err).Str("card", ref.ID).Msg("upsert card+variants")
			skipped++
			continue
		}
		if wasNew {
			inserted++
		} else {
			updated++
		}

		if downloadImages && ref.Image != "" {
			imgJobs <- imgJob{
				cardID:   dbCard.ID,
				setCode:  dbSet.Code,
				localID:  ref.LocalID,
				imageURL: ref.Image,
				tcg:      tcgName,
			}
		}
	}
	return
}

// upsertCardWithVariants upserts the card row and creates its variant rows.
// Returns true if the card was newly inserted (i.e. created_at ≈ updated_at).
func upsertCardWithVariants(
	ctx context.Context,
	repo *postgres.CardRepo,
	_ card.Set,
	dbCard card.Card,
	ref tcgdex.CardRef,
	fullCard *tcgdex.Card,
) (isNew bool, err error) {
	if err := repo.UpsertCard(ctx, &dbCard); err != nil {
		return false, fmt.Errorf("upsert card: %w", err)
	}
	// Detect new row: on INSERT the RETURNING timestamps are equal (both set to now()).
	// On UPDATE, updated_at is refreshed to now() while created_at keeps its original value.
	isNew = dbCard.UpdatedAt.Sub(dbCard.CreatedAt) < time.Second

	// Determine variant finishes from the TCGDex Variants struct.
	var variants tcgdex.Variants
	if fullCard != nil {
		variants = fullCard.Variants
	} else {
		// Fallback: assume normal when we couldn't fetch the full card.
		variants = tcgdex.Variants{Normal: true}
	}

	finishes := variantsToFinishes(variants)
	for _, finish := range finishes {
		v := card.Variant{
			CardID: dbCard.ID,
			Finish: finish,
		}
		if err := repo.UpsertVariant(ctx, &v); err != nil {
			log.Warn().Err(err).
				Str("card", ref.ID).
				Str("finish", string(finish)).
				Msg("upsert variant failed")
		}
	}

	return isNew, nil
}

// variantsToFinishes converts TCGDex variant flags to variant_finish ENUM values.
// Always returns at least [FinishNormal] when no flags are set.
func variantsToFinishes(v tcgdex.Variants) []card.Finish {
	if !v.Normal && !v.Holo && !v.Reverse && !v.FirstEdition {
		return []card.Finish{card.FinishNormal}
	}
	var finishes []card.Finish
	if v.Normal {
		finishes = append(finishes, card.FinishNormal)
	}
	if v.Holo {
		finishes = append(finishes, card.FinishHolo)
	}
	if v.Reverse {
		finishes = append(finishes, card.FinishReverseHolo)
	}
	if v.FirstEdition {
		finishes = append(finishes, card.FinishFirstEdition)
	}
	return finishes
}

// ----------------------------------------------------------------------------
// Image download helpers
// ----------------------------------------------------------------------------

// downloadSetImages downloads and stores the logo and symbol images for a set.
func downloadSetImages(
	ctx context.Context,
	httpClient *http.Client,
	provider upload.Provider,
	repo *postgres.CardRepo,
	dbSet card.Set,
	tcg, setID, logoURL, symbolURL string,
) {
	if logoURL != "" {
		key := fmt.Sprintf("%s/sets/%s_logo.png", tcg, setID)
		if newURL, err := downloadAndStore(ctx, httpClient, provider, key, logoURL+".png"); err != nil {
			log.Warn().Err(err).Str("set", setID).Msg("download set logo failed")
		} else if newURL != "" {
			if err := repo.UpdateSetImageURL(ctx, dbSet.ID, newURL); err != nil {
				log.Warn().Err(err).Str("set", setID).Msg("UpdateSetImageURL failed")
			}
		}
	}
	if symbolURL != "" {
		key := fmt.Sprintf("%s/sets/%s_symbol.png", tcg, setID)
		if newURL, err := downloadAndStore(ctx, httpClient, provider, key, symbolURL+".png"); err != nil {
			log.Warn().Err(err).Str("set", setID).Msg("download set symbol failed")
		} else if newURL != "" {
			if err := repo.UpdateSetSymbolURL(ctx, dbSet.ID, newURL); err != nil {
				log.Warn().Err(err).Str("set", setID).Msg("UpdateSetSymbolURL failed")
			}
		}
	}
}

// downloadCardImage downloads a card image to the storage provider.
// TCGDex image base URLs need "/high.webp" appended for the full-size webp image.
func downloadCardImage(
	ctx context.Context,
	imgHTTP *http.Client,
	provider upload.Provider,
	tcg, setCode, localID, baseURL string,
) (string, error) {
	if baseURL == "" {
		return "", nil
	}
	key := fmt.Sprintf("%s/cards/%s/%s.webp", tcg, setCode, localID)
	return downloadAndStore(ctx, imgHTTP, provider, key, baseURL+"/high.webp")
}

// downloadAndStore downloads srcURL and stores it at key via provider.
// Returns the public URL. Skips (returns existing URL) if the file already exists.
func downloadAndStore(
	ctx context.Context,
	imgHTTP *http.Client,
	provider upload.Provider,
	key, srcURL string,
) (string, error) {
	if srcURL == "" {
		return "", nil
	}

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

	ext := strings.ToLower(filepath.Ext(path.Base(key)))
	contentType := "image/png"
	switch ext {
	case ".webp":
		contentType = "image/webp"
	case ".jpg", ".jpeg":
		contentType = "image/jpeg"
	}

	publicURL, err := provider.Put(ctx, key, io.Reader(resp.Body), contentType)
	if err != nil {
		return "", fmt.Errorf("put %s: %w", key, err)
	}
	return publicURL, nil
}

// ----------------------------------------------------------------------------
// TCG detection and utilities
// ----------------------------------------------------------------------------

// detectTCG returns the platform TCG identifier for a given TCGDex serie.id.
// TCG Pocket series use uppercase letter prefixes (A, B, C...).
// All other series are main Pokémon TCG ("pokemon").
func detectTCG(serieID string) string {
	if serieID == "" {
		return "pokemon"
	}
	first := rune(serieID[0])
	if first >= 'A' && first <= 'Z' {
		return "pocket"
	}
	return "pokemon"
}

// imageURL returns the TCGDex high-quality webp image URL for a card.
// TCGDex image fields are base paths; "/high.webp" gives the best quality.
func imageURL(base string) string {
	if base == "" {
		return ""
	}
	return base + "/high.webp"
}

// parseISO8601 parses a date string in "2006-01-02" or "2006/01/02" format.
func parseISO8601(s string) *time.Time {
	if s == "" {
		return nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return &t
	}
	if t, err := time.Parse("2006/01/02", s); err == nil {
		return &t
	}
	return nil
}

func filterByID(sets []tcgdex.SetSummary, id string) []tcgdex.SetSummary {
	var out []tcgdex.SetSummary
	for _, s := range sets {
		if strings.EqualFold(s.ID, id) {
			out = append(out, s)
		}
	}
	return out
}

func filterBySeries(sets []tcgdex.SetSummary, prefix string) []tcgdex.SetSummary {
	lp := strings.ToLower(prefix)
	var out []tcgdex.SetSummary
	for _, s := range sets {
		if strings.HasPrefix(strings.ToLower(s.ID), lp) {
			out = append(out, s)
		}
	}
	return out
}

// mostRecent returns the N sets with the lexicographically largest IDs.
// This is a heuristic since SetSummary does not include release dates.
func mostRecent(sets []tcgdex.SetSummary, n int) []tcgdex.SetSummary {
	sorted := make([]tcgdex.SetSummary, len(sets))
	copy(sorted, sets)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].ID > sorted[j].ID
	})
	if n > len(sorted) {
		n = len(sorted)
	}
	return sorted[:n]
}
