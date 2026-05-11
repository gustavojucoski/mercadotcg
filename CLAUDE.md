# MercadoTCG — Memória do Projeto

> Documento vivo. Atualizar a cada decisão arquitetural relevante e ao final de cada fase de trabalho.

## 0. Agentes Especializados e Workflow de Desenvolvimento

Este projeto possui agentes especializados. **Sempre delegue a tarefa ao agente correto antes de escrever qualquer código.** Use o agente via ferramenta `Agent` com o `subagent_type` correspondente.

| Agente | `subagent_type` | Quando usar |
|---|---|---|
| Go Backend Engineer | `go-backend-engineer` | Qualquer código Go: handlers, repositórios, serviços, scrapers, migrations, testes de integração Go |
| Next.js Frontend Engineer | `nextjs-frontend-engineer` | Qualquer código frontend: pages, components, lib/, hooks, estilos Tailwind, otimizações de performance |
| Senior Software Architect | `senior-software-architect` | Decisões arquiteturais, novos ADRs, avaliação de tecnologias, design de módulos novos, **qualquer pesquisa técnica** |
| Product Manager | `product-manager` | Priorização de backlog, definição de MVP, análise de ROI de features, especificação de requisitos |
| QA / SDET Engineer | `qa-sdet-engineer` | Testes integrados com testcontainers, E2E com Playwright, revisão de cobertura, CI/CD |

---

### Workflow obrigatório para qualquer alteração de código

```
1. PM          → define requisitos, escopo, critérios de aceite
2. Arquiteto   → design da solução, ADRs se necessário, plano para os agentes de código
3. Branch      → criar branch com nome descritivo (feat/, fix/, refactor/)
4. Código      → agentes de backend e/ou frontend implementam na branch, fazendo commits incrementais
5. QA          → qa-sdet-engineer revisa o que foi implementado e escreve/propõe testes
6. PR          → abrir Pull Request da branch para main
7. Revisão     → Claude principal revisa a PR antes de aprovar
8. Merge       → squash merge para main após aprovação
```

**Regras absolutas:**
- **Nenhuma linha de código é escrita sem passar por PM → Arquiteto antes.** Não há exceções, nem para correções pequenas.
- **Todo código vai para uma branch.** Nunca commitar diretamente em `main`.
- **Toda pesquisa técnica** (bibliotecas, APIs externas, padrões, viabilidade) é feita pelo `senior-software-architect`.
- **QA é obrigatório** após cada entrega dos agentes de código. O Claude principal spawna o QA automaticamente, sem precisar o usuário solicitar.
- Tarefas puramente de **leitura/análise** (explicar código, responder perguntas) podem ser feitas inline pelo Claude principal.
- Para mudanças que tocam **ambas as camadas** (backend + frontend), spawne os agentes de código em paralelo após o Arquiteto definir o plano.
- Se um agente precisar de contexto de sessão anterior, inclua no prompt os tipos, interfaces e convenções relevantes.

### Workflow de revisão de feature ou decisão (quando solicitado)

Quando o usuário pedir para "revisar", "analisar" ou "avaliar" algo:
1. Spawnar `product-manager` — análise de ROI, riscos de produto, gaps de UX.
2. Com o output do PM, spawnar `senior-software-architect` — ajustes arquiteturais, validação das decisões de design.
3. Apresentar os dois outputs consolidados ao usuário.

## 1. Visão

Marketplace e rastreador de preços de Pokémon TCG, focado em **vendas reais** (na própria plataforma e via scraping de fontes externas) e em **gestão de coleção** com rigor de variantes (Master Ball, Poke Ball Mirror, Holo, Reverse Holo, etc.).

Diferenciais perseguidos:

- Histórico de preços com profundidade temporal real (séries diárias, anos de dados).
- Variantes tratadas como cidadãos de primeira classe — não como atributo solto.
- Conversão cambial preservando auditabilidade (preço original + preço BRL + cotação usada na ingestão).
- Backend de alta performance preparado para milhões de observações.

## 2. Stack

| Camada | Tecnologia |
|---|---|
| Backend | Go 1.25 (`chi`, `pgx/v5`, `shopspring/decimal`, `zerolog`, `golang-jwt/jwt/v5`, `resend-go/v2`) |
| Banco | PostgreSQL 16 (local via Docker), migrations com `golang-migrate` |
| Frontend | Next.js 16 (App Router) + TypeScript + Tailwind CSS 4 |
| Auth | JWT (access token em memória, refresh token em `localStorage`), Google OAuth 2.0, email/senha com bcrypt cost 12 |
| Email | Resend (`resend-go/v2`), `NoopProvider` quando sem chave |
| Scrapers | Go: `goquery` (LigaPokemon), pokewallet.io API (TCGPlayer+Cardmarket), Scrydex (eBay graded) |

