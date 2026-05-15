// Command import-catalog populates card_sets, cards, and card_variants from the
// Scrydex API (https://api.scrydex.io), which provides rich marketplace variant
// data (TCGPlayer, Cardmarket) alongside the standard catalog metadata.
//
// Usage:
//
//	import-catalog                         # imports all expansions + cards
//	import-catalog --set sv8               # imports a single specific expansion
//	import-catalog --series "Scarlet & Violet"  # imports all sets of a series
//	import-catalog --lang ja               # imports only Japanese expansions
//	import-catalog --skip-images           # imports metadata only, no S3 uploads
//	import-catalog --dry-run               # logs what would happen without persisting
//	import-catalog --enrich-pt             # fills name_pt + image_url_pt via TCGDex
//	import-catalog --set sv8 --enrich-pt   # import then enrich in one run
//
// Idempotent: existing rows are updated via UPSERT; sets with import_source =
// 'manual' are never overwritten by this importer.
//
// Environment variables:
//
//	DATABASE_URL            (required)
//	SCRYDEX_API_KEY         (required)
//	SCRYDEX_TEAM            (required)
//	SCRYDEX_BASE_URL        optional, defaults to https://api.scrydex.io
//	SCRYDEX_RATE_LIMIT      optional float, defaults to 2.0 req/s
//	STORAGE_BACKEND         "local" (default) | "s3"
//	STORAGE_LOCAL_PATH      local directory (default: ./data/images)
//	STORAGE_LOCAL_BASE_URL  public base URL; required with STORAGE_BACKEND=local
//	S3_BUCKET               bucket name; required with STORAGE_BACKEND=s3
//	S3_REGION               AWS region (default: us-east-1)
//	STORAGE_S3_CUSTOM_URL   optional CloudFront/custom URL for S3
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/gustavojucoski/mercadotcg/backend/internal/catalog/scrydex"
	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/card"
	"github.com/gustavojucoski/mercadotcg/backend/internal/repository/postgres"
	"github.com/gustavojucoski/mercadotcg/backend/internal/tcgdex"
	"github.com/gustavojucoski/mercadotcg/backend/internal/upload"
)

// imgJob describes a card image download to be processed by the worker pool.
type imgJob struct {
	cardID    uuid.UUID
	setCode   string
	scrydexID string // Scrydex card ID (e.g. "sv8-199") used as filename base
	imageURL  string // full URL for the large image
}

// counters tracks import progress for the final summary log.
type counters struct {
	mu             sync.Mutex
	setsImported   int
	setsUpdated    int
	setsSkipped    int
	cardsImported  int
	cardsUpdated   int
	variantsUpsert int
	imagesDownload int
	errors         int
}

func (c *counters) incSet(isNew bool) {
	c.mu.Lock()
	if isNew {
		c.setsImported++
	} else {
		c.setsUpdated++
	}
	c.mu.Unlock()
}

func (c *counters) incCard(isNew bool) {
	c.mu.Lock()
	if isNew {
		c.cardsImported++
	} else {
		c.cardsUpdated++
	}
	c.mu.Unlock()
}

