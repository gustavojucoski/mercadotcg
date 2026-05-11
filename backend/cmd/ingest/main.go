// cmd/ingest é o job de ingestão de preços do MercadoTCG.
//
// Fluxo:
//
//  1. Conecta ao banco via DATABASE_URL.
//  2. Garante que as partições do trimestre atual e do próximo existem.
//  3. Carrega todos os card_variants ativos com seus dados de card+set.
//  4. Para cada variante, constrói um scraper.Query e consulta cada fonte
//     configurada, respeitando rate limits por fonte.
//  5. Cada resultado passa pelo matching.Service; resultados quarentenados
//     são contados e ignorados; os demais são preenchidos via pricing.Service
//     e acumulados no buffer.
//  6. A cada 500 observações (ou ao final) o buffer é desduplicado e persistido
//     via InsertBatch (pgx.CopyFrom).
//  7. Ao final, executa RebuildDay para hoje para atualizar price_daily.
//
// Flags:
//
//	-dry-run   não persiste nada, apenas loga o que seria feito.
//	-limit N   processa apenas as primeiras N variantes (útil para teste).
//	-source X  processa apenas a fonte X (ex.: "ligapokemon", "tcgplayer").
//
// DATABASE_URL é obrigatório. O binário não chama config.Load() — não requer
// JWT_SECRET nem nenhuma outra variável do servidor HTTP.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/pricing"
	"github.com/gustavojucoski/mercadotcg/backend/internal/forex"
	"github.com/gustavojucoski/mercadotcg/backend/internal/repository/postgres"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper/cardmarket"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper/ebay"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper/ligapokemon"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper/pokewallet"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper/tcgplayer"
	matchingsvc "github.com/gustavojucoski/mercadotcg/backend/internal/service/matching"
	pricingsvc "github.com/gustavojucoski/mercadotcg/backend/internal/service/pricing"
)

// batchSize é o número máximo de observações acumuladas antes de um InsertBatch.
const batchSize = 500

