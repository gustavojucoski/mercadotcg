// Command api é o servidor HTTP do MercadoTCG.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/gustavojucoski/mercadotcg/backend/internal/config"
	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/pricing"
	"github.com/gustavojucoski/mercadotcg/backend/internal/forex"
	"github.com/gustavojucoski/mercadotcg/backend/internal/handler"
	"github.com/gustavojucoski/mercadotcg/backend/internal/pokemontcgio"
	"github.com/gustavojucoski/mercadotcg/backend/internal/repository/postgres"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper/cardmarket"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper/ebay"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper/ligapokemon"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper/tcgplayer"
	pricesvc "github.com/gustavojucoski/mercadotcg/backend/internal/service/pricing"
	"github.com/gustavojucoski/mercadotcg/backend/internal/service/pricesignal"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config inválida: %v\n", err)
		os.Exit(1)
	}

	zerolog.TimeFieldFormat = time.RFC3339Nano
	if lvl, perr := zerolog.ParseLevel(cfg.LogLevel); perr == nil {
		zerolog.SetGlobalLevel(lvl)
	}

	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// ---- Persistência -----------------------------------------------------
	connectCtx, cancel := context.WithTimeout(rootCtx, 10*time.Second)
	defer cancel()
	pool, err := postgres.Connect(connectCtx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("conectar postgres")
	}
	defer pool.Close()

	cardRepo := postgres.NewCardRepo(pool)
	storeRepo := postgres.NewStoreRepo(pool)
	stockRepo := postgres.NewStockRepo(pool)
	forexRepo := postgres.NewForexRepo(pool)

	// ---- Serviços ---------------------------------------------------------
	bcb := forex.NewBCBProvider(15 * time.Second)
	forexSvc := forex.NewService(forexRepo, bcb)
	_ = pricesvc.NewService(forexSvc) // pricing.Service ainda não tem rota; usado por scrapers
	signalSvc := pricesignal.NewService(pool)

	ptcgClient := pokemontcgio.New(10*time.Second, cfg.PokemonTCGAPIKey)

	// ---- HTTP -------------------------------------------------------------
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"}, // dev: aberto; em prod restringe à origem do front.
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// ---- Scrapers ------------------------------------------------------------
	// LigaPokemon: HTML scraping sem credenciais (BRL).
	// TCGplayer: pricepoints API pública; ExternalID = product ID (USD).
	// eBay: vendas via Scrydex; ExternalID = pokemontcg.io card ID (USD).
	// Cardmarket: via FlareSolverr se FLARESOLVERR_URL configurado; caso contrário
	//   tentativa direta (provavelmente 403) e fallback via pokemontcg.io (EUR).
	const cmTimeout = 70 * time.Second // FlareSolverr pode levar até 60s
	var cmScraper scraper.Source
	if cfg.FlareSolverrURL != "" {
		cmScraper = cardmarket.NewWithFlareSolverr(cmTimeout, cfg.FlareSolverrURL)
		log.Info().Str("flaresolverr", cfg.FlareSolverrURL).Msg("Cardmarket usando FlareSolverr")
	} else {
		cmScraper = cardmarket.New(12 * time.Second)
	}

	scrapers := []scraper.Source{
		ligapokemon.New(12 * time.Second),
		tcgplayer.New(12 * time.Second),
		ebay.New(12 * time.Second),
		cmScraper,
	}

	// API v1
	r.Route("/api/v1", func(r chi.Router) {
		handler.NewCardHandler(cardRepo, signalSvc).Routes(r)
		handler.NewStoreHandler(storeRepo, stockRepo, signalSvc).Routes(r)
		handler.NewVariantHandler(signalSvc).Routes(r)

		extHandler := handler.NewExternalHandler(scrapers...).WithCatalog(ptcgClient)
		if cfg.FlareSolverrURL != "" {
			// FlareSolverr precisa de timeout maior — outras fontes têm 12s.
			extHandler = extHandler.WithSourceTimeout(pricing.SourceCardmarket, cmTimeout)
		}
		extHandler.Routes(r)
	})

	// Imprime as rotas no boot — útil pra você descobrir o que tem.
	_ = chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		log.Info().Str("method", method).Str("route", route).Msg("rota registrada")
		return nil
	})

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Info().Int("port", cfg.HTTPPort).Str("env", cfg.Env).Msg("api iniciada")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("erro no http server")
		}
	}()

	<-rootCtx.Done()
	log.Info().Msg("encerrando api")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("shutdown forçado")
	}
}