## 3. Estrutura de Diretórios

```
MercadoTCG/
├── CLAUDE.md
├── backend/
│   ├── go.mod                    # go 1.25
│   ├── .env / .env.example
│   ├── Dockerfile                # multi-stage: golang:1.25-alpine builder + alpine:3.20 runtime
│   ├── docker-compose.yml        # db, adminer, migrate, seed, api, import-catalog(profile)
│   ├── cmd/
│   │   ├── api/            # servidor HTTP principal
│   │   ├── migrate/        # CLI: up | down [N] | version | force <v>  — lê DATABASE_URL direto
│   │   ├── seed/           # popula demo data — lê DATABASE_URL direto (sem config.Load)
│   │   └── import-catalog/ # importa catálogo da pokemontcg.io API
│   ├── internal/
│   │   ├── domain/
│   │   │   ├── card/       # Set, Card, Variant, Finish enum
│   │   │   ├── pricing/    # Observation, DailyPoint, Condition, Source enums
│   │   │   ├── store/      # Store (+DocumentType/Status), StockItem, StockMovement
│   │   │   ├── user/       # User, PlatformRole, StoreRole, StoreRoleLevel()
│   │   │   ├── listing/    # Listing (futuro marketplace)
│   │   │   └── matching/   # ExternalCardRef
│   │   ├── auth/
│   │   │   ├── password.go   # HashPassword / CheckPassword (bcrypt cost 12)
│   │   │   ├── token.go      # TokenService: IssueAccessToken, ParseAccessToken, GenerateRefreshToken
│   │   │   ├── oauth.go      # OAuthService: AuthCodeURL, ValidateState, Exchange→GoogleProfile
│   │   │   ├── service.go    # AuthService: Register, Login, GoogleCallback, ForgotPW, ResetPW, Refresh, Logout
│   │   │   └── middleware.go # RequireAuth, RequirePlatformAdmin, RequireStoreRole, UserFromCtx
│   │   ├── config/
│   │   │   └── config.go   # Load() com fail-fast em JWT_SECRET; todos os demais opcionais
│   │   ├── email/
│   │   │   ├── email.go      # Provider interface + Message type
│   │   │   ├── resend.go     # ResendProvider wrapping resend-go/v2
│   │   │   ├── noop.go       # NoopProvider (loga no stdout)
│   │   │   └── templates.go  # HTML inline: verificação, boas-vindas, reset de senha
│   │   ├── handler/
│   │   │   ├── auth.go       # POST register/login/google/verify-email/forgot-pw/reset-pw/refresh/logout; GET me
│   │   │   ├── admin.go      # RequirePlatformAdmin: CRUD users, CRUD stores + doc validation, membros
│   │   │   ├── store.go      # GET stock, POST purchase/sale (requer StoreRole)
│   │   │   ├── card.go       # GET search, GET lookup
│   │   │   ├── external.go   # GET external-search (RequirePlatformAdmin)
│   │   │   └── helpers.go    # writeJSON, writeErr, decodeJSON, parseUUID, atoiOrDefault
│   │   ├── repository/postgres/
│   │   │   ├── db.go               # Connect, sentinelas ErrNotFound/ErrAlreadyExists
│   │   │   ├── card_repo.go
│   │   │   ├── price_history_repo.go
│   │   │   ├── price_daily_repo.go
│   │   │   ├── forex_repo.go
│   │   │   ├── external_ref_repo.go
│   │   │   ├── store_repo.go       # CRUD + List/Update/SetDocumentVerified
│   │   │   ├── stock_repo.go       # RegisterPurchase/Sale/Adjustment com SELECT FOR UPDATE
│   │   │   ├── user_repo.go        # Create/GetByID/GetByEmail/UpdateRole/MarkVerified/List
│   │   │   ├── token_repo.go       # UseVerificationToken / UsePasswordResetToken (atômico UPDATE RETURNING)
│   │   │   └── store_member_repo.go # GetMembership/AddMember/RemoveMember/ListMembers
│   │   ├── service/
│   │   │   ├── pricing/    # NormalizeBRL, FillObservation
│   │   │   ├── pricesignal/ # For(variantID, condition), ByConditions(variantID, window)
│   │   │   └── document/   # ValidateCNPJ, ValidateCPF (checksum), LookupCNPJ (ReceitaWS)
│   │   ├── scraper/
│   │   │   ├── scraper.go      # interface Source, Query, Result, ErrNotConfigured, MeasureSearch
│   │   │   ├── ligapokemon/    # scraping HTML via goquery
│   │   │   ├── pokewallet/     # tcgSource + cmSource compartilham Client+cache (1 req HTTP por carta)
│   │   │   ├── ebay/           # Scrydex scraper (graded sales)
│   │   │   ├── tcgplayer/      # legado — não registrado no main.go
│   │   │   └── cardmarket/     # legado (FlareSolverr) — não registrado no main.go
│   │   ├── pokemontcgio/   # client pokemontcg.io: CardInfo com TCGPlayer/Cardmarket prices
│   │   └── forex/          # BCBProvider (PTAX OData), Service com cache+fallback 7 dias
│   └── migrations/
│       ├── 000001 — extensions (pgcrypto, citext, pg_trgm)
│       ├── 000002 — card_sets, cards, card_variants, ENUM variant_finish
│       ├── 000003 — price_history (particionada), price_daily, ENUMs pricing, trigger updated_at
│       ├── 000004 — forex_rates, listings, ENUM listing_status
│       ├── 000005 — stores, stock_items, stock_movements, external_card_refs, ENUM stock_movement_kind
│       ├── 000006 — users, user_oauth_providers, email_verification_tokens, password_reset_tokens,
│       │            refresh_tokens, store_members; FKs listings.seller_id + stores.owner_id ativadas
│       ├── 000007 — seed admin: gustavojucoski@gmail.com / ewq9brd5gan2dzf@FZD (bcrypt via pgcrypto)
│       └── 000008 — stores + document_type/document_number/document_status/legal_name/verified_*
└── frontend/
    ├── package.json / next.config.ts / tailwind.config.ts
    ├── proxy.ts                 # Next.js middleware: pass-through, sem proteção de rota
    ├── app/
    │   ├── layout.tsx           # AuthProvider wrapping global
    │   ├── page.tsx             # Homepage pública — SiteHeader + hero + feature cards
    │   ├── admin/
    │   │   ├── layout.tsx       # Auth guard (redireciona não-admin) + SiteHeader
    │   │   ├── page.tsx         # Busca Externa (SearchForm + resultados) — sem header próprio
    │   │   └── lojas/
    │   │       ├── page.tsx     # Listagem de lojas com badges de status
    │   │       └── nova/
    │   │           └── page.tsx # Formulário de cadastro CNPJ/CPF + lookup ReceitaWS
    │   └── auth/
    │       ├── login/page.tsx          # Email+senha; admin → /admin, user → /
    │       ├── register/page.tsx       # Cadastro público (usuários comuns)
    │       ├── verify-email/page.tsx   # Lê ?token=, POST /auth/verify-email
    │       ├── callback/page.tsx       # Google OAuth callback: lê ?access_token=&refresh_token=
    │       ├── forgot-password/page.tsx
    │       └── reset-password/page.tsx # Lê ?token=
    ├── components/
    │   ├── AuthProvider.tsx   # contexto {user, loading, clearAuth, refresh}; hidrata token no mount
    │   ├── SiteHeader.tsx     # Logo + nav inline (Busca Externa, Lojas — só para platform_admin) + UserMenu
    │   ├── UserMenu.tsx       # Dropdown: unauthenticated → Entrar▾; authenticated → nome+avatar, Minha conta, Sair
    │   ├── SearchForm.tsx     # SetCombobox + número da carta
    │   ├── SetCombobox.tsx    # combobox filtrável de sets (pokemontcg.io)
    │   ├── CardInfo.tsx
    │   ├── PriceMatrix.tsx    # tabela condição × fonte
    │   ├── GradedSection.tsx  # vendas eBay gradeadas
    │   ├── SourceCard.tsx     # accordion por fonte
    │   └── ConditionBadge.tsx
    └── lib/
        ├── types.ts           # tipos espelhando respostas do backend
        ├── api.ts             # authedFetch (retry 401→refresh), searchCard()
        ├── auth.ts            # login, register, logout, refreshAccessToken, fetchCurrentUser
        ├── sets.ts            # useSets() — cache em memória
        └── stores-admin.ts    # listStores, createStore, lookupCNPJ, verifyDocument
```

