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

**Fase:** Auth completo · gestão de lojas · catálogo multi-TCG via **Scrydex** (primário, EN + sets JA) + TCGDex (complemento PT-BR) · bilíngue PT-BR/EN · catálogo público navegável (sets, cartas, autocomplete, sitemap) · imagens em S3 próprio · `import_source` rastreia origem de cada registro · Pokémon TCG Pocket separado como `tcg='pokemon-pocket'` · autocomplete suporta formato `1/217` e `/217` (PR #17) · `cards.number` removido, `collector_number` é a chave natural (PR #18).

### Migrations

| # | Conteúdo |
|---|---|
| 000001 | Extensions: `pgcrypto`, `citext`, `pg_trgm` |
| 000002 | `card_sets`, `cards`, `card_variants`, ENUM `variant_finish` |
| 000003 | `price_history` (particionada por trimestre), `price_daily`, ENUMs pricing, trigger `updated_at` |
| 000004 | `forex_rates`, `listings`, ENUM `listing_status` |
| 000005 | `stores`, `stock_items`, `stock_movements`, `external_card_refs`, ENUM `stock_movement_kind` |
| 000006 | `users`, oauth providers, email/password tokens, `refresh_tokens`, `store_members` |
| 000007 | Seed admin: `gustavojucoski@gmail.com` / `ewq9brd5gan2dzf@FZD` |
| 000008 | `stores` + `document_type/number/status`, `legal_name`, `verified_*` |
| 000009 | `stores` + colunas de endereço |
| 000010 | `store_audit_log` (JSONB) |
| 000012 | `card_sets.tcg VARCHAR(32)` + CHECK constraint (ADR-021) |
| 000013 | `card_series` (entidade própria); `series_id FK` em `card_sets`; `collector_number` e `name_pt` em `cards` (ADR-022) |
| 000014 | Limpa `collector_number` — remove `"/217"` → mantém só o número |
| 000015 | `card_sets.printed_total INTEGER` — para autocomplete de formato `"110/217"` |
| 000016 | `card_sets.symbol_url`; `tcg='pocket'` no CHECK; índices GIN para autocomplete bilíngue |
| 000017 | `cards.image_url_pt` — imagem PT-BR (TCG Pocket via TCGDex) |
| 000019 | `card_sets.import_source`, `cards.import_source` VARCHAR(32) DEFAULT `'tcgdex_legacy'` (ADR-028) |
| 000020 | CHECK constraint `'pocket'` → `'pokemon-pocket'`; migra 19 sets `tcgp-*` de `tcg='pokemon'` para `tcg='pokemon-pocket'`; recria séries Pocket com novo valor (PR #16) |
| 000021 | Dropa `cards.number` (legado); adiciona `UNIQUE (set_id, collector_number)` como nova chave natural (PR #18) |

### Fontes de catálogo (PR #15 — ADRs 027–028)

| Fonte | Papel | Dados |
|---|---|---|
| **Scrydex** (`api.scrydex.io`) | Primário | Sets EN + JA, cartas, variantes, imagens |
| **TCGDex** (`api.tcgdex.net`) | Complemento PT-BR | `name_pt`, `image_url_pt` via `--enrich-pt` |

`cmd/import-catalog` flags: `--set`, `--series`, `--lang`, `--dry-run`, `--skip-images`, `--enrich-pt`, `--enrich-limit`.
`import_source` valores: `'scrydex'` · `'tcgdex_only'` · `'tcgdex_legacy'` · `'manual'` (protegido de sobrescrita).

## Próximos Passos

1. **Executar reimportação completa** — `import-catalog` com plano Scrydex Growth (50k créditos) para substituir os 208 sets legados TCGDex. Depois `--enrich-pt` para PT-BR.
2. **Remover remotePatterns de transição** — `assets.tcgdex.net` e `images.pokemontcg.io` em `next.config.ts` após confirmar que tudo aponta para o S3 próprio.
3. **Frontend estoque de singles** — `/lojas/[id]/singles` com seleção de cartas via API de catálogo.
4. **Frontend estoque de selados** — `/lojas/[id]/selados`.
5. **Testes integrados** — `testcontainers-go`: StockRepo, PriceDailyRepo, card queries (import_source tests já adicionados no PR #15).
6. **Marketplace público** — listings + pagamentos (ver `docs/gaps.md`).

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
- Slug de carta: `{setCode}-{collectorNumber}`. Set codes **nunca** devem conter hífen.

**Frontend:**
- App Router. Server component por padrão; `"use client"` só com estado/eventos/hooks.
- Guard de auth centralizado em `app/admin/layout.tsx` — páginas filhas não duplicam.
- Bilíngue: `<LocalizedText en={...} pt={...} />` ou `t(en, pt)` de `lib/locale.ts`. SEO sempre em EN.
- `NEXT_PUBLIC_API_URL` via `.env.local`.

**Upload:**
- Sempre via `upload.Provider` interface. `NewFromEnv()` seleciona local vs S3.
- S3 usa bucket policy (não ACL por objeto). `Put` bufferiza body via `io.ReadAll`.
- Ao adicionar fonte de imagens externas, adicionar domínio em `next.config.ts`.
