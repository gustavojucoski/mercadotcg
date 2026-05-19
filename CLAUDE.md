# MercadoTCG

> Documento vivo. Atualizar a cada decisão arquitetural e após cada merge.
> Referências detalhadas em [`docs/`](docs/): [ADRs](docs/adrs.md) · [API & Rotas](docs/api.md) · [Diretórios](docs/directory.md) · [Gaps & Pagamentos](docs/gaps.md)

## Visão

Marketplace e rastreador de preços de Pokémon TCG focado em **vendas reais** e **gestão de coleção** com rigor de variantes (Master Ball, Poke Ball Mirror, Holo, Reverse Holo, etc.). Multi-TCG por design.

## Stack

| Camada | Tecnologia |
|---|---|
| Backend | Go 1.25 (`chi`, `pgx/v5`, `shopspring/decimal`, `zerolog`, `golang-jwt/jwt/v5`, `resend-go/v2`) |
| Banco | PostgreSQL 16 (Docker local), migrations com `golang-migrate` |
| Frontend | Next.js 16 (App Router) + TypeScript + Tailwind CSS 4 |
| Auth | JWT (access em memória, refresh em `localStorage`), Google OAuth 2.0, bcrypt cost 12 |
| Scrapers | goquery (LigaPokemon), pokewallet.io (TCGPlayer+Cardmarket), Scrydex (eBay graded) |
| Storage | S3 (`mercadotcg-images-549803608550-sa-east-1-an`, `sa-east-1`) via `upload.Provider` |

## Agentes & Workflow

| Agente | `subagent_type` | Quando usar |
|---|---|---|
| Go Backend Engineer | `go-backend-engineer` | Qualquer código Go |
| Next.js Frontend Engineer | `nextjs-frontend-engineer` | Qualquer código frontend |
| Senior Software Architect | `senior-software-architect` | Decisões arquiteturais, pesquisa técnica, novos ADRs |
| Product Manager | `product-manager` | Requisitos, UX, riscos, priorização |
| QA / SDET Engineer | `qa-sdet-engineer` | Testes integrados, E2E, CI/CD |

**Workflow obrigatório:**
```
1. PM        → requisitos, escopo, critérios de aceite
2. Arquiteto → design, ADRs, plano para os agentes
3. Branch    → feat/ | fix/ | refactor/
4. Código    → agentes implementam com commits incrementais
5. Revisão técnica → Arquiteto revisa código gerado; se reprovado, engenheiro corrige
               e Arquiteto revalida — ciclo repete até aprovação explícita
6. QA        → qa-sdet-engineer revisa e propõe testes (Claude spawna automaticamente)
7. PR        → Pull Request da branch para main
8. Revisão   → Claude principal revisa
9. Merge     → squash merge
10. Docs     → atualizar CLAUDE.md (migrations, ADRs, fase, próximos passos)
```

**Regras absolutas:**
- Nenhum código sem PM → Arquiteto antes. Sem exceções.
- Todo código em branch. Nunca commitar em `main`.
- Arquiteto revisa TODO código gerado pelos engenheiros — obrigatório antes do QA.
- QA obrigatório após aprovação do Arquiteto — Claude spawna sem precisar o usuário solicitar.
- Docs obrigatórios após cada merge — Claude atualiza sem precisar o usuário solicitar.
- Leitura/análise pode ser feita inline pelo Claude principal.

## Status Atual