## 4. Decisões de Arquitetura (ADR-style enxuto)

### ADR-001 — SQL puro versionado em vez de Prisma/ORM
**Decisão:** Migrations escritas à mão em SQL e aplicadas com `golang-migrate`.
**Razão:** Precisamos de particionamento por range, índices BRIN, ENUMs nativos do Postgres e GIN com `pg_trgm`. Prisma e ORMs escondem ou complicam esses recursos. Auditabilidade de schema vale mais que ergonomia.

### ADR-002 — `shopspring/decimal` para todo valor monetário
**Decisão:** Nenhum `float32`/`float64` em campo de preço, em qualquer camada Go.
**Razão:** Erros de arredondamento em `float` são inaceitáveis em marketplace. `decimal.Decimal` mapeia naturalmente para `NUMERIC(14,2)` no Postgres.

### ADR-003 — Variantes como tabela própria (`card_variants`)
**Decisão:** A formação de preço, listings e price_history apontam para `card_variants.id`, não para `cards.id`.
**Razão:** A diferença Master Ball Mirror × Poke Ball Mirror × Holo × Reverse Holo é o cerne do produto. Uma carta com 4 variantes tem 4 séries históricas distintas; misturá-las destrói o sinal.

### ADR-004 — Particionamento de `price_history` por trimestre
**Decisão:** `price_history` é `PARTITION BY RANGE (observed_at)`, com partições trimestrais criadas previamente.
**Razão:** Volume esperado em dezenas de milhões de linhas; trimestres permitem `DROP TABLE` rápido em retenção e index pruning automático em queries com filtro temporal.
**Trade-off aceito:** PK precisa incluir `observed_at` (composta). UNIQUE de deduplicação também.