func main() {
	setFilter := flag.String("set", "", "import only the expansion with this ID (e.g. sv8)")
	seriesFilter := flag.String("series", "", "import only expansions whose series name matches this string")
	langFilter := flag.String("lang", "all", "filter by language: en, ja, ko, or all (default)")
	skipImages := flag.Bool("skip-images", false, "skip all image downloads; only upsert metadata")
	dryRun := flag.Bool("dry-run", false, "log what would happen without writing to the DB or S3")
	enrichPT := flag.Bool("enrich-pt", false, "fill name_pt and image_url_pt for scrydex cards via TCGDex")
	enrichLimit := flag.Int("enrich-limit", 500, "max cards to enrich per run (avoid hammering TCGDex)")
	flag.Parse()

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	if *langFilter != "all" && *langFilter != "en" && *langFilter != "ja" && *langFilter != "ko" {
		log.Fatal().Str("lang", *langFilter).Msg("invalid --lang value; use: en, ja, ko, all")
	}

	_ = godotenv.Load()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal().Msg("DATABASE_URL is required")
	}

	// 120 min for import; enrichment at 1 req/s can take longer for large batches.
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Minute)
	defer cancel()

	// runScrydex is true when the caller wants to import from Scrydex (either a
	// specific set/series was requested, or --enrich-pt was NOT the sole flag).
	// Passing --enrich-pt alone skips the full Scrydex import.
	runScrydex := *setFilter != "" || *seriesFilter != "" || !*enrichPT

	var scrydexClient *scrydex.Client
	if runScrydex {
		var err error
		scrydexClient, err = scrydex.NewFromEnv()
		if err != nil {
			log.Fatal().Err(err).Msg("create scrydex client")
		}
		defer scrydexClient.Close()
	}

	var (
		repo     *postgres.CardRepo
		provider upload.Provider
	)

	if !*dryRun {
		pool, err := postgres.Connect(ctx, databaseURL)
		if err != nil {
			log.Fatal().Err(err).Msg("connect postgres")
		}
		defer pool.Close()
		repo = postgres.NewCardRepo(pool)

		if !*skipImages {
			provider, err = upload.NewFromEnv()
			if err != nil {
				log.Fatal().Err(err).Msg("create storage provider")
			}
		}
	}

	log.Info().
		Str("set", *setFilter).
		Str("series", *seriesFilter).
		Str("lang", *langFilter).
		Bool("dry_run", *dryRun).
		Bool("skip_images", *skipImages).
		Bool("enrich_pt", *enrichPT).
		Int("enrich_limit", *enrichLimit).
		Msg("==> Starting import-catalog")

	cnt := &counters{}

	if runScrydex {
		expansions, err := scrydexClient.ListExpansions(ctx)
		if err != nil {
			log.Fatal().Err(err).Msg("list expansions")
		}
		log.Info().Msgf("    %d expansions available from Scrydex", len(expansions))

		expansions = applyFilters(expansions, *setFilter, *seriesFilter, *langFilter)
		log.Info().Msgf("    %d expansions after filters", len(expansions))

		if len(expansions) == 0 {
			log.Warn().Msg("no expansions matched filters — nothing to import")
		} else {
			// Background worker pool for image downloads. 3 workers share the same
			// Scrydex client rate limiter implicitly via separate HTTP clients — the
			// rate limiter governs the Scrydex API calls, not the image CDN calls.
			jobs := make(chan imgJob, 200)
			workerCtx, workerCancel := context.WithCancel(ctx)
			defer workerCancel()

			var wg sync.WaitGroup

			if !*dryRun && !*skipImages {
				const numWorkers = 3
				imgHTTP := &http.Client{Timeout: 30 * time.Second}
				for i := 0; i < numWorkers; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						for job := range jobs {
							key := fmt.Sprintf("pokemon/cards/%s/%s.webp", job.setCode, job.scrydexID)
							newURL, err := downloadAndStore(workerCtx, imgHTTP, provider, key, job.imageURL)
							if err != nil {
								log.Warn().Err(err).
									Str("card", job.scrydexID).
									Msg("card image download failed")
								cnt.mu.Lock()
								cnt.errors++
								cnt.mu.Unlock()
								continue
							}
							if newURL == "" {
								// Already existed in S3 — skip the DB update.
								continue
							}
							if err := repo.UpdateCardImages(workerCtx, job.cardID, newURL, newURL); err != nil {
								log.Warn().Err(err).
									Str("card", job.scrydexID).
									Msg("UpdateCardImages failed")
							} else {
								cnt.mu.Lock()
								cnt.imagesDownload++
								cnt.mu.Unlock()
							}
						}
					}()
				}
			}

			httpClient := &http.Client{Timeout: 30 * time.Second}

			for i, exp := range expansions {
				log.Info().Msgf("[%d/%d] %s — %s (lang=%s, series=%q)",
					i+1, len(expansions), exp.ID, exp.Name, exp.Language, exp.Series)

				if *dryRun {
					log.Info().Msgf("    [dry-run] would upsert set %q and its cards", exp.ID)
					continue
				}

				if err := importExpansion(ctx, scrydexClient, repo, provider, httpClient, exp, jobs, *skipImages, cnt); err != nil {
					log.Error().Err(err).Str("expansion", exp.ID).Msg("import expansion failed — skipping")
					cnt.mu.Lock()
					cnt.errors++
					cnt.mu.Unlock()
				}
			}

			// Signal workers no more jobs are coming and wait for them to drain.
			close(jobs)
			wg.Wait()
		}
	}

	if *enrichPT {
		imgHTTP := &http.Client{Timeout: 30 * time.Second}
		tcgdexClient := tcgdex.New(30 * time.Second)
		runPTEnrichment(ctx, repo, provider, imgHTTP, tcgdexClient, *enrichLimit, *dryRun, *skipImages, cnt)
	}

	log.Info().
		Int("sets_new", cnt.setsImported).
		Int("sets_updated", cnt.setsUpdated).
		Int("sets_skipped", cnt.setsSkipped).
		Int("cards_new", cnt.cardsImported).
		Int("cards_updated", cnt.cardsUpdated).
		Int("variants_upserted", cnt.variantsUpsert).
		Int("images_downloaded", cnt.imagesDownload).
		Int("errors", cnt.errors).
		Msg("==> Done")
}

