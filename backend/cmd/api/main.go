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
	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/pricing"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper/cardmarket"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper/ebay"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper/ligapokemon"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper/pokewallet"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper/tcgplayer"
	pricesvc "github.com/gustavojucoski/mercadotcg/backend/internal/service/pricing"
	"github.com/gustavojucoski/mercadotcg/backend/internal/service/pricesignal"
	"github.com/gustavojucoski/mercadotcg/backend/internal/upload"
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
	priceDailyRepo := postgres.NewPriceDailyRepo(pool)
	storeRepo := postgres.NewStoreRepo(pool)
	stockRepo := postgres.NewStockRepo(pool)
	forexRepo := postgres.NewForexRepo(pool)
	userRepo := postgres.NewUserRepo(pool)
	tokenRepo := postgres.NewTokenRepo(pool)
	storeMemberRepo := postgres.NewStoreMemberRepo(pool)
	storeAuditRepo := postgres.NewStoreAuditRepo(pool)

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

	// ---- Upload storage ---------------------------------------------------
	localUploads, err := upload.NewLocal(cfg.UploadsDir, cfg.UploadsBaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("inicializar storage de uploads")
	}

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

	// Serve arquivos de upload em /uploads/* (dev: disco local; prod: CDN/S3 serve direto).
	r.Handle("/uploads/*", http.StripPrefix("/uploads", localUploads.FileServer()))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// ---- Scrapers ------------------------------------------------------------
	// LigaPokemon: HTML scraping sem credenciais (BRL). Sem fallback — fonte única.
	// PokéWallet: pokewallet.io — cobre TCGPlayer (USD) e Cardmarket (EUR) via uma
	//   única chamada HTTP por carta (cache interno de 60s). Free tier: 100 req/hora.
	// Legados como fallback: tcgplayer/ (mpapi sem credenciais) e cardmarket/
	//   (HTML scraping, opcionalmente via FlareSolverr). Ficam inativos quando o
	//   circuito está aberto ou quando pokewallet responde normalmente.
	// eBay: vendas gradeadas via Scrydex. Sem fallback por ora.
	pwTCG, pwCM := pokewallet.New(cfg.PokeWalletAPIKey, 15*time.Second)

	var cmLegacy scraper.Source
	if cfg.FlareSolverrURL != "" {
		cmLegacy = cardmarket.NewWithFlareSolverr(20*time.Second, cfg.FlareSolverrURL)
	} else {
		cmLegacy = cardmarket.New(20 * time.Second)
	}
	tcgLegacy := tcgplayer.New(15 * time.Second)

	registry := scraper.NewRegistry()
	// Cardmarket: pokewallet primário (3 falhas → fallback), legado com 5 falhas → circuito abre.
	registry.Register(pricing.SourceCardmarket, pwCM, scraper.PrimarySource, 3)
	registry.Register(pricing.SourceCardmarket, cmLegacy, scraper.FallbackSource, 5)
	// TCGPlayer: pokewallet primário, legado mpapi como fallback.
	registry.Register(pricing.SourceTCGPlayer, pwTCG, scraper.PrimarySource, 3)
	registry.Register(pricing.SourceTCGPlayer, tcgLegacy, scraper.FallbackSource, 5)

	scrapers := []scraper.Source{
		ligapokemon.New(12 * time.Second),
		registry.ForSource(pricing.SourceCardmarket),
		registry.ForSource(pricing.SourceTCGPlayer),
		ebay.New(12 * time.Second),
	}

	// API v1
	r.Route("/api/v1", func(r chi.Router) {
		handler.NewAuthHandler(authSvc, tokenSvc, authMw, userRepo, handler.AuthHandlerConfig{
			FrontendBaseURL: cfg.FrontendBaseURL,
		}).Routes(r)

		handler.NewAdminHandler(userRepo, storeRepo, storeMemberRepo, storeAuditRepo, authMw, localUploads).Routes(r)

		handler.NewCardHandler(cardRepo, priceDailyRepo, signalSvc, authMw).Routes(r)

		handler.NewAdminCatalogHandler(cardRepo, localUploads, authMw, log.Logger).Routes(r)

		// Rotas de loja: leitura pública, escrita requer membro com stock_manager+.
		storeH := handler.NewStoreHandler(storeRepo, stockRepo, cardRepo, signalSvc, storeMemberRepo, storeAuditRepo, localUploads, userRepo)
		storeH.Routes(r)
		r.With(authMw.RequireAuth).Get("/stores/me", storeH.ListMyStores)
		r.With(authMw.RequireAuth).Get("/stores/{id}/my-role", storeH.GetMyRole)
		r.With(authMw.RequireStoreRole(user.StoreRoleViewer)).Get("/stores/{id}/members", storeH.StoreListMembers)
		r.With(authMw.RequireStoreRole(user.StoreRoleAdmin)).Post("/stores/{id}/members", storeH.StoreAddMember)
		r.With(authMw.RequireStoreRole(user.StoreRoleAdmin)).Delete("/stores/{id}/members/{userId}", storeH.StoreRemoveMember)
		r.With(authMw.RequireStoreRole(user.StoreRoleAdmin)).Patch("/stores/{id}/members/{userId}/role", storeH.StoreUpdateMemberRole)
		r.With(authMw.RequireStoreRole(user.StoreRoleAdmin)).
			Patch("/stores/{id}/profile", storeH.UpdateProfile)
		r.With(authMw.RequireStoreRole(user.StoreRoleAdmin)).
			Post("/stores/{id}/logo", storeH.UploadLogo)
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