### ADR-005 — Tabela quente `price_daily` separada do raw
**Decisão:** Pré-agregamos diariamente min/avg/max/median/p25/p75 de vendas e listings em `price_daily`. Gráficos no front sempre leem dela.
**Razão:** Calcular percentis em runtime sobre milhões de linhas mata latência. `price_daily` cabe inteira em cache.

### ADR-006 — Auditoria cambial dupla
**Decisão:** `price_history` armazena `price_original + currency` **e** `price_brl + fx_rate_used`. Convertemos uma única vez na ingestão e nunca recalculamos.
**Razão:** Preserva auditabilidade ("vendeu por ¥4.500 quando o iene estava a R$ 0,034"). Recalcular a posteriori reescreveria o passado quando a cotação atual mudasse.

### ADR-007 — BRIN em `observed_at`
**Decisão:** Em vez de só BTree, usamos BRIN para varreduras por janela temporal.
**Razão:** Em séries temporais inseridas em ordem aproximada, BRIN é ~1000× menor que BTree e suficiente para filtros de range. Mantemos BTree composto `(variant_id, observed_at DESC)` para o caso "última observação por variante".

### ADR-008 — Multi-tenant desde a fundação
**Decisão:** `stores` existe desde o primeiro dia, com `owner_id` em toda store e `store_id` em todo `stock_item`. Não há "loja default" hard-coded.
**Razão:** Refatorar para multi-tenant depois exige reescrever todas as queries de leitura. O custo extra agora é uma coluna FK; o custo depois seria migrar dados em produção.

### ADR-009 — Estoque agregado + log de movimentos
**Decisão:** `stock_items` tem uma linha por (store, variant, condition, language, grade) com `quantity` cumulativa e `cost_avg_brl` (média ponderada). Toda alteração gera linha em `stock_movements` (append-only).
**Razão:** Leitura quente ("quanto eu tenho?") consulta uma linha. Contabilidade ("qual minha margem?") ainda é exata via log. Custo médio ponderado é suficiente para o MVP; FIFO pode ser derivado do log se necessário.
**Trade-off aceito:** Cartas gradeadas (PSA, Beckett) com cert numbers únicos não têm rastreio individual hoje.

### ADR-010 — Matching strict via `external_card_refs`
**Decisão:** Toda observação vinda de scraper só vira `price_history` se já existir uma linha em `external_card_refs` para `(source, external_id)`. Sem match → quarentena (staging futura).
**Razão:** Misturar IDs ambíguos polui a série temporal e destrói o sinal. Preferimos perder amplitude (deixar de ingerir) a perder precisão (ingerir errado).
**Trade-off aceito:** Bootstrap precisa de matching manual ou semi-automático para popular as primeiras refs.