// importExpansion processes a single Scrydex expansion: upserts the series,
// upserts the set (respecting import_source = 'manual'), downloads set images,
// and imports all cards with their variants.
func importExpansion(
	ctx context.Context,
	client *scrydex.Client,
	repo *postgres.CardRepo,
	provider upload.Provider,
	httpClient *http.Client,
	exp scrydex.Expansion,
	jobs chan<- imgJob,
	skipImages bool,
	cnt *counters,
) error {
	// Upsert series. An empty Series field falls back to "Unknown" so we never
	// create a NULL series name in the DB.
	seriesName := exp.Series
	if seriesName == "" {
		seriesName = "Unknown"
	}
	ser, err := repo.UpsertSeriesWithPT(ctx, seriesName, "", "pokemon")
	if err != nil {
		return fmt.Errorf("upsert series %q: %w", seriesName, err)
	}

	releaseDate := parseISO8601(exp.ReleaseDate)

	// For Japanese sets, use the English translation as the primary name when
	// the API actually provides one (no CJK characters). If the API only echoes
	// the Japanese name back in translation.en, we keep the Japanese as name —
	// translate-set-names fills name_en via DeepL, and upsertSetSQL preserves
	// any existing English name on re-import.
	name := exp.Name
	nameEN := exp.Translations.En.Name
	if isJapanese(exp) && nameEN != "" && !containsCJK(nameEN) {
		name = nameEN
	}

	dbSet := card.Set{
		Code:         exp.ID,
		Name:         name,
		NameEN:       nameEN,
		SeriesID:     &ser.ID,
		TCG:          "pokemon",
		Language:     expansionLanguage(exp),
		ReleaseDate:  releaseDate,
		TotalCards:   exp.Total,
		PrintedTotal: exp.PrintedTotal,
		ImportSource: "scrydex",
	}

	if err := upsertSetProtected(ctx, repo, &dbSet, exp.LogoURL, exp.SymbolURL); err != nil {
		return fmt.Errorf("upsert set %q: %w", exp.ID, err)
	}

	isNew := dbSet.UpdatedAt.Sub(dbSet.CreatedAt) < time.Second
	cnt.incSet(isNew)

	if !skipImages && provider != nil {
		downloadSetImages(ctx, httpClient, provider, repo, dbSet, exp.ID, exp.LogoURL, exp.SymbolURL)
	}

	cards, err := client.ListCards(ctx, exp.ID)
	if err != nil {
		// Log but continue — we already persisted the set.
		log.Error().Err(err).Str("expansion", exp.ID).Msg("list cards failed — set imported, cards skipped")
		return nil
	}

	for _, sc := range cards {
		if err := importCard(ctx, repo, dbSet, sc, jobs, skipImages, cnt); err != nil {
			log.Error().Err(err).
				Str("card", sc.ID).
				Msg("import card failed — skipping")
			cnt.mu.Lock()
			cnt.errors++
			cnt.mu.Unlock()
		}
	}

	return nil
}

// importCard upserts a single card and all its variants, then enqueues the
// image download job when applicable.
func importCard(
	ctx context.Context,
	repo *postgres.CardRepo,
	dbSet card.Set,
	sc scrydex.Card,
	jobs chan<- imgJob,
	skipImages bool,
	cnt *counters,
) error {
	dbCard := card.Card{
		SetID:           dbSet.ID,
		Number:          sc.Number,
		CollectorNumber: sc.Number,
		Name:            sc.Name,
		ImageLargeURL:   sc.LargeImageURL(),
		ImageSmallURL:   sc.SmallImageURL(),
		ImportSource:    "scrydex",
	}

	if err := repo.UpsertCard(ctx, &dbCard); err != nil {
		return fmt.Errorf("upsert card: %w", err)
	}

	isNew := dbCard.UpdatedAt.Sub(dbCard.CreatedAt) < time.Second
	cnt.incCard(isNew)

	for _, sv := range sc.Variants {
		finish, label, isPromo, ok := mapVariantFinish(sv.Name)
		if !ok {
			log.Warn().
				Str("card", sc.ID).
				Str("variant", sv.Name).
				Msg("unknown variant name — skipping variant")
			continue
		}
		v := card.Variant{
			CardID:  dbCard.ID,
			Finish:  finish,
			Label:   label,
			IsPromo: isPromo,
		}
		if err := repo.UpsertVariant(ctx, &v); err != nil {
			log.Warn().Err(err).
				Str("card", sc.ID).
				Str("finish", string(finish)).
				Msg("upsert variant failed")
		} else {
			cnt.mu.Lock()
			cnt.variantsUpsert++
			cnt.mu.Unlock()
		}
	}

	// When there are no variants at all in the Scrydex response, fall back to
	// Normal — every card needs at least one variant for pricing to work.
	if len(sc.Variants) == 0 {
		v := card.Variant{CardID: dbCard.ID, Finish: card.FinishNormal}
		if err := repo.UpsertVariant(ctx, &v); err != nil {
			log.Warn().Err(err).Str("card", sc.ID).Msg("fallback normal variant upsert failed")
		} else {
			cnt.mu.Lock()
			cnt.variantsUpsert++
			cnt.mu.Unlock()
		}
	}

	if !skipImages && sc.LargeImageURL() != "" && jobs != nil {
		jobs <- imgJob{
			cardID:    dbCard.ID,
			setCode:   dbSet.Code,
			scrydexID: sc.ID,
			imageURL:  sc.LargeImageURL(),
		}
	}

	return nil
}