**Fase:** Auth completo · gestão de lojas · catálogo multi-TCG via **Scrydex** (primário, EN + sets JA) + TCGDex (complemento PT-BR) · bilíngue PT-BR/EN · catálogo público navegável (sets, cartas, autocomplete, sitemap) · imagens em S3 próprio · `import_source` rastreia origem de cada registro · Pokémon TCG Pocket separado como `tcg='pokemon-pocket'` · autocomplete suporta formato `1/217` e `/217` (PR #17) · `cards.number` removido, `collector_number` é a chave natural (PR #18) · **admin CRUD de sets/cartas/variantes completo** (PR #19) · **busca server-side + infinite scroll em `/sets/[tcg]`** (PR #22) · **busca externa de preços + URL `/cards/{code}/{number}`** (PR #23) · **página de busca geral `/search` com infinite scroll, filtros e ordenação** (PR #24).

### Migrations

| # | Conteúdo |
|---|---|
| 000001 | Schema completo inicial — todas as tabelas, ENUMs, índices, triggers e seed do admin (PR #20) |
| 000002 | `UNIQUE(code, language)` em `card_sets`; sufixo `_ja` removido dos codes japoneses (PR #23) |

> A partir do PR #20, migrations são incrementais: `000002_*.sql`, `000003_*.sql`, etc.

### Fontes de catálogo (PR #15 — ADRs 027–028)

| Fonte | Papel | Dados |
|---|---|---|
| **Scrydex** (`api.scrydex.io`) | Primário | Sets EN + JA, cartas, variantes, imagens |
| **TCGDex** (`api.tcgdex.net`) | Complemento PT-BR | `name_pt`, `image_url_pt` via `--enrich-pt` |

`cmd/import-catalog` flags: `--set`, `--series`, `--lang`, `--dry-run`, `--skip-images`, `--enrich-pt`, `--enrich-limit`.
`import_source` valores: `'scrydex'` · `'tcgdex_only'` · `'tcgdex_legacy'` · `'manual'` (protegido de sobrescrita).

## Próximos Passos

1. **Executar reimportação completa** — `import-catalog` com plano Scrydex Growth (50k créditos) para substituir os 208 sets legados TCGDex. Depois `--enrich-pt` para PT-BR.
2. **Remover remotePatterns de transição** — `assets.tcgdex.net`, `images.pokemontcg.io` e `images.scrydex.com` em `next.config.ts` após confirmar que tudo aponta para o S3 próprio.
3. **Frontend estoque de singles** — `/lojas/[id]/singles` com seleção de cartas via API de catálogo.
4. **Frontend estoque de selados** — `/lojas/[id]/selados`.
5. **Testes integrados** — `testcontainers-go`: StockRepo, PriceDailyRepo, card queries (import_source tests já adicionados no PR #15; ListSetsByTCG coberto no PR #22; SearchCards handler + repo cobertos no PR #24).
6. **Marketplace público** — listings + pagamentos (ver `docs/gaps.md`).

## Página de Busca Geral /search (PR #24 — mergeado 2026-05-19)

**Funcionalidade:** página `/search` com resultados paginados pelo servidor, infinite scroll, filtros e ordenação. O autocomplete do header redireciona para ela ao pressionar Enter ou clicar na lupa.

**Backend — `GET /search/cards`** (parâmetros: `q`, `sort`, `order`, `tcg`, `rarity`, `lang`, `page`, `limit`):
- `SearchCards` no repo: `COUNT(*) OVER()` em passagem única, ORDER BY via allowlist Go (sem interpolação livre), WHERE builder com `$N` posicionais.
- Ordenação: `name` (default), `release_date`, `collector_number` (cast numérico via `regexp_replace` — suporta `TG01`, `GG70`). Tie-breaker `c.id ASC` em todas as branches.
- Guard de profundidade: `(page-1)*limit > 1000` → `ErrSearchOffsetTooDeep` → HTTP 400.
- `escapeLikePattern` em `q` e `rarity`; truncagem de `q` com `[]rune` (max 80 chars).
- Cache-Control: `max-age=3600` sem filtros voláteis, `max-age=60` com `q` ou `rarity`.
- Response: `{data, total, page, limit, has_more}`. `data` sempre array (nunca `null`).

**Frontend:**
- `app/search/page.tsx` — RSC; SSR da primeira página via `API_INTERNAL`; Suspense boundary; `key` composta força remontagem ao navegar via GlobalSearch.
- `SearchResults` (`'use client'`) — debounce 300ms, `requestSeq` anti-race, IntersectionObserver, `router.replace` para URL sync, `robots: noindex`.
- `GlobalSearch` — Enter e clique na lupa navegam para `/search?q=`.
- `lib/rarities.ts` — `POKEMON_RARITIES` (20 raridades do Pokémon TCG).

**Testes:** 28 testes unitários de handler + 14 testes de integração de repo (skipam sem `DATABASE_URL`). Fix em `card_repo_import_source_test.go` (assinatura `GetSetByCode` quebrada após PR #23).

**Padrões estabelecidos:**
- `key` composta em RSC → Client Component força remontagem ao mudar `searchParams` via `router.push` — evita estado obsoleto.
- `fetchSearchResultsServer` (usa `API_INTERNAL`) separado de `fetchSearchResults` (usa `API_URL`) — mesmo padrão de `catalog.ts`.
- Novos domínios de imagem externos → sempre adicionar a `next.config.ts` `remotePatterns` (`images.scrydex.com` adicionado neste PR).

## Busca Externa de Preços + URL /cards/{code}/{number} (PR #23 — mergeado 2026-05-19)

**Funcionalidade:** botão "Buscar preços" na página de detalhe de carta abre modal com resultados em tempo real de LigaPokemon, TCGPlayer, Cardmarket e eBay.

**URL de carta reformulada:** `/cards/{code}/{number}?lan=` substitui `/cards/{slug}`. Set code e collector number são segmentos de path separados; language é query param (omitido quando `en`). Chi suporta hífens em segmentos de path nativamente (ex: `tcgp-PB`).

**Backend:**
- `GET /cards/{code}/{number}` — handler lê `chi.URLParam(r, "code")`, `chi.URLParam(r, "number")`, `parseLanguage(r)` (default `"en"`)
- SQL sempre recebe language: `WHERE cs.code = $1 AND cs.language = $2` — sem concatenação, queries indexadas
- `AutocompleteResult.slug` retorna `base1/001` (EN) ou `base1/001?lan=ja` (JA)
- LigaPokemon scraper: `numParamMatches` compara como inteiro (tolera zero-padding `num=013` == `"13"`); `findBestCardLink` com 4 níveis de prioridade (numAndEd > numOnly > edOnly > first); nome da carta como hint para fallback pokemontcg.io

**Migration 000002:** `DROP CONSTRAINT card_sets_code_key` → `ADD CONSTRAINT card_sets_code_lang_key UNIQUE (code, language)`. UPDATE remove sufixo `_ja` dos ~210 set codes japoneses. Aplicar com `migrate up` em deploy.

**Frontend:** `app/cards/[code]/[number]/page.tsx` (substituiu `[slug]`) lê `searchParams.lan`. `CardThumbnail`/`CardGridFilter` geram URLs com `?lan=ja` quando `setLanguage !== 'en'`. `SetCard` linka sets JA com `?lan=ja`. `TCGSet.language` adicionado ao tipo.

**Padrões estabelecidos:**
- URL de carta: `{code}/{number}` (sem hífen separando código do número) — set codes podem conter hífen (ex: `tcgp-PB`).
- Language sempre passada explicitamente ao SQL, default `"en"` — nunca concatenar `code + "_" + language`.
- `schema_migrations` do golang-migrate tem apenas UMA linha (versão atual) — nunca inserir múltiplas linhas manualmente.

## Busca Server-Side + Infinite Scroll em /sets/[tcg] (PR #22 — mergeado 2026-05-18)

**Problema corrigido:** sets vintage (Base Set, Fossil, Neo) ficavam invisíveis — listagem carregava 200 sets em `release_date DESC` e filtrava no client-side.

**Backend:** parâmetro `q` opcional no `GET /sets/{tcg}` público. ILIKE em `code` e `name` com `ESCAPE '\'` explícito. `escapeLikePattern` (handler) escapa `%`, `_`, `\` antes de passar ao repo. Truncagem de `q` em runes (UTF-8 safe, max 80). Cache-Control `max-age=3600` sem `q`, `max-age=60` com `q`.

**Frontend:** `SetBrowser` (`'use client'`) substitui `SetFilter`. SSR da primeira página (`limit=24`) via `searchParams.q`. Debounce 300ms → busca no backend. IntersectionObserver + botão "Carregar mais". URL sync via `router.replace`. `requestSeq` ref para descartar respostas obsoletas. `robots: noindex` quando `q` presente.

**Padrões estabelecidos:**
- `ESCAPE '\'` obrigatório em toda cláusula ILIKE que aceita input do usuário.
- `escapeLikePattern` no handler, nunca no repo (separação de responsabilidades).
- Client Components que fazem fetch usam `API_URL` (não `API_INTERNAL`).
- Truncagem de strings com `[]rune(s)[:n]`, nunca `s[:n]` em bytes.

## Admin Catálogo (PR #19 — mergeado 2026-05-18) + Séries (PR #21 — mergeado 2026-05-18)

20 endpoints `platform_admin` para CRUD de sets, cartas, variantes e séries.

**Endpoints sets/cartas/variantes (PR #19):** `GET/POST /admin/sets` · `GET/PATCH/DELETE /admin/sets/{id}` · `POST /admin/sets/{id}/image` · `GET/POST /admin/cards` · `GET/PATCH/DELETE /admin/cards/{id}` · `POST /admin/cards/{id}/image` · `GET /admin/cards/{id}/variants` · `POST /admin/cards/{cardId}/variants` · `PATCH/DELETE /admin/variants/{id}`

**Endpoints séries (PR #21):** `GET /admin/series` · `POST /admin/series` · `PATCH /admin/series/{id}` · `DELETE /admin/series/{id}`

**Notas PR #21:**
- `PATCH /admin/series/{id}/name-pt` removido — substituído pelo PATCH genérico.
- `tcg` é imutável após criação (PATCH com `tcg` retorna 400).
- Delete bloqueado via `ErrDeleteBlocked{Sets: n}` quando há `card_sets` vinculados, executado dentro de `WithTx`.
- `card_series` não tem `import_source` — não estampar `manual` nessa entidade.

**Padrões estabelecidos:**
- Delete atômico com `ErrDeleteBlocked` sentinel dentro de `WithTx` (conta dependências na mesma tx).
- PATCH dinâmico: campos `nil` não são sobrescritos; no-op guard retorna early.
- `import_source='manual'` estampado em sets/cards/variants editados via admin (ADR-028); não se aplica a `card_series`.
- `lib/config.ts` centraliza `API_URL` (client) e `API_INTERNAL` (RSC/server) — único lugar a mudar.
- `next.config.ts` proxia `/api/*` e `/uploads/*` via `API_INTERNAL_URL` (Docker interno), permitindo ngrok com um único túnel.

## Convenções Críticas

**Go:**
- `shopspring/decimal` em todo valor monetário — nunca float.
- Erros embrulhados com `fmt.Errorf("...: %w", err)`. Sentinelas exportados.
- `pgx/v5 + ENUMs Postgres`: cast explícito obrigatório no SQL (`$n::nome_do_enum`).
- `CITEXT + pg_trgm`: cast `::text` obrigatório antes de `%` ou `similarity()`.
- `cmd/migrate`, `cmd/seed`, `cmd/import-catalog`: leem `DATABASE_URL` direto — **não usam `config.Load()`**.

**Schema:**
- Nunca editar migration já aplicada. Toda alteração de ENUM = nova migration.
- `collector_number` armazena só o número (`"274"`), nunca `"274/217"`. Display composto no frontend.
- URL de carta: `/cards/{code}/{number}?lan=` — set code e collector number como segmentos separados. Set codes **podem** conter hífen (ex: `tcgp-PB`); collector numbers nunca.

**Frontend:**
- App Router. Server component por padrão; `"use client"` só com estado/eventos/hooks.
- Guard de auth centralizado em `app/admin/layout.tsx` — páginas filhas não duplicam.
- Bilíngue: `<LocalizedText en={...} pt={...} />` ou `t(en, pt)` de `lib/locale.ts`. SEO sempre em EN.
- URL da API centralizada em `lib/config.ts` (`API_URL` para client-side, `API_INTERNAL` para RSC). Nunca redefinir localmente nos arquivos.
- `NEXT_PUBLIC_API_URL=` (vazio) em `.env.local` e docker-compose — browser usa paths relativos proxiados pelo Next.js.
- `next.config.ts` tem `allowedDevOrigins` para `*.ngrok-free.*` (acesso remoto em dev).

**Upload:**
- Sempre via `upload.Provider` interface. `NewFromEnv()` seleciona local vs S3.
- S3 usa bucket policy (não ACL por objeto). `Put` bufferiza body via `io.ReadAll`.
- Ao adicionar fonte de imagens externas, adicionar domínio em `next.config.ts`.