// variantRow é a projeção plana de card_variant + card + card_set usada pelo job.
type variantRow struct {
	VariantID    string
	CardName     string
	CardNumber   string
	SetCode      string
	SetName      string
	PrintedTotal int
}

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})

	// ---- flags ---------------------------------------------------------------
	dryRun   := flag.Bool("dry-run", false, "não persiste nada, apenas loga o que seria feito")
	limitN   := flag.Int("limit", 0, "processar apenas as primeiras N variantes (0 = sem limite)")
	sourceF  := flag.String("source", "", "processar apenas esta fonte (ex.: ligapokemon, tcgplayer)")
	flag.Parse()

	// ---- banco de dados -------------------------------------------------------
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		fatalf("DATABASE_URL é obrigatório")
	}

	ctx := context.Background()

	pool, err := postgres.Connect(ctx, databaseURL)
	if err != nil {
		fatalf("conectar ao banco: %v", err)
	}
	defer pool.Close()

	// ---- garantir partições --------------------------------------------------
	if !*dryRun {
		if err := ensurePartitions(ctx, pool); err != nil {
			fatalf("ensurePartitions: %v", err)
		}
	} else {
		log.Info().Msg("dry-run: pulando ensurePartitions")
	}

	// ---- scrapers ------------------------------------------------------------
	pokeWalletKey := os.Getenv("POKEWALLET_API_KEY")
	flareSolverr  := os.Getenv("FLARESOLVERR_URL")

	pwTCG, pwCM := pokewallet.New(pokeWalletKey, 20*time.Second)

	var cmLegacy scraper.Source
	if flareSolverr != "" {
		cmLegacy = cardmarket.NewWithFlareSolverr(25*time.Second, flareSolverr)
	} else {
		cmLegacy = cardmarket.New(25 * time.Second)
	}
	tcgLegacy := tcgplayer.New(15 * time.Second)

	registry := scraper.NewRegistry()
	registry.Register(pricing.SourceCardmarket, pwCM,     scraper.PrimarySource,  3)
	registry.Register(pricing.SourceCardmarket, cmLegacy, scraper.FallbackSource, 5)
	registry.Register(pricing.SourceTCGPlayer,  pwTCG,    scraper.PrimarySource,  3)
	registry.Register(pricing.SourceTCGPlayer,  tcgLegacy, scraper.FallbackSource, 5)

	allSources := []scraper.Source{
		ligapokemon.New(15 * time.Second),
		registry.ForSource(pricing.SourceCardmarket),
		registry.ForSource(pricing.SourceTCGPlayer),
		ebay.New(15 * time.Second),
	}

	// Filtrar por -source se informado.
	activeSources := filterSources(allSources, *sourceF)
	if len(activeSources) == 0 {
		fatalf("nenhuma fonte ativa (filtro: %q)", *sourceF)
	}

	// ---- serviços ------------------------------------------------------------
	forexRepo  := postgres.NewForexRepo(pool)
	bcb        := forex.NewBCBProvider(15 * time.Second)
	forexSvc   := forex.NewService(forexRepo, bcb)
	pricesSvc  := pricingsvc.NewService(forexSvc)
	matchSvc   := matchingsvc.New(pool)
	histRepo   := postgres.NewPriceHistoryRepo(pool)
	dailyRepo  := postgres.NewPriceDailyRepo(pool)

	// ---- carregar variantes --------------------------------------------------
	variants, err := listAllVariants(ctx, pool)
	if err != nil {
		fatalf("listar variantes: %v", err)
	}

	if *limitN > 0 && *limitN < len(variants) {
		variants = variants[:*limitN]
	}

	log.Info().
		Int("variants", len(variants)).
		Int("sources", len(activeSources)).
		Bool("dry_run", *dryRun).
		Msg("ingest: início do job")

	// ---- rate limiters por fonte ---------------------------------------------
	// pokewallet.io free tier: 100 req/hora → 1 req por 40s (com folga).
	// eBay/Scrydex: sem limite documentado → 1 req por 5s por precaução.
	// LigaPokemon: sem limite externo, mas limitamos a 1 req por 2s para ser cortês.
	limiters := map[string]*rateLimiter{
		"pokewallet_tcgplayer": newRateLimiter(40 * time.Second),
		"pokewallet_cardmarket": newRateLimiter(40 * time.Second),
		// O SourceAdapter do registry tem Name() == logical source (pricing.Source).
		string(pricing.SourceTCGPlayer):  newRateLimiter(40 * time.Second),
		string(pricing.SourceCardmarket): newRateLimiter(40 * time.Second),
		string(pricing.SourceEbay):       newRateLimiter(5 * time.Second),
		string(pricing.SourceLigaPokemon): newRateLimiter(2 * time.Second),
	}

	// ---- contadores de sumário -----------------------------------------------
	var (
		totalVariants  = len(variants)
		totalResults   int
		totalQuarantined int
		totalIngested  int
		totalDailyRows int64
	)

	// buffer de observações pendentes para InsertBatch.
	buffer := make([]pricing.Observation, 0, batchSize)

	flush := func() {
		if len(buffer) == 0 {
			return
		}
		deduped := deduplicate(buffer)
		if *dryRun {
			log.Info().
				Int("observations", len(deduped)).
				Msg("dry-run: InsertBatch seria chamado aqui")
			buffer = buffer[:0]
			return
		}
		n, err := histRepo.InsertBatch(ctx, deduped)
		if err != nil {
			log.Error().Err(err).Int("observations", len(deduped)).Msg("InsertBatch falhou")
		} else {
			totalIngested += int(n)
			log.Debug().Int64("inserted", n).Msg("InsertBatch concluído")
		}
		buffer = buffer[:0]
	}

	jobStart := time.Now()

	// ---- loop principal -------------------------------------------------------
	for _, v := range variants {
		q := scraper.Query{
			Name:            v.CardName,
			Number:          v.CardNumber,
			SetCode:         v.SetCode,
			SetName:         v.SetName,
			SetPrintedTotal: v.PrintedTotal,
			Limit:           20,
		}

		for _, src := range activeSources {
			srcName := string(src.Name())

			// Aplicar rate limit antes de chamar o scraper.
			if lim, ok := limiters[srcName]; ok {
				lim.Wait()
			}

			srcCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			results, err := src.Search(srcCtx, q)
			cancel()

			if err != nil {
				log.Debug().
					Err(err).
					Str("source", srcName).
					Str("variant_id", v.VariantID).
					Msg("scraper retornou erro, pulando")
				continue
			}

			totalResults += len(results)

			for _, result := range results {
				rr, err := matchSvc.Resolve(ctx, src.Name(), result, q)
				if err != nil {
					log.Error().
						Err(err).
						Str("source", srcName).
						Str("variant_id", v.VariantID).
						Str("external_id", result.ExternalID).
						Msg("matching.Resolve falhou, pulando resultado")
					continue
				}

				if rr.Action == matchingsvc.ActionQuarantined {
					totalQuarantined++
					log.Debug().
						Str("source", srcName).
						Str("variant_id", v.VariantID).
						Str("title", result.Title).
						Int("confidence", rr.Confidence).
						Msg("resultado quarentenado")
					continue
				}

				// Construir a observação base a partir do Result.
				obs := buildObservation(rr.VariantID, result, src.Name())

				// Normalizar BRL via forex.
				if err := pricesSvc.FillObservation(ctx, &obs); err != nil {
					log.Error().
						Err(err).
						Str("source", srcName).
						Str("variant_id", v.VariantID).
						Msg("FillObservation falhou, pulando resultado")
					continue
				}

				buffer = append(buffer, obs)

				if len(buffer) >= batchSize {
					flush()
				}
			}
		}
	}

	// Flush final.
	flush()

	// ---- RebuildDay ----------------------------------------------------------
	today := time.Now().UTC().Truncate(24 * time.Hour)
	if *dryRun {
		log.Info().Str("date", today.Format("2006-01-02")).Msg("dry-run: RebuildDay seria chamado aqui")
	} else {
		n, err := dailyRepo.RebuildDay(ctx, today)
		if err != nil {
			log.Error().Err(err).Str("date", today.Format("2006-01-02")).Msg("RebuildDay falhou")
		} else {
			totalDailyRows = n
			log.Info().
				Str("date", today.Format("2006-01-02")).
				Int64("rows", n).
				Msg("RebuildDay concluído")
		}
	}

	// ---- sumário final -------------------------------------------------------
	log.Info().
		Int("total_variants", totalVariants).
		Int("total_results", totalResults).
		Int("quarantined", totalQuarantined).
		Int("ingested", totalIngested).
		Int64("rows_daily", totalDailyRows).
		Dur("elapsed", time.Since(jobStart)).
		Msg("ingest: job concluído")
}