// ----------------------------------------------------------------------------
// PT-BR enrichment via TCGDex
// ----------------------------------------------------------------------------

// runPTEnrichment queries scrydex cards that are missing PT-BR data and fills
// name_pt / image_url_pt from the TCGDex API.
//
// The TCGDex card ID format is "{setCode}-{localId}" (e.g. "sv01-001"). Scrydex
// stores the collector number as the numeric part, so we zero-pad it to 3 digits
// to match the TCGDex convention (e.g. "1" → "001"). This covers most sets;
// cards that don't follow the pattern simply get a 404 and are skipped silently.
func runPTEnrichment(
	ctx context.Context,
	repo *postgres.CardRepo,
	provider upload.Provider,
	imgHTTP *http.Client,
	tcgdexClient *tcgdex.Client,
	limit int,
	dryRun, skipImages bool,
	cnt *counters,
) {
	log.Info().Int("limit", limit).Msg("==> PT enrichment phase")

	if dryRun {
		log.Info().Msg("    [dry-run] would query candidates and call TCGDex — no writes")
		return
	}

	candidates, err := repo.ListCardsForPTEnrichment(ctx, limit)
	if err != nil {
		log.Error().Err(err).Msg("list enrichment candidates failed")
		return
	}
	log.Info().Msgf("    %d cards need PT enrichment", len(candidates))

	var enriched, skipped int

	for _, c := range candidates {
		// TCGDex local IDs use zero-padded 3-digit numbers for purely numeric
		// collector numbers (e.g. "1" → "sv01-001"), or the raw value for
		// alphanumeric numbers (e.g. "TG01" → "sv01-TG01").
		localID := buildTCGDexLocalID(c.SetCode, c.CollectorNumber)

		ptCard, err := tcgdexClient.GetCard(ctx, "pt-br", localID)
		if err != nil {
			log.Debug().Err(err).
				Str("card", localID).
				Msg("tcgdex get card pt-br failed — skipping")
			cnt.mu.Lock()
			cnt.errors++
			cnt.mu.Unlock()
			continue
		}
		if ptCard == nil {
			// 404: this card/set is not available in PT-BR — normal for non-Pocket sets.
			log.Debug().Str("card", localID).Msg("not found in pt-br — skipping")
			skipped++
			continue
		}

		namePT := ptCard.Name

		// The TCGDex image field is a base URL without extension; append /high.webp
		// for the high-resolution variant used everywhere else in the importer.
		var imageURLPT string
		if !skipImages && ptCard.Image != "" && provider != nil {
			srcURL := ptCard.Image + "/high.webp"
			key := fmt.Sprintf("pokemon/cards/%s/%s_pt.webp", c.SetCode, c.ID)
			newURL, err := downloadAndStore(ctx, imgHTTP, provider, key, srcURL)
			if err != nil {
				log.Warn().Err(err).
					Str("card", localID).
					Msg("pt-br image download failed")
				cnt.mu.Lock()
				cnt.errors++
				cnt.mu.Unlock()
				// Still persist the name even if the image failed.
			} else if newURL != "" {
				imageURLPT = newURL
				cnt.mu.Lock()
				cnt.imagesDownload++
				cnt.mu.Unlock()
			}
		}

		if namePT == "" && imageURLPT == "" {
			skipped++
			continue
		}

		if err := repo.UpdateCardPT(ctx, c.ID, namePT, imageURLPT); err != nil {
			log.Warn().Err(err).
				Str("card", localID).
				Msg("UpdateCardPT failed")
			cnt.mu.Lock()
			cnt.errors++
			cnt.mu.Unlock()
			continue
		}
		enriched++

		log.Debug().
			Str("card", localID).
			Str("name_pt", namePT).
			Bool("image_stored", imageURLPT != "").
			Msg("enriched")
	}

	log.Info().
		Int("enriched", enriched).
		Int("skipped_no_pt", skipped).
		Int("errors", cnt.errors).
		Msg("==> PT enrichment done")
}

