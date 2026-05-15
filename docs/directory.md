# Estrutura de Diretórios — MercadoTCG

```
MercadoTCG/
├── CLAUDE.md
├── docs/                     # referências detalhadas
├── backend/
│   ├── go.mod                # go 1.25
│   ├── .env / .env.example
│   ├── Dockerfile            # multi-stage: golang:1.25-alpine + alpine:3.20
│   ├── docker-compose.yml    # db, adminer, migrate, seed, api, import-catalog
│   ├── cmd/
│   │   ├── api/              # servidor HTTP principal
│   │   ├── migrate/          # CLI: up | down [N] | version | force <v>
│   │   ├── seed/             # stub — admin criado pela migration 000007
│   │   └── import-catalog/   # TCGDex API; flags: --set, --series, --recent, --download-images
│   ├── internal/
│   │   ├── domain/
│   │   │   ├── card/         # Series, Set, Card, Variant, Finish enum
│   │   │   ├── pricing/      # Observation, DailyPoint, Condition, Source enums
│   │   │   ├── store/        # Store, StockItem, StockMovement
│   │   │   ├── user/         # User, PlatformRole, StoreRole
│   │   │   ├── listing/      # Listing (futuro marketplace)
│   │   │   └── matching/     # ExternalCardRef
│   │   ├── auth/
│   │   │   ├── password.go   # HashPassword / CheckPassword (bcrypt cost 12)
│   │   │   ├── token.go      # TokenService: Issue/Parse/Generate
│   │   │   ├── oauth.go      # OAuthService: Google OAuth 2.0
│   │   │   ├── service.go    # AuthService: Register, Login, GoogleCallback, ForgotPW...
│   │   │   └── middleware.go # RequireAuth, RequirePlatformAdmin, RequireStoreRole
│   │   ├── config/config.go  # Load() — fail-fast em JWT_SECRET; demais opcionais
│   │   ├── email/            # Provider interface; ResendProvider; NoopProvider; templates HTML
│   │   ├── handler/
│   │   │   ├── auth.go
│   │   │   ├── admin.go      # RequirePlatformAdmin: users, stores, membros
│   │   │   ├── store.go      # stock: purchase/sale
│   │   │   ├── card.go       # search, lookup
│   │   │   ├── external.go   # external-search
│   │   │   └── helpers.go    # writeJSON, writeErr, decodeJSON, parseUUID
│   │   ├── repository/postgres/
│   │   │   ├── db.go               # Connect; ErrNotFound/ErrAlreadyExists
│   │   │   ├── card_repo.go        # UpsertSeries/Set/Card/Variant; UpdateImageURL/NamePT
│   │   │   ├── price_history_repo.go
│   │   │   ├── price_daily_repo.go
│   │   │   ├── forex_repo.go
│   │   │   ├── external_ref_repo.go
│   │   │   ├── store_repo.go
│   │   │   ├── stock_repo.go       # SELECT FOR UPDATE
│   │   │   ├── user_repo.go
│   │   │   ├── token_repo.go       # atômico UPDATE RETURNING
│   │   │   └── store_member_repo.go
│   │   ├── service/
│   │   │   ├── pricing/     # NormalizeBRL, FillObservation
│   │   │   ├── pricesignal/ # For(variantID, condition)
│   │   │   └── document/    # ValidateCNPJ, ValidateCPF, LookupCNPJ (ReceitaWS)
│   │   ├── scraper/
│   │   │   ├── scraper.go      # interface Source, Query, Result, ErrNotConfigured
│   │   │   ├── ligapokemon/    # scraping HTML via goquery
│   │   │   ├── pokewallet/     # TCGPlayer + Cardmarket (compartilham Client+cache)
│   │   │   ├── ebay/           # Scrydex (graded sales)
│   │   │   ├── tcgplayer/      # legado — não registrado
│   │   │   └── cardmarket/     # legado — não registrado
│   │   ├── tcgdex/         # client TCGDex; rate limit 1 req/s; EnrichSet/EnrichCard bilíngue
│   │   ├── pokemontcgio/   # FindCard (preços), ListSets/Cards — external-search + fallback logos
│   │   ├── upload/
│   │   │   ├── upload.go   # Provider interface + LocalProvider + NewFromEnv
│   │   │   └── s3.go       # S3Provider (aws-sdk-go-v2)
│   │   └── forex/          # BCBProvider (PTAX OData), cache+fallback 7 dias
│   └── migrations/         # 000001–000017 (ver tabela no CLAUDE.md)
└── frontend/
    ├── next.config.ts / tailwind.config.ts
    ├── middleware.ts         # pass-through, sem proteção de rota
    ├── app/
    │   ├── layout.tsx        # LocaleProvider + AuthProvider; lang="pt-BR"
    │   ├── page.tsx          # Homepage
    │   ├── admin/            # layout.tsx = auth guard (redireciona não-admin)
    │   ├── auth/             # login, register, verify-email, callback, forgot/reset-password
    │   ├── sets/             # hub → [tcg] → [code]
    │   ├── cards/[slug]/     # detalhe da carta
    │   └── lojas/[id]/       # layout.tsx com abas; perfil, membros, selados, singles
    ├── components/
    │   ├── AuthProvider.tsx  # {user, loading, clearAuth, refresh}
    │   ├── LocaleProvider.tsx / LangToggle.tsx / LocalizedText.tsx
    │   ├── SiteHeader.tsx    # Logo + GlobalSearch + LangToggle + UserMenu + dropdowns
    │   ├── GlobalSearch.tsx  # debounce 300ms, ARIA combobox
    │   ├── SetFilter.tsx / SetCard.tsx / CardGridFilter.tsx / CardThumbnail.tsx
    │   ├── VariantTabs.tsx / PriceMatrix.tsx / GradedSection.tsx
    │   └── SearchForm.tsx / SetCombobox.tsx (admin)
    └── lib/
        ├── types.ts          # tipos espelhando respostas do backend
        ├── catalog.ts        # fetchSeries/Sets/Set/Cards/Card, autocompleteCards
        ├── api.ts            # authedFetch (retry 401→refresh)
        ├── auth.ts           # login, register, logout, refresh, fetchCurrentUser
        ├── locale.ts         # useLang(), t(en, pt)
        └── stores-admin.ts   # listStores, createStore, lookupCNPJ, verifyDocument
```