### ADR-011 — Estratégia por fonte: scraping vs API
**Decisão:** **LigaPokemon** = scraping HTML via goquery. **TCGPlayer + Cardmarket** = pokewallet.io API (`internal/scraper/pokewallet/`). **eBay** = Scrydex (graded sales, sem credenciais). Scrapers legados `tcgplayer/` e `cardmarket/` mantidos no código mas não registrados no `main.go`.
**Trade-off aceito:** eBay via Scrydex só tem dados gradeados (PSA/BGS/CGC/ACE/TAG). pokewallet.io free tier: 100 req/hora.

### ADR-012 — TCGPlayer product ID (SUPERSEDIDO pelo ADR-015)
Supersedida. O scraper `internal/scraper/tcgplayer/` ainda existe mas não é registrado.

### ADR-013 — Preços TCGPlayer por condição via multiplicadores
**Decisão:** Preço base NM → LP=80%, MP=64%, HP=40%, DMG=24%.
**Trade-off aceito:** São estimativas, não preços reais de listagens por condição.

### ADR-014 — Cardmarket: multiplicadores por condição
**Decisão:** Preço base `low` NM → LP=70%, MP=45%, HP=25%, DMG=10%.

### ADR-015 — pokewallet.io como fonte primária para TCGPlayer + Cardmarket
**Decisão:** `New(apiKey, timeout)` devolve dois `scraper.Source` que compartilham um único `*Client` + `requestCache` (TTL 60s). Chamadas paralelas do fan-out geram apenas uma requisição HTTP por carta. `pickBestMatch`: 3 passes — set code+number exatos > set name+number > number only. Sem credencial → `ErrNotConfigured`.

### ADR-016 — Auth próprio: JWT + Google OAuth + email/senha
**Decisão:** Tabela `users` própria no Postgres. Access token JWT (HS256, 15 min, payload: sub/email/platform_role) em memória no frontend. Refresh token (SHA-256 hash armazenado em `refresh_tokens`) em `localStorage` (`mtcg_rt`). Google OAuth com state HMAC (10 min). bcrypt cost 12 para senhas. Email via Resend.
**Razão:** Supabase Auth foi descartado — queremos controle total do schema (RBAC de loja, FK constraints, audit trail). JWT em memória + refresh em localStorage é o equilíbrio segurança/UX padrão para SPAs.
**Trade-off aceito:** Refresh token em localStorage é vulnerável a XSS. Mitigação: access token curto (15 min), HTTPS em produção, CSP header.

### ADR-017 — Criação de lojas: apenas platform_admin
**Decisão:** Lojas são conveniadas — não há self-service. Toda criação passa por `POST /api/v1/admin/stores` (requer `RequirePlatformAdmin`). O admin informa `owner_id`, e o backend automaticamente adiciona o owner em `store_members` com `role=admin`.
**Razão:** Controle de qualidade das lojas na plataforma; evita spam/fraude no cadastro.

### ADR-018 — Validação de documento: CNPJ auto-verified, CPF manual
**Decisão:** CNPJ → checksum Receita Federal + consulta ReceitaWS (free, 3 req/min). Se `situacao == "ATIVA"` → `document_status = auto_verified` com `legal_name` preenchido. Se rate limit/erro de rede → `pending`. CPF → checksum apenas → sempre `pending`. Revisão manual via `POST /admin/stores/{id}/verify-document`.
**Documento armazenado:** apenas dígitos (`VARCHAR(14)`), sem máscara. Mascaramento é responsabilidade do frontend.
**Razão:** ReceitaWS é gratuito e suficiente para o MVP. CPF de pessoa física não tem API pública de situação cadastral, então aprovação manual é obrigatória.
**Gotcha pgx/v5:** ENUMs Postgres (`document_type`, `document_status`) precisam de cast explícito no SQL (`$n::document_type`) — pgx não faz cast automático de `string` para ENUM customizado.

### ADR-020 — Registro em duas etapas: email-first
**Decisão:** `POST /auth/register` aceita apenas `email`. O usuário recebe um link de verificação; ao clicar, a página `/auth/verify-email` exibe um formulário para definir nome e senha. `POST /auth/verify-email` recebe `{ token, password, display_name }`, completa o cadastro (`CompleteRegistration`: password_hash + display_name + email_verified_at em um único UPDATE) e devolve tokens de sessão (auto-login).
**Razão:** Reduz abandono no cadastro (menor fricção na entrada) e garante que apenas emails válidos chegam à etapa de criação de senha.
**Reenvio:** Se o mesmo email tentar se registrar novamente e a conta ainda não estiver verificada, o backend reenvia o link em vez de retornar 409. Conta já verificada → 409.
**Dev:** O `verify_url` é sempre logado no stdout da API (`[dev] link de verificação`) para facilitar testes sem depender de entrega de email.
**Email prod:** Requer domínio verificado no Resend. Sem domínio verificado, `onboarding@resend.dev` só entrega para o email do dono da conta Resend.

