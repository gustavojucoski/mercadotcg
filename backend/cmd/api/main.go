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

	"github.com/gustavojucoski/mercadotcg/backend/internal/auth"
	"github.com/gustavojucoski/mercadotcg/backend/internal/config"
	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/user"
	"github.com/gustavojucoski/mercadotcg/backend/internal/email"
	"github.com/gustavojucoski/mercadotcg/backend/internal/forex"
	"github.com/gustavojucoski/mercadotcg/backend/internal/handler"
	"github.com/gustavojucoski/mercadotcg/backend/internal/pokemontcgio"
	"github.com/gustavojucoski/mercadotcg/backend/internal/repository/postgres"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper/ebay"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper/ligapokemon"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper/pokewallet"
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
	userRepo := postgres.NewUserRepo(pool)
	tokenRepo := postgres.NewTokenRepo(pool)
	storeMemberRepo := postgres.NewStoreMemberRepo(pool)

	// ---- Serviços ---------------------------------------------------------
	bcb := forex.NewBCBProvider(15 * time.Second)
	forexSvc := forex.NewService(forexRepo, bcb)
	_ = pricesvc.NewService(forexSvc) // pricing.Service ainda não tem rota; usado por scrapers
	signalSvc := pricesignal.NewService(pool)

	ptcgClient := pokemontcgio.New(10*time.Second, cfg.PokemonTCGAPIKey)

	// ---- Auth ---------------------------------------------------------------
	tokenSvc := auth.NewTokenService(cfg.JWTSecret, cfg.JWTAccessTTL, cfg.JWTRefreshTTL)
	oauthSvc := auth.NewOAuthService(cfg.GoogleClientID, cfg.GoogleClientSecret,
		cfg.GoogleRedirectURL, cfg.OAuthStateHMACKey)

	var mailer email.Provider
	if cfg.ResendAPIKey != "" {
		mailer = email.NewResendProvider(cfg.ResendAPIKey, cfg.EmailFromAddress)
	} else {
		mailer = email.NewNoopProvider()
	}

	authSvc := auth.NewService(userRepo, tokenRepo, tokenSvc, oauthSvc, mailer,
		auth.ServiceConfig{FrontendBaseURL: cfg.FrontendBaseURL})
	authMw := auth.NewMiddleware(tokenSvc, storeMemberRepo)

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
	// PokéWallet: API oficial pokewallet.io — cobre TCGPlayer (USD) e Cardmarket (EUR).
	//   Uma chamada por carta, retornada para ambas as fontes via cache interno.
	// eBay: vendas gradeadas via Scrydex; ExternalID = pokemontcg.io card ID (USD).
	pwTCG, pwCM := pokewallet.New(cfg.PokeWalletAPIKey, 15*time.Second)

	scrapers := []scraper.Source{
		ligapokemon.New(12 * time.Second),
		pwTCG,
		pwCM,
		ebay.New(12 * time.Second),
	}

	// API v1
	r.Route("/api/v1", func(r chi.Router) {
		handler.NewAuthHandler(authSvc, tokenSvc, authMw, userRepo, handler.AuthHandlerConfig{
			FrontendBaseURL: cfg.FrontendBaseURL,
		}).Routes(r)

		handler.NewAdminHandler(userRepo, storeRepo, storeMemberRepo, authMw).Routes(r)

		handler.NewCardHandler(cardRepo, signalSvc).Routes(r)

		// Rotas de loja: leitura pública, escrita requer membro com stock_manager+.
		storeH := handler.NewStoreHandler(storeRepo, stockRepo, signalSvc)
		storeH.Routes(r)
		r.With(authMw.RequireStoreRole(user.StoreRoleStockManager)).
			Post("/stores/{id}/stock/purchase", storeH.RegisterPurchase)
		r.With(authMw.RequireStoreRole(user.StoreRoleStockManager)).
			Post("/stores/{id}/stock-items/{itemID}/sale", storeH.RegisterSale)

		handler.NewVariantHandler(signalSvc).Routes(r)

		// External search restrito a platform_admin.
		extH := handler.NewExternalHandler(scrapers...).WithCatalog(ptcgClient)
		r.With(authMw.RequirePlatformAdmin).Get("/external-search", extH.Search)
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