// buildTCGDexLocalID constructs a TCGDex card ID from a set code and collector
// number. Purely numeric collector numbers are zero-padded to 3 digits to match
// the TCGDex convention (e.g. "1" → "sv01-001"). Alphanumeric numbers such as
// "TG01" or "SWSH001" are used verbatim (e.g. "sv01-TG01").
func buildTCGDexLocalID(setCode, collectorNumber string) string {
	if n, err := strconv.Atoi(collectorNumber); err == nil {
		return fmt.Sprintf("%s-%03d", setCode, n)
	}
	return fmt.Sprintf("%s-%s", setCode, collectorNumber)
}

// upsertSetProtected skips manual sets and delegates to UpsertSet for all others.
// When the set exists with import_source = 'manual', s is populated from the
// existing row so callers can use s.ID for the card import loop.
func upsertSetProtected(ctx context.Context, repo *postgres.CardRepo, s *card.Set, logoURL, symbolURL string) error {
	existing, err := repo.GetSetByCode(ctx, s.Code)
	if err == nil && existing.ImportSource == "manual" {
		log.Info().Str("set", s.Code).Msg("skipping manual set")
		*s = existing
		return nil
	}

	// Attach resolved image URLs so the SQL COALESCE fills them in on insert
	// but does not overwrite already-stored S3 keys on update.
	s.ImageURL = logoURL
	s.SymbolURL = symbolURL

	return repo.UpsertSet(ctx, s)
}

// downloadSetImages downloads the logo and symbol for a set and updates the DB.
func downloadSetImages(
	ctx context.Context,
	httpClient *http.Client,
	provider upload.Provider,
	repo *postgres.CardRepo,
	dbSet card.Set,
	setID, logoURL, symbolURL string,
) {
	if logoURL != "" {
		key := fmt.Sprintf("pokemon/sets/%s_logo.png", setID)
		if newURL, err := downloadAndStore(ctx, httpClient, provider, key, logoURL); err != nil {
			log.Warn().Err(err).Str("set", setID).Msg("download set logo failed")
		} else if newURL != "" {
			if err := repo.UpdateSetImageURL(ctx, dbSet.ID, newURL); err != nil {
				log.Warn().Err(err).Str("set", setID).Msg("UpdateSetImageURL failed")
			}
		}
	}
	if symbolURL != "" {
		key := fmt.Sprintf("pokemon/sets/%s_symbol.png", setID)
		if newURL, err := downloadAndStore(ctx, httpClient, provider, key, symbolURL); err != nil {
			log.Warn().Err(err).Str("set", setID).Msg("download set symbol failed")
		} else if newURL != "" {
			if err := repo.UpdateSetSymbolURL(ctx, dbSet.ID, newURL); err != nil {
				log.Warn().Err(err).Str("set", setID).Msg("UpdateSetSymbolURL failed")
			}
		}
	}
}

// downloadAndStore fetches srcURL and stores it at key via provider.
// Returns ("", nil) when the file already exists — callers skip the DB update.
// Returns (publicURL, nil) on a successful new download.
func downloadAndStore(
	ctx context.Context,
	httpClient *http.Client,
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
		// File already in storage. Return the public URL so callers can still
		// update the DB — necessary when a re-import accidentally cleared the
		// DB URL while the S3 file remained intact.
		return provider.PublicURL(key), nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srcURL, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http get %s: %w", srcURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: status %d", srcURL, resp.StatusCode)
	}

	contentType := contentTypeFromKey(key)
	publicURL, err := provider.Put(ctx, key, resp.Body, contentType)
	if err != nil {
		return "", fmt.Errorf("put %s: %w", key, err)
	}
	return publicURL, nil
}

// ----------------------------------------------------------------------------
// Variant mapping
// ----------------------------------------------------------------------------