### ADR-019 — Navegação via hover dropdowns no SiteHeader
**Decisão:** "Minha Loja" e "Admin" são botões com hover dropdown no `SiteHeader`. "Admin" expande para "Busca Externa" e "Lojas". "Minha Loja" expande para as abas da loja corrente (Perfil, Membros, Selados, Singles) quando o pathname está em `/lojas/{id}/*`, ou para "Ir para minha loja" caso contrário. O `UserMenu` serve apenas para ações do usuário (conta, logout).
**Razão:** Dropdowns contextuais permitem navegar entre páginas da loja e retornar ao app global sem depender do botão voltar do browser.
**Implementação:** `useState<string | null>` no SiteHeader para controlar qual dropdown está aberto; `onMouseEnter`/`onMouseLeave` nos wrappers. Pathname extraído via `usePathname()` para detectar `currentLojaId` com regex `/^\/lojas\/([^/]+)/`.
**Layout admin:** `app/admin/layout.tsx` é o guard — redireciona não-admins para `/auth/login`. As páginas filhas não têm guard próprio nem header duplicado.
**Layout loja:** `app/lojas/[id]/layout.tsx` exibe barra com logo, nome, role e abas de navegação. Contém seta "←" que leva ao Início.

## 5. Status Atual

**Fase:** Auth completo + gestão de lojas com validação de documento.

### Migrations (000001–000010)

| # | Conteúdo |
|---|---|
| 000001 | Extensions: `pgcrypto`, `citext`, `pg_trgm` |
| 000002 | `card_sets`, `cards`, `card_variants`, ENUM `variant_finish` |
| 000003 | `price_history` (particionada por trimestre), `price_daily`, ENUMs de pricing, trigger `updated_at` |
| 000004 | `forex_rates`, `listings`, ENUM `listing_status` |
| 000005 | `stores`, `stock_items`, `stock_movements`, `external_card_refs`, ENUM `stock_movement_kind` |
| 000006 | `users`, `user_oauth_providers`, `email_verification_tokens`, `password_reset_tokens`, `refresh_tokens`, `store_members`; FKs `listings.seller_id` e `stores.owner_id` ativadas |
| 000007 | Seed admin: `gustavojucoski@gmail.com` / `ewq9brd5gan2dzf@FZD`, `platform_admin`, email verificado |
| 000008 | `stores` + colunas `document_type`, `document_number`, `document_status`, `legal_name`, `document_verified_at`, `document_verified_by`; índice unique parcial |
| 000009 | `stores` + colunas de endereço (address_zip … address_country) separadas em migration dedicada |
| 000010 | `store_audit_log` — id, store_id, changed_by, change_type, changes (JSONB), created_at; índice em (store_id, created_at DESC) |

### Endpoints HTTP disponíveis