// buildObservation converte um scraper.Result em uma pricing.Observation parcial
// (sem price_brl e fx_rate_used — preenchidos por FillObservation depois).
//
// variantID é o UUID resolvido pelo matching.Service — pode diferir do variant
// da iteração quando o ref já existia apontando para outro variant_id.
func buildObservation(variantID uuid.UUID, r scraper.Result, source pricing.Source) pricing.Observation {
	// Mapear condition raw para nosso enum.
	cond := pricing.ConditionFromTCG(r.Condition)
	if cond == "" {
		// Condition não mapeada — default NM para não perder o dado.
		// O valor original está em r.RawCondition para auditoria.
		cond = pricing.ConditionNearMint
	}

	kind := r.Kind
	if kind == "" {
		kind = pricing.KindListing
	}

	return pricing.Observation{
		VariantID:     variantID,
		Condition:     cond,
		Source:        source,
		Kind:          kind,
		PriceOriginal: r.Price,
		Currency:      r.Currency,
		Quantity:      max(r.Stock, 1),
		ExternalURL:   r.URL,
		ExternalID:    r.ExternalID,
		ObservedAt:    time.Now().UTC(),
	}
}

// deduplicate remove observações duplicadas pelo critério
// (source, external_id, date(observed_at)), mantendo a mais recente.
// Observações sem external_id não são deduplicadas (cada uma é única).
func deduplicate(obs []pricing.Observation) []pricing.Observation {
	type dedupKey struct {
		source     string
		externalID string
		day        string // YYYY-MM-DD
	}

	seen  := make(map[dedupKey]int, len(obs)) // key → índice em out
	out   := make([]pricing.Observation, 0, len(obs))

	for _, o := range obs {
		if o.ExternalID == "" {
			// Sem external_id: não há critério de deduplicação — mantém tudo.
			out = append(out, o)
			continue
		}
		k := dedupKey{
			source:     string(o.Source),
			externalID: o.ExternalID,
			day:        o.ObservedAt.UTC().Format("2006-01-02"),
		}
		if idx, exists := seen[k]; exists {
			// Mantém o mais recente.
			if o.ObservedAt.After(out[idx].ObservedAt) {
				out[idx] = o
			}
			continue
		}
		seen[k] = len(out)
		out = append(out, o)
	}
	return out
}