// mapVariantFinish converts a Scrydex variant name to a (finish, label, isPromo, ok) tuple.
// label is non-empty when multiple variants share the same finish but differ visually
// (e.g. ball-type reverse holos, stamp promos). isPromo flags event/stamp promos so the
// UI can display the PROMO badge. Returns ("", "", false, false) for names that are too
// niche to track (jumbo cards, numbered artist stamps, numbered event stamps, error prints).
func mapVariantFinish(name string) (finish card.Finish, label string, isPromo bool, ok bool) {
	switch name {
	// ── Standard finishes ──────────────────────────────────────────────────────
	case "normal":
		return card.FinishNormal, "", false, true
	case "normalAlternate":
		return card.FinishNormal, "Alternate", false, true
	case "normalUnnumbered":
		return card.FinishNormal, "Unnumbered", false, true
	case "holofoil":
		return card.FinishHolo, "", false, true
	case "reverseHolofoil":
		return card.FinishReverseHolo, "", false, true

	// ── Holo foil variants ─────────────────────────────────────────────────────
	case "cosmosHolofoil":
		return card.FinishCosmosHolo, "", false, true
	case "crackedIceHolofoil":
		return card.FinishHolo, "Cracked Ice", false, true
	case "crackedIcePrismHolofoil":
		return card.FinishHolo, "Cracked Ice Prism", false, true
	case "sheenHolofoil":
		return card.FinishHolo, "Sheen", false, true
	case "sequinHolofoil":
		return card.FinishHolo, "Sequin", false, true
	case "tinselHolofoil":
		return card.FinishHolo, "Tinsel", false, true
	case "waterWebHolofoil":
		return card.FinishHolo, "Water Web", false, true
	case "prismHolofoil":
		return card.FinishHolo, "Prism", false, true
	case "blockPrismHolofoil":
		return card.FinishHolo, "Block Prism", false, true
	case "checkeredPrismHolofoil":
		return card.FinishHolo, "Checkered Prism", false, true
	case "blisterHolofoil":
		return card.FinishHolo, "Blister", false, true
	case "lineHolofoil":
		return card.FinishHolo, "Line", false, true
	case "meadowPinkHolofoil":
		return card.FinishHolo, "Meadow Pink", false, true
	case "noRaritySymbolHolofoil":
		return card.FinishHolo, "No Rarity Symbol", false, true

	// ── Reverse holo variants ──────────────────────────────────────────────────
	case "cosmosReverseHolofoil":
		return card.FinishReverseHolo, "Cosmos", false, true
	case "crackedIceReverseHolofoil":
		return card.FinishReverseHolo, "Cracked Ice", false, true
	case "mirrorReverseHolofoil":
		return card.FinishReverseHolo, "Mirror", false, true
	case "meadowPinkReverseHolofoil":
		return card.FinishReverseHolo, "Meadow Pink", false, true

	// ── Ball/item reverse holos (Gym sets, 151, Paldean Fates…) ───────────────
	// Each ball type is a distinct variant — label is the unique key component.
	case "pokeBallReverseHolofoil":
		return card.FinishReverseHolo, "Poke Ball", false, true
	case "masterBallReverseHolofoil":
		return card.FinishReverseHolo, "Master Ball", false, true
	case "duskBallReverseHolofoil":
		return card.FinishReverseHolo, "Dusk Ball", false, true
	case "energyReverseHolofoil":
		return card.FinishReverseHolo, "Energy", false, true
	case "friendBallReverseHolofoil":
		return card.FinishReverseHolo, "Friend Ball", false, true
	case "loveBallReverseHolofoil":
		return card.FinishReverseHolo, "Love Ball", false, true
	case "quickBallReverseHolofoil":
		return card.FinishReverseHolo, "Quick Ball", false, true
	case "premierBallReverseHolofoil":
		return card.FinishReverseHolo, "Premier Ball", false, true
	case "rocketReverseHolofoil":
		return card.FinishReverseHolo, "Rocket", false, true

	// ── Vintage / Base Set variants ────────────────────────────────────────────
	// Holo ones keep empty label to avoid orphaning existing (card_id,"holo",null) rows.
	case "firstEdition":
		return card.FinishFirstEdition, "", false, true
	case "firstEditionShadowless":
		return card.FinishFirstEdition, "Shadowless", false, true
	case "firstEditionHolofoil",
		"firstEditionShadowlessHolofoil",
		"firstEditionUnlimitedHolofoil":
		return card.FinishHolo, "", false, true
	case "unlimited":
		return card.FinishUnlimited, "", false, true
	case "unlimitedShadowless":
		return card.FinishShadowless, "", false, true
	case "unlimitedHolofoil",
		"unlimitedShadowlessHolofoil":
		return card.FinishHolo, "", false, true
	case "noRaritySymbol":
		return card.FinishNormal, "No Rarity Symbol", false, true
	case "nonEReader":
		return card.FinishNormal, "Non-e-Reader", false, true
	case "blueBack":
		return card.FinishNormal, "Blue Back", false, true
	case "greenBack":
		return card.FinishNormal, "Green Back", false, true
	case "goldBorder":
		return card.FinishNormal, "Gold Border", false, true

	// ── Promo / stamp variants ─────────────────────────────────────────────────
	// Finish reflects the card's foil treatment; label names the stamp/event.
	case "leagueStamp":
		return card.FinishNormal, "League", true, true
	case "leagueCupStamp":
		return card.FinishNormal, "League Cup", true, true
	case "leagueCupStaffStamp":
		return card.FinishNormal, "League Cup Staff", true, true
	case "league1stPlaceStamp":
		return card.FinishNormal, "League 1st Place", true, true
	case "league2ndPlaceStamp":
		return card.FinishNormal, "League 2nd Place", true, true
	case "league3rdPlaceStamp":
		return card.FinishNormal, "League 3rd Place", true, true
	case "league4thPlaceStamp":
		return card.FinishNormal, "League 4th Place", true, true
	case "staffStamp":
		return card.FinishNormal, "Staff", true, true
	case "expansionStamp":
		return card.FinishNormal, "Expansion", true, true
	case "expansionStampHolofoil":
		return card.FinishHolo, "Expansion", true, true
	case "expansionStaffStamp":
		return card.FinishNormal, "Expansion Staff", true, true
	case "cosmosHolofoilExpansionStamp":
		return card.FinishCosmosHolo, "Expansion", true, true
	case "playPokemonStamp":
		return card.FinishNormal, "Play Pokémon", true, true
	case "playPokemonStampHolofoil",
		"playPokemonStampHolofoil ": // trailing space is a Scrydex data quality issue
		return card.FinishHolo, "Play Pokémon", true, true
	case "playPokemonStampReverseHolofoil",
		"playPokemonStampReverseHolofoi": // truncated name in Scrydex
		return card.FinishReverseHolo, "Play Pokémon", true, true
	case "playPokemonThankYouStamp":
		return card.FinishNormal, "Play Pokémon Thank You", true, true
	case "prereleaseStamp":
		return card.FinishNormal, "Prerelease", true, true
	case "prereleaseStaffStamp":
		return card.FinishNormal, "Prerelease Staff", true, true
	case "professorProgram":
		return card.FinishNormal, "Professor Program", true, true
	case "professorProgramStamp":
		return card.FinishNormal, "Professor Program", true, true
	case "professorProgramStampHolofoil":
		return card.FinishHolo, "Professor Program", true, true
	case "professorProgramStampReverseHolofoil":
		return card.FinishReverseHolo, "Professor Program", true, true
	case "professorProgramChampionStamp":
		return card.FinishNormal, "Professor Program Champion", true, true
	case "professorProgramTop4Stamp":
		return card.FinishNormal, "Professor Program Top 4", true, true
	case "professorProgramTop8Stamp":
		return card.FinishNormal, "Professor Program Top 8", true, true
	case "professorProgramStaffStamp":
		return card.FinishNormal, "Professor Program Staff", true, true
	case "holidayStamp":
		return card.FinishNormal, "Holiday", true, true
	case "snowflakeStamp":
		return card.FinishNormal, "Snowflake", true, true
	case "anniversaryStamp":
		return card.FinishNormal, "Anniversary", true, true
	case "gamestopStamp":
		return card.FinishNormal, "GameStop", true, true
	case "buildABearStamp":
		return card.FinishNormal, "Build-A-Bear", true, true
	case "toysRUsStamp":
		return card.FinishNormal, "Toys R Us", true, true
	case "ebGamesStamp", "ebgamesStamp":
		return card.FinishNormal, "EB Games", true, true
	case "burgerKingStamp", "burgerKingExpansionStamp":
		return card.FinishNormal, "Burger King", true, true
	case "sevenElevenStamp":
		return card.FinishNormal, "7-Eleven", true, true
	case "pokemonCenterStamp":
		return card.FinishNormal, "Pokémon Center", true, true
	case "pokemonDayStamp":
		return card.FinishNormal, "Pokémon Day", true, true
	case "pokemonHorizonsStamp":
		return card.FinishNormal, "Pokémon Horizons", true, true
	case "pokemonRocksAmericaStamp":
		return card.FinishNormal, "Pokémon Rocks America", true, true
	case "pokemonTogetherStamp":
		return card.FinishNormal, "Pokémon Together", true, true
	case "pokeTourStamp":
		return card.FinishNormal, "Poké Tour", true, true
	case "eeveeStamp":
		return card.FinishNormal, "Eevee", true, true
	case "mewtwoStamp":
		return card.FinishNormal, "Mewtwo", true, true
	case "darkraiStamp":
		return card.FinishNormal, "Darkrai", true, true
	case "ionoStamp":
		return card.FinishNormal, "Iono", true, true
	case "detectivePikachuStamp":
		return card.FinishNormal, "Detective Pikachu", true, true
	case "legendaryPokemonStamp":
		return card.FinishNormal, "Legendary Pokémon", true, true
	case "movieStamp":
		return card.FinishNormal, "Movie", true, true
	case "gymChallengeStamp":
		return card.FinishNormal, "Gym Challenge", true, true
	case "wStamp":
		return card.FinishNormal, "W Stamp", true, true
	case "jrStampRally":
		return card.FinishNormal, "JR Stamp Rally", true, true
	case "blackStarPromo":
		return card.FinishNormal, "Black Star", true, true
	case "stamp", "stamp22":
		return card.FinishNormal, "Stamp", true, true
	case "winnerStamp":
		return card.FinishNormal, "Winner", true, true
	case "finalistStamp", "semiFinalistStamp":
		return card.FinishNormal, "Finalist", true, true
	case "quarterFinalistStamp":
		return card.FinishNormal, "Quarter Finalist", true, true
	case "top16Stamp":
		return card.FinishNormal, "Top 16", true, true
	case "top32Stamp", "topThirtyTwo":
		return card.FinishNormal, "Top 32", true, true
	case "championStamp":
		return card.FinishNormal, "Champion", true, true
	case "championStaff":
		return card.FinishNormal, "Champion Staff", true, true
	case "battleRoadStamp", "battleRoadStampAlternate", "battleRoadStampAlternate2":
		return card.FinishNormal, "Battle Road", true, true
	case "comicConStamp", "comicConStaffStamp":
		return card.FinishNormal, "Comic Con", true, true
	case "genConStamp":
		return card.FinishNormal, "Gen Con", true, true
	case "scryeStamp":
		return card.FinishNormal, "Scrye", true, true
	case "inquestGamerStamp":
		return card.FinishNormal, "InQuest Gamer", true, true
	case "gamesExpoStamp":
		return card.FinishNormal, "Games Expo", true, true
	case "rainCityShowcaseStamp":
		return card.FinishNormal, "Rain City Showcase", true, true
	case "nationalChampionshipsStamp", "nationalChampionshipsStaffStamp":
		return card.FinishNormal, "National Championships", true, true
	case "regionalChampionshipsStamp", "regionalChampionshipsStaffStamp":
		return card.FinishNormal, "Regional Championships", true, true
	case "internationalChallengeStamp", "internationalChallengeStaffStamp":
		return card.FinishNormal, "International Challenge", true, true
	case "oceaniaChampionshipsStamp", "oceaniaChampionshipsStaffStamp", "oceaniaStamp":
		return card.FinishNormal, "Oceania Championships", true, true
	case "asiaChampionshipStamp":
		return card.FinishNormal, "Asia Championship", true, true
	case "worldChampionshipsStamp", "worldChampionshipsStaffStamp", "worldsStamp", "worldsStaffStamp":
		return card.FinishNormal, "World Championships", true, true
	case "latinStamp", "latinStaffStamp":
		return card.FinishNormal, "Latin America", true, true
	case "ultraBallLeagueStamp":
		return card.FinishNormal, "Ultra Ball League", true, true
	case "greatBallLeagueStamp":
		return card.FinishNormal, "Great Ball League", true, true

	default:
		return "", "", false, false
	}
}