```
GET  /healthz

# Auth (público)
POST /api/v1/auth/register          # body: { email } — envia link de verificação; se email já existe e não verificado, reenvia
POST /api/v1/auth/login
GET  /api/v1/auth/google
GET  /api/v1/auth/google/callback
POST /api/v1/auth/verify-email      # body: { token, password, display_name } — completa cadastro + retorna tokens (auto-login)
POST /api/v1/auth/forgot-password
POST /api/v1/auth/reset-password
POST /api/v1/auth/refresh
POST /api/v1/auth/logout
GET  /api/v1/auth/me               (RequireAuth)

# Catálogo (público)
GET  /api/v1/cards/search
GET  /api/v1/cards/lookup

# Busca externa (RequirePlatformAdmin)
GET  /api/v1/external-search?number=&set=

# Admin — usuários (RequirePlatformAdmin)
GET    /api/v1/admin/users
GET    /api/v1/admin/users/search?q=
POST   /api/v1/admin/users
PATCH  /api/v1/admin/users/{id}/role
DELETE /api/v1/admin/users/{id}

# Admin — lojas (RequirePlatformAdmin)
GET    /api/v1/admin/stores                      # lista paginada
POST   /api/v1/admin/stores                      # cria loja + valida CNPJ/CPF
GET    /api/v1/admin/stores/cnpj-lookup?cnpj=    # proxy ReceitaWS (rota literal ANTES de {id})
GET    /api/v1/admin/stores/{id}
PATCH  /api/v1/admin/stores/{id}                 # aceita owner_id para trocar proprietário
POST   /api/v1/admin/stores/{id}/verify-document # aprovação manual de documento
POST   /api/v1/admin/stores/{id}/logo
GET    /api/v1/admin/stores/{id}/audit-log
POST   /api/v1/admin/stores/{id}/members
PATCH  /api/v1/admin/stores/{id}/members/{userId}/role
DELETE /api/v1/admin/stores/{id}/members/{userId}
GET    /api/v1/admin/stores/{id}/members

# Lojas — acesso do próprio membro (RequireAuth / RequireStoreRole)
GET    /api/v1/stores/{id}                              # público
GET    /api/v1/stores/me                                # lojas do usuário autenticado
GET    /api/v1/stores/{id}/my-role                      # role do caller nesta loja
GET    /api/v1/stores/{id}/members                      # (viewer+)
POST   /api/v1/stores/{id}/members                      # (admin)
PATCH  /api/v1/stores/{id}/members/{userId}/role        # (admin)
DELETE /api/v1/stores/{id}/members/{userId}             # (admin)
PATCH  /api/v1/stores/{id}/profile                      # edição restrita: nome, endereço (admin)
POST   /api/v1/stores/{id}/logo                         # (admin)
GET    /api/v1/stores/{id}/stock
POST   /api/v1/stores/{id}/stock/purchase               # (stock_manager+)
POST   /api/v1/stores/{id}/stock-items/{itemID}/sale    # (stock_manager+)

# Variantes
GET  /api/v1/variants/{id}/signal
```

### Dados de demonstração (cmd/seed)

- Set SV8 (Surging Sparks), 3 cartas, 7 variantes
- Loja "Mercado do Gus" (owner: gustavojucoski@gmail.com)
- 1890 linhas em `price_daily` (3 condições × 3 fontes × 30 dias × 7 variantes)
- Forex: 1 USD = R$ 5,40

### Frontend (rotas)

| Rota | Acesso | Conteúdo |
|---|---|---|
| `/` | público | Homepage com hero, feature cards, SiteHeader |
| `/auth/login` | público | Email+senha; admin→/admin, user→/ |
| `/auth/register` | público | Cadastro email-only (step 1) |
| `/auth/verify-email?token=` | público | Define nome + senha após confirmar email (step 2, auto-login) |
| `/auth/callback?access_token=&refresh_token=` | público | Google OAuth redirect |
| `/auth/forgot-password` | público | Solicita reset |
| `/auth/reset-password?token=` | público | Nova senha |
| `/admin` | platform_admin | Busca Externa com SearchForm |
| `/admin/lojas` | platform_admin | Lista de lojas com status de documento |
| `/admin/lojas/nova` | platform_admin | Formulário de cadastro CNPJ/CPF |
| `/admin/lojas/[id]` | platform_admin | Edição completa da loja + log de auditoria |
| `/lojas/me` | autenticado | Redireciona para a loja do usuário |
| `/lojas/[id]/perfil` | membro da loja | Edição restrita (nome, logo, endereço) |
| `/lojas/[id]/membros` | membro da loja | Gestão de membros (admin: adicionar/remover/alterar role) |
| `/lojas/[id]/selados` | membro da loja | Placeholder — estoque de selados |
| `/lojas/[id]/singles` | membro da loja | Placeholder — estoque de singles (multi-TCG futuro) |

## 6. Próximos Passos (priorizados)

1. **Job de agregação diária** — `cmd/aggregate` (ou cron) chama `PriceDailyRepo.RebuildDay(today)`. Sem isso o `pricesignal` fica com dados do seed e nunca atualiza com preços reais dos scrapers.
2. **Matching service** — `internal/service/matching`: dada uma observação raw (title, set, number) tenta achar variant_id e cria automaticamente o `external_card_ref`. Hoje os refs são inseridos manualmente pelo seed.
3. **Pipeline scraping → price_history** — ligar os scrapers ao storage. Hoje `external-search` só devolve ao caller, não persiste nada.
4. **Frontend estoque de singles** — `/lojas/[id]/singles` com cadastro de cards. Suporte multi-TCG desde o início: o formulário de seleção de carta não pode ser Pokémon-only; modelar de forma genérica (TCG + set + número/ID de carta).
5. **Frontend estoque de selados** — `/lojas/[id]/selados` com cadastro de produtos selados (booster box, ETB, etc.).
6. **Testes integrados** — `tests/integration` com `testcontainers-go` para repos críticos: `StockRepo` (transações, custo médio ponderado), `PriceDailyRepo.RebuildDay`, `forex.Service` com fake Provider.
7. **Marketplace público** — listings, reservas e checkout. Entra integração de pagamentos (seção 9).