// ensurePartitions verifica e cria as partições trimestrais de price_history
// para o trimestre atual e o próximo. Idempotente.
func ensurePartitions(ctx context.Context, pool *pgxpool.Pool) error {
	now := time.Now().UTC()

	quarters := []struct{ year, q int }{
		quarterOf(now),
		nextQuarter(quarterOf(now)),
	}

	// Buscar partições já existentes.
	rows, err := pool.Query(ctx, `
		SELECT tablename
		FROM pg_tables
		WHERE schemaname = 'public'
		  AND tablename LIKE 'price_history_%'`)
	if err != nil {
		return fmt.Errorf("listar partições: %w", err)
	}
	existing := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return fmt.Errorf("scan tablename: %w", err)
		}
		existing[name] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterar partições: %w", err)
	}

	for _, qtr := range quarters {
		name := fmt.Sprintf("price_history_%d_q%d", qtr.year, qtr.q)
		if existing[name] {
			log.Debug().Str("partition", name).Msg("partição já existe")
			continue
		}

		from, to := quarterBounds(qtr.year, qtr.q)
		ddl := fmt.Sprintf(
			`CREATE TABLE IF NOT EXISTS %s PARTITION OF price_history FOR VALUES FROM ('%s') TO ('%s')`,
			name,
			from.Format("2006-01-02"),
			to.Format("2006-01-02"),
		)

		if _, err := pool.Exec(ctx, ddl); err != nil {
			return fmt.Errorf("criar partição %s: %w", name, err)
		}
		log.Info().Str("partition", name).
			Str("from", from.Format("2006-01-02")).
			Str("to", to.Format("2006-01-02")).
			Msg("partição criada")
	}
	return nil
}

// quarterOf retorna o ano e trimestre (1-4) de um time.Time.
func quarterOf(t time.Time) struct{ year, q int } {
	m := int(t.Month())
	q := (m-1)/3 + 1
	return struct{ year, q int }{t.Year(), q}
}

// nextQuarter devolve o trimestre seguinte ao fornecido.
func nextQuarter(cur struct{ year, q int }) struct{ year, q int } {
	if cur.q == 4 {
		return struct{ year, q int }{cur.year + 1, 1}
	}
	return struct{ year, q int }{cur.year, cur.q + 1}
}

// quarterBounds devolve [from, to) para um trimestre dado.
func quarterBounds(year, q int) (from, to time.Time) {
	startMonth := time.Month((q-1)*3 + 1)
	from = time.Date(year, startMonth, 1, 0, 0, 0, 0, time.UTC)
	to   = from.AddDate(0, 3, 0)
	return
}

// listAllVariants executa um SELECT plano de card_variants JOIN cards JOIN card_sets.
// Retorna apenas variantes cujo set tem código não vazio (ignora dados corrompidos).
const listAllVariantsSQL = `
SELECT
    cv.id::text,
    c.name::text,
    c.number,
    cs.code,
    cs.name,
    COALESCE(cs.total_cards, 0)
FROM card_variants cv
JOIN cards      c  ON c.id  = cv.card_id
JOIN card_sets  cs ON cs.id = c.set_id
WHERE cs.code <> ''
ORDER BY cs.release_date DESC NULLS LAST, c.number ASC`

func listAllVariants(ctx context.Context, pool *pgxpool.Pool) ([]variantRow, error) {
	rows, err := pool.Query(ctx, listAllVariantsSQL)
	if err != nil {
		return nil, fmt.Errorf("query card_variants: %w", err)
	}
	defer rows.Close()

	var out []variantRow
	for rows.Next() {
		var v variantRow
		if err := rows.Scan(
			&v.VariantID, &v.CardName, &v.CardNumber,
			&v.SetCode, &v.SetName, &v.PrintedTotal,
		); err != nil {
			return nil, fmt.Errorf("scan variant row: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// filterSources devolve apenas as fontes cujo Name() contém o filtro
// (case-insensitive). Se filter é vazio, devolve todas.
func filterSources(sources []scraper.Source, filter string) []scraper.Source {
	if filter == "" {
		return sources
	}
	filter = strings.ToLower(filter)
	var out []scraper.Source
	for _, s := range sources {
		if strings.Contains(strings.ToLower(string(s.Name())), filter) {
			out = append(out, s)
		}
	}
	return out
}

// rateLimiter é um limitador simples baseado em tempo mínimo entre chamadas.
// Mais simples que golang.org/x/time/rate e sem dependência externa.
type rateLimiter struct {
	interval time.Duration
	last     time.Time
}

func newRateLimiter(interval time.Duration) *rateLimiter {
	return &rateLimiter{interval: interval}
}

// Wait bloqueia até que o intervalo mínimo desde a última chamada tenha passado.
func (l *rateLimiter) Wait() {
	if l.last.IsZero() {
		l.last = time.Now()
		return
	}
	elapsed := time.Since(l.last)
	if elapsed < l.interval {
		time.Sleep(l.interval - elapsed)
	}
	l.last = time.Now()
}

// max devolve o maior de dois inteiros. Evita dep de slices.Max para compat
// com versões mais antigas do toolchain nos testes de CI.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// fatalf loga mensagem fatal e encerra com código 1.
func fatalf(format string, args ...any) {
	log.Fatal().Msgf(format, args...)
}