// ----------------------------------------------------------------------------
// Filter helpers
// ----------------------------------------------------------------------------

// applyFilters applies the --set, --series, and --lang flags to the expansion list.
// --lang accepts both ISO codes ("ja") and full names ("Japanese") because the
// Scrydex API returns full names (e.g. "Japanese") while users type short codes.
func applyFilters(exps []scrydex.Expansion, setFilter, seriesFilter, langFilter string) []scrydex.Expansion {
	var out []scrydex.Expansion
	for _, e := range exps {
		if setFilter != "" && !strings.EqualFold(e.ID, setFilter) {
			continue
		}
		if seriesFilter != "" && !strings.EqualFold(e.Series, seriesFilter) {
			continue
		}
		if langFilter != "all" {
			// Normalize both sides via expansionLanguage so "ja" matches "Japanese".
			want := expansionLanguage(scrydex.Expansion{Language: langFilter})
			got := expansionLanguage(e)
			if got != want {
				continue
			}
		}
		out = append(out, e)
	}
	return out
}

// ----------------------------------------------------------------------------
// Language helpers
// ----------------------------------------------------------------------------

// expansionLanguage maps a Scrydex Expansion to the domain Language constant.
func expansionLanguage(exp scrydex.Expansion) card.Language {
	lang := strings.ToLower(exp.Language)
	switch lang {
	case "ja", "japanese":
		return card.LanguageJapanese
	case "ko", "korean":
		return card.LanguageKorean
	case "zh-tw", "chinese":
		return card.LanguageChinese
	default:
		return card.LanguageEnglish
	}
}

// isJapanese reports whether this expansion is a Japanese-language release.
func isJapanese(exp scrydex.Expansion) bool {
	return strings.EqualFold(exp.Language, "ja") ||
		strings.EqualFold(exp.Language, "japanese") ||
		strings.EqualFold(exp.Region, "japan")
}

// ----------------------------------------------------------------------------
// Misc utilities
// ----------------------------------------------------------------------------

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

// containsCJK reports whether s contains CJK unified ideographs, hiragana, or
// katakana — used to detect whether a "translated" name is actually Japanese.
func containsCJK(s string) bool {
	for _, r := range s {
		if (r >= 0x4E00 && r <= 0x9FFF) || (r >= 0x3040 && r <= 0x30FF) {
			return true
		}
	}
	return false
}

// contentTypeFromKey derives a MIME type from the file extension in a storage key.
func contentTypeFromKey(key string) string {
	switch strings.ToLower(filepath.Ext(path.Base(key))) {
	case ".webp":
		return "image/webp"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	default:
		return "image/png"
	}
}