## 7. Convenções

- **Agentes**: toda tarefa que escreve ou modifica código deve ser delegada ao agente especializado (ver Seção 0). Claude principal coordena e revisa; agentes executam.
- **Go**: pacotes em `lowercase`, exportados em `CamelCase`. Sem stutter (`card.Card`). Erros sempre embrulhados com `fmt.Errorf("...: %w", err)`. Sentinelas exportados (`ErrNotFound`, `ErrAlreadyExists`, `ErrInsufficientStock`).
- **SQL**: snake_case. ENUMs no plural natural. Toda alteração de ENUM = nova migration. Nunca editar migration já aplicada.
- **pgx/v5 + ENUMs customizados**: sempre usar cast explícito no SQL (`$n::nome_do_enum`). pgx não faz auto-cast de `string` para ENUM Postgres.
- **Frontend**: App Router. Server component por padrão; `"use client"` só quando há estado/eventos/hooks. Auth guard centralizado em `app/admin/layout.tsx` — páginas filhas não duplicam o guard. `NEXT_PUBLIC_API_URL` via `.env.local`.
- **Docker**: `cmd/migrate` e `cmd/seed` leem `DATABASE_URL` diretamente — **não usam `config.Load()`** (que exigiria JWT_SECRET e outras vars de auth desnecessárias nesses binários).
- **Multi-TCG**: o sistema deve suportar qualquer TCG (não só Pokémon). Ao implementar qualquer feature de catálogo de cartas ou estoque de singles, modelar de forma agnóstica ao TCG (campo `tcg` ou similar). Não assumir Pokémon como padrão hard-coded.

## 8. O que NÃO está pronto

- Não há job criando partições futuras de `price_history`. Hardcoded até 2026-Q4.
- `PriceHistoryRepo.InsertBatch` (CopyFrom) não respeita ON CONFLICT — deduplicação é responsabilidade do pipeline a montante.
- `ListingRepo` não existe — virá com o marketplace.
- `forex.BCBProvider` usa `decimal.NewFromFloat` no parsing — ok para BCB (4 casas), mas trocar para `decimal.NewFromString` se mudar de fonte.
- Cartas gradeadas: `stock_items` agrega por `grade` mas não distingue dois "PSA 10" com cert numbers diferentes (ver ADR-009).
- Reservas de estoque (`reservation`/`release`): ENUMs declarados em `stock_movement_kind` mas código não os emite ainda.
- Pipeline de scraping → matching → `price_history` não existe. Scraper retorna ao caller mas não persiste.
- Emails de transação funcionam apenas com `RESEND_API_KEY` configurada; sem a chave usa `NoopProvider` (loga no stdout, não envia). O `verify_url` é sempre logado no stdout mesmo quando Resend está ativo — útil em dev. Em produção, exige domínio verificado no Resend (`EMAIL_FROM_ADDRESS` deve usar esse domínio); `onboarding@resend.dev` só entrega para o email do dono da conta.
- Google OAuth requer `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `GOOGLE_REDIRECT_URL` e `OAUTH_STATE_HMAC_KEY` configurados.

## 9. Integração de Pagamentos (planejada)

### 9.1 Escolha do PSP

- **Mercado Pago** default — PIX, cartão, boleto, dominante no BR.
- **Stripe** momento 2 — compradores internacionais.
- Não usar múltiplos PSPs simultâneos no MVP.

### 9.2 Modelo de dados (a criar)

- `payment_intents` — id, listing_id, buyer_id, amount_brl, psp, psp_payment_id, status, idempotency_key.
- `payment_events` — log append-only de webhooks (payload JSONB).
- `payouts` — seller_id, amount_brl, fee_brl, psp_transfer_id, status, paid_at.

### 9.3 Regras críticas

- Idempotência em webhooks (PSP pode reenviar).
- Escrow lógico: liberar payout só após confirmação de recebimento.
- `decimal.Decimal` em todo valor monetário — nunca float.
- Fees registradas separadas do bruto.
- Validar HMAC do PSP antes de qualquer mutação.

### 9.4 Código

- `internal/payment/` — interface `Provider`, implementações `mercadopago.go` e `stripe.go`.
- `internal/handler/payment_webhook.go` — `POST /webhooks/payments/{psp}`.
