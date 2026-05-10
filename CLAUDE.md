# MercadoTCG — Memória do Projeto

> Documento vivo. Atualizar a cada decisão arquitetural relevante e ao final de cada fase de trabalho.

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
| Backend | Go 1.22 (`chi`, `pgx/v5`, `shopspring/decimal`, `zerolog`) |
| Banco | PostgreSQL via Supabase, migrations com `golang-migrate` |
| Frontend | Next.js 16 (App Router) + TypeScript + Tailwind CSS 4 |
| Scrapers | Go: `goquery` (LigaPokemon/eBay), pokewallet.io API (TCGPlayer+Cardmarket) |

## 3. Estrutura de Diretórios

```
MercadoTCG/
├── CLAUDE.md
├── backend/
│   ├── go.mod
│   ├── .env.example
│   ├── cmd/
│   │   ├── api/        # servidor HTTP (entrypoint principal)
│   │   ├── scraper/    # workers de coleta por fonte
│   │   └── migrate/    # CLI de migrations
│   ├── internal/
│   │   ├── domain/     # tipos puros (card, pricing, listing, user)
│   │   ├── repository/ # acesso a dados (postgres)
│   │   ├── service/    # regras de negócio
│   │   ├── handler/    # adaptadores HTTP
│   │   ├── scraper/    # implementações por fonte (ligapokemon, ebay, pokewallet)
│   │   ├── forex/      # cotação BRL/USD/JPY/EUR
│   │   ├── config/     # carregamento de env
│   │   └── middleware/
│   ├── pkg/            # utilidades reutilizáveis (decimal helpers, logger)
│   ├── migrations/     # SQL versionado (.up/.down)
│   ├── scripts/
│   └── tests/integration/
└── frontend/
    ├── package.json
    ├── next.config.ts
    ├── tsconfig.json
    ├── tailwind.config.ts
    ├── app/
    │   ├── layout.tsx
    │   ├── page.tsx        # search form + results
    │   └── globals.css
    ├── components/
    │   ├── SearchForm.tsx  # SetCombobox + number input
    │   ├── SetCombobox.tsx # combobox filtrável de sets
    │   ├── CardInfo.tsx
    │   ├── PriceMatrix.tsx # tabela condição × fonte
    │   ├── GradedSection.tsx
    │   ├── SourceCard.tsx  # accordion por fonte
    │   └── ConditionBadge.tsx
    └── lib/
        ├── types.ts        # tipos espelhando resposta do backend
        ├── api.ts          # searchCard() → /api/v1/external-search
        └── sets.ts         # useSets() — pokemontcg.io
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
**Trade-off aceito:** Cartas gradeadas (PSA, Beckett) com cert numbers únicos não têm rastreio individual hoje. Quando virarmos esse caso, adicionamos `cert_number` em `stock_movements` ou separamos `graded_items`.

### ADR-010 — Matching strict via `external_card_refs`
**Decisão:** Toda observação vinda de scraper só vira `price_history` se já existir uma linha em `external_card_refs` para `(source, external_id)`. Sem match → quarentena (staging futura).
**Razão:** Misturar IDs ambíguos polui a série temporal e destrói o sinal. Preferimos perder amplitude (deixar de ingerir) a perder precisão (ingerir errado).
**Trade-off aceito:** Bootstrap precisa de matching manual ou semi-automático para popular as primeiras refs. Confidence < 100 sinaliza matches que precisam de revisão.

### ADR-011 — Estratégia por fonte: scraping vs API
**Decisão:** **LigaPokemon** = scraping HTML via goquery (sem API pública). **TCGPlayer + Cardmarket** = API pokewallet.io (`internal/scraper/pokewallet/`) — uma chamada por carta retorna preços das duas plataformas (ver ADR-015). **eBay** = vendas recentes via Scrydex (`scrydex.com/pokemon/cards/x/{card-id}`), HTML com atributos `data-*` — sem credenciais, usa pokemontcg.io card ID como `ExternalID`. O scraper HTML do Cardmarket (`internal/scraper/cardmarket/`) ainda existe no código (com lógica de FlareSolverr e V-number retry) mas não é mais registrado no `cmd/api/main.go`; mantido por referência caso pokewallet.io fique indisponível.
**Razão:** Scrydex agrega vendas reais do eBay para cartas gradeadas em HTML server-rendered, sem autenticação. O scraper Cardmarket via FlareSolverr produzia mapeamentos errados de cartas e requeria Chrome headless pesado (~13s por chamada); a pokewallet.io API é mais confiável e cobre ambas as fontes com uma única chamada HTTP.
**Trade-off aceito:** eBay via Scrydex só tem dados de cartas **gradeadas** (PSA/BGS/CGC/ACE/TAG). pokewallet.io não filtra por idioma de card (inglês, português, espanhol etc.) — todos os idiomas disponíveis aparecem misturados. Free tier: 100 req/hora, 1.000/dia.

### ADR-012 — Resolução de product ID do TCGPlayer (SUPERSEDIDO pelo ADR-015)
**Decisão (histórica):** Resolvia o product ID via redirect encadeado `prices.pokemontcg.io → tcgplayer.pxf.io → product/{id}`, com fallback via Scrydex. Ambas as estratégias eram best-effort com timeout de 5s.
**Status:** Supersedida pelo ADR-015 (pokewallet.io). O scraper `internal/scraper/tcgplayer/` ainda existe mas não é registrado no main.go.

### ADR-013 — Preços TCGPlayer por condição via multiplicadores
**Decisão:** O preço base NM (da pokewallet.io `market_price` ou do scraper direto) gera LP/MP/HP/DMG aplicando multiplicadores padrão: NM=100%, LP=80%, MP=64%, HP=40%, DMG=24%.
**Razão:** A API pública do TCGPlayer não tem endpoint de preços por condição sem credenciais. Os multiplicadores espelham os mesmos que o TCGPlayer usa internamente.
**Trade-off aceito:** São preços estimados, não preços de listagens reais por condição. Aceitável para MVP de referência de preço.

### ADR-014 — Cardmarket: multiplicadores por condição
**Decisão:** Preços Cardmarket são derivados aplicando multiplicadores sobre o preço base `low` (menor preço por variante retornado pela pokewallet.io): NM=100%, LP=70%, MP=45%, HP=25%, DMG=10%.
**Razão:** Cardmarket não expõe preços por condição diretamente via API. Os multiplicadores correspondem aos descontos praticados no mercado europeu.
**Trade-off aceito:** São estimativas baseadas no menor preço disponível de cada variante (normal/holo/etc.), não preços reais de listings por condição.

### ADR-015 — pokewallet.io como fonte primária para TCGPlayer + Cardmarket
**Decisão:** O pacote `internal/scraper/pokewallet/` implementa `scraper.Source` duas vezes — `tcgSource` e `cmSource` — ambas retornadas por `New(apiKey, timeout)`. As duas wrappers compartilham um único `*Client` com HTTP client e `requestCache`, de forma que chamadas paralelas do fan-out no handler geram apenas uma requisição HTTP por carta. Busca via `GET /search?limit=20&q={name}+{number}`; seleção da carta certa via `pickBestMatch` com três passes de prioridade: (1) set code + number exatos, (2) set name contém + number (lida com prefixo de idioma "ME: Ascended Heroes"), (3) number only. TCGPlayer: usa o sub-tipo com maior `market_price`; Cardmarket: itera todas as variantes de preço e usa `low` como base NM. Cache de 60s por chave `SetCode/Number`.
**Razão:** Substitui dois scrapers problemáticos (TCGPlayer via endpoint não-documentado, Cardmarket via FlareSolverr) por uma API oficial com cobertura confiável das duas fontes. Elimina o serviço FlareSolverr (container Chrome headless) do docker-compose.
**Trade-off aceito:** Sem filtragem por idioma de card — pokewallet.io não expõe esse campo no endpoint de busca; todos os idiomas disponíveis aparecem como variantes. Free tier: 100 req/hora. Sem credencial (`POKEWALLET_API_KEY` vazia), as duas fontes retornam `ErrNotConfigured` graciosamente.

## 5. Status Atual

**Fase:** MVP de busca de preços concluído — frontend operacional.

Concluído na sessão de bootstrap (2026-05-09):

- Estrutura de diretórios do backend Go.
- `go.mod` declarado (chi, pgx/v5, decimal, zerolog, migrate, goquery, uuid, godotenv, testify).
- Migrations 000001–000004:
  - 000001 — extensions (`pgcrypto`, `citext`, `pg_trgm`).
  - 000002 — `card_sets`, `cards`, `card_variants` + ENUM `variant_finish`.
  - 000003 — `price_history` (particionada) + `price_daily` + ENUMs de pricing + trigger `updated_at`.
  - 000004 — `forex_rates` + `listings` + ENUM `listing_status`.
- Domain types Go: `card.Set`, `card.Card`, `card.Variant`, `pricing.Observation`, `pricing.DailyPoint`.
- `config.Load()` com validação fail-fast.
- `cmd/api/main.go` — esqueleto chi com healthz, CORS e graceful shutdown.

Adicionado em 2026-05-09 (fase 1):

- `internal/repository/postgres/db.go` — `Connect(ctx, url) → *pgxpool.Pool` com timeouts conservadores; sentinelas `ErrNotFound` e `ErrAlreadyExists`; constante `PgUniqueViolation = "23505"`.
- `CardRepo` — `CreateSet`, `GetSetByCode`, `CreateCard`, `GetCardByID`, `SearchCardsByName` (pg_trgm), `CreateVariant`, `ListVariantsByCard`. Detecta colisão UNIQUE → `ErrAlreadyExists`.
- `PriceHistoryRepo` — `Insert` (avulso), `InsertBatch` via `pgx.CopyFrom` para ingestão em escala, `LatestForVariant` e `ObservationsInRange`.
- `PriceDailyRepo` — `Upsert`, `SeriesByVariant` e `RebuildDay` (agregação completa do dia direto no servidor com `PERCENTILE_CONT`, evitando trazer milhões de linhas para o cliente).
- `ForexRepo` — `Upsert` idempotente e `LatestOnOrBefore(currency, day)` para resolver fim de semana/feriado caindo no dia útil anterior.
- `cmd/migrate` — CLI `up | down [N] | version | force <v>` sobre `golang-migrate` v4 (driver postgres + source file).

Adicionado em 2026-05-09 (fase 2, câmbio + normalização):

- `internal/forex/forex.go` — tipo `Quote` e contrato `Provider` (permite trocar BCB por outra fonte sem mexer em service).
- `internal/forex/bcb.go` — `BCBProvider` consultando o endpoint OData PTAX do BCB; prefere o boletim "Fechamento PTAX" quando há múltiplos no dia; `cotacaoVenda` é a referência usada.
- `internal/forex/service.go` — `Service.Quote(ctx, currency, day)` com cascata cache → DB → provider, e fallback automático de até 7 dias para trás (cobre fim de semana/feriado). BRL é identidade (rate=1) sem I/O.
- `internal/service/pricing/pricing.go` — `Service.NormalizeBRL(price, currency, observedAt) → (priceBRL, fxRate, quotedAt, source)` arredondando a 2 casas. Helper `FillObservation` deixa uma `pricing.Observation` pronta para `price_history`.

Adicionado em 2026-05-09 (fase 5, busca AO VIVO + catálogo):

- `internal/scraper/scraper.go` — interface `Source` (Name, Search), tipos `Query`, `Result`, `SourceResult`. Sentinel `ErrNotConfigured` para fontes sem credencial. Helper `MeasureSearch` empacota duração+erro de cada fonte.
- `internal/scraper/ligapokemon/` — scraper HTML com goquery. **Best-effort**: seletores CSS escritos sem acesso ao HTML real do site, vai precisar iteração na primeira run. Defensivo: cada parse de produto falha silenciosamente sem quebrar a busca. Parsing brasileiro de preço (R$ 1.298,40) tratado em `parseBRLPrice`.
- `internal/scraper/tcgplayer/` — endpoint não-documentado `mpapi.tcgplayer.com/v2/product/{id}/pricepoints`. Sem ExternalID (product ID) → lista vazia sem erro. Gera 1 resultado por condição (NM/LP/MP/HP/DMG) via multiplicadores sobre o `marketPrice` NM.
- `internal/scraper/ebay/` — eBay graded sales via Scrydex. Sem credenciais. `ExternalID` = pokemontcg.io card ID. Retorna vendas recentes agrupadas por company+grade (ex: "PSA 10"), com o menor preço por grupo.
- Handler `GET /external-search` — aceita **apenas** `number` + `set` (ambos obrigatórios). Fan-out paralelo via goroutines + WaitGroup, timeout independente por fonte (12s). Resposta inclui `duration_ms` e `error` por fonte.
- `cmd/import-catalog` — importador da Pokemon TCG API (https://pokemontcg.io). Paginação automática, idempotente. Heurística de variantes: cria `normal` sempre; adiciona `holo` ou `reverse_holo` se a raridade indicar. Suporta `--set <code>` e `--recent <N>` para imports parciais.
- Config ganhou `TCGPlayerPublicKey/PrivateKey`, `EbayClientID/ClientSecret`, `PokemonTCGAPIKey` (todas opcionais). `.env.example` documenta como obter.
- docker-compose: serviço `import-catalog` sob profile `catalog` — não sobe em `up` normal, dispara via `docker compose --profile catalog run --rm import-catalog`.

Adicionado em 2026-05-10 (Cardmarket ao vivo + FlareSolverr + per-condition fallback + set listing resolver):

- `internal/scraper/cardmarket/cardmarket.go` — scraper Cardmarket via HTML + FlareSolverr. `New(timeout)` = direto (403 do Cloudflare → vazio sem erro). `NewWithFlareSolverr(timeout, url)` = Chrome headless via FlareSolverr para bypass. Seletores CSS confirmados contra HTML real: `div.article-row` (linhas) → `a.article-condition span.badge` (texto NM/EX/GD/LP/PO via `data-original-title`/`title`/text) → `div.price-container span.color-primary` (preço EUR europeu `1.234,56 €`). Retorna `cheapestPerCondition`. `resolveFromSetListing(ctx, cardName, setName)`: quando `ExternalID` está vazio e FlareSolverr está configurado, busca a página do set no Cardmarket (`/Singles/{SetSlug}`) e extrai a URL da carta procurando links cujo slug começa com o slug do nome da carta — permite encontrar cartas de sets novos não mapeados pelo pokemontcg.io (ex: ASC 290 Mega Dragonite ex). `toCardmarketSlug`: converte nome de carta/set para o slug do Cardmarket (espaços → hífens, apóstrofos/& removidos, acentos normalizados).
- `internal/scraper/scraper.go` — `Query` ganhou campo `SetName string` (nome do set em inglês, ex: "Ascended Heroes") usado pelo Cardmarket para `resolveFromSetListing`.
- `internal/handler/external.go` — passa `info.CardmarketURL` como ExternalID e `info.SetName` em `baseQuery.SetName`. Fallback: quando scraper retorna vazio + pokemontcg.io tem `trendPrice`, injeta 5 preços por condição com multiplicadores sobre trendPrice. Suporte a per-source timeout via `WithSourceTimeout(SourceCardmarket, d)`.
- `cmd/api/main.go` — branch: se `FLARESOLVERR_URL` configurado, usa `NewWithFlareSolverr(90s)` e `WithSourceTimeout(SourceCardmarket, 90s)`; senão usa `New(12s)`.
- `docker-compose.yml` — serviço `flaresolverr` (ghcr.io/flaresolverr/flaresolverr:latest) na porta 8191. api depende de `flaresolverr: service_started` e recebe `FLARESOLVERR_URL=http://flaresolverr:8191`.
- `internal/config/config.go` e `.env.example` — `FLARESOLVERR_URL` adicionado (opcional).

Adicionado em 2026-05-10 (Cardmarket SIR URL resolution + helpers):

- `internal/scraper/cardmarket/cardmarket.go` — `resolveTarget` substitui a chamada direta a `resolveFromSetListing`. Para SIR cards (number > printedTotal) constrói a URL diretamente: `{SetSlug}/{CardSlug}-{SetCode}{Number}` sem prefixo V (ex: `Ascended-Heroes/Pikachu-ex-ASC276`), economizando uma chamada ao FlareSolverr. Para cartas regulares delega ao `resolveFromSetListing` que usa `slugContainsNumber` para match exato. `fetchViaFlareSolverr` agora trata HTTP 404 e 503 como lista vazia (sem erro), permitindo que URLs construídas para cards ainda não indexados no CM falhem graciosamente. Helpers internos: `parseCardNumber`, `slugContainsNumber`, `extractVNumber`, `pickByVNumber`, `findSlugOrigCase`.
- `internal/scraper/cardmarket/helpers_test.go` — testes unitários para todos os helpers (package interno). Todos PASS sem FlareSolverr.
- `internal/scraper/cardmarket/cardmarket_test.go` — `TestSearch_PikachuExSIR`: testa a construção automática da URL para Pikachu ex SIR (276/217) do set ASC, confirmando preços reais do CM (~€550 NM). Testes de debug adicionados: `TestDebugSetListing`, `TestDebugSetListingSearch`, `TestDebugVNumbersInHTML` (todos requerem `FLARESOLVERR_URL`).
- `internal/scraper/scraper.go` — `Query` ganhou `SetPrintedTotal int` (total de cartas base do set, usado para distinguir SIR de regular no resolver Cardmarket).
- `internal/pokemontcgio/client.go` — `CardInfo` ganhou `SetPrintedTotal int`, populado de `set.printedTotal` da pokemontcg.io API.
- `internal/handler/external.go` — passa `info.SetPrintedTotal` em `baseQuery.SetPrintedTotal`.

Adicionado em 2026-05-10 (refinamento do external-search + eBay via Scrydex):

- `internal/pokemontcgio/client.go` — `CardInfo` ganhou `CardmarketURL` e `CardmarketPrices *CardmarketPriceRange` (preços EUR: `averageSellPrice`, `lowPrice`, `trendPrice`, `avg1/7/30`). Query agora inclui `select=...,cardmarket`. Resolução de product ID do TCGPlayer ganhou fallback via Scrydex: `GET scrydex.com/pokemon/cards/{id}/purchase?variant=holofoil` → redirect → extrai product ID (cobre cards sem `tcgplayer.url` no pokemontcg.io, como cards novos do set ASC).
- `internal/handler/external.go` — endpoint requer obrigatoriamente `number` + `set` (removido parâmetro `name`). Adicionada função `cardmarketResultsFromCatalog` que injeta preços Cardmarket em EUR como fonte sintética quando pokemontcg.io retorna dados. Fallback TCGPlayer via preços do catálogo mantido para cards com `tcgplayer.prices` mas sem product ID. Passa pokemontcg.io card ID como `ExternalID` para o scraper eBay.
- `internal/scraper/tcgplayer/tcgplayer.go` — removida abordagem OAuth. Agora usa `mpapi.tcgplayer.com/v2/product/{id}/pricepoints` sem autenticação. Gera 5 resultados por tipo de impressão (NM/LP/MP/HP/DMG) aplicando multiplicadores sobre o `marketPrice` NM: LP=80%, MP=64%, HP=40%, DMG=24%.
- `internal/scraper/ligapokemon/ligapokemon.go` — `Search` agora aplica `cheapestPerCondition`: agrupa resultados por condição e devolve apenas o menor preço por condição (máx. 5 resultados — um por NM/LP/MP/HP/DMG).
- `internal/scraper/ebay/ebay.go` — **reescrito** usando Scrydex como fonte. Sem credenciais. Busca `scrydex.com/pokemon/cards/x/{card-id}` (slug ignorado, só o ID importa). Parseia vendas recentes do eBay via regex sobre atributos `data-company`, `data-grade`, `data-price`, `data-currency`, `data-sold-at`. Agrupa por company+grade e devolve o menor preço por grupo. Ordena por company (PSA→BGS→CGC→ACE→TAG) e grade decrescente. `Kind = KindSale` (vendas reais, não listagens).
- `internal/domain/pricing/pricing.go` — adicionada função `ConditionFromTCG(s string) Condition` para normalizar strings de condição.

Adicionado em 2026-05-10 (pokewallet.io + frontend Next.js):

- `internal/scraper/pokewallet/pokewallet.go` — substitui scrapers TCGPlayer e Cardmarket. `New(apiKey, timeout)` devolve dois `scraper.Source` (`tcgSource` + `cmSource`) que compartilham um único HTTP client e `requestCache` (deduplicação de chamadas paralelas, TTL 60s). Busca por `{name} {number}` em `GET /search?limit=20`. `pickBestMatch`: 3 passes — set code + number exato > set name contém + number > number only. TCGPlayer: maior `market_price` → 5 condições via multiplicadores (NM/LP/MP/HP/DMG 100%/80%/64%/40%/24%). Cardmarket: itera todas as variantes, usa `low` como NM, 5 condições via multiplicadores (NM/LP/MP/HP/DMG 100%/70%/45%/25%/10%). Sem credencial → `ErrNotConfigured`.
- `backend/internal/scraper/cardmarket/cardmarket.go` — adicionado retry V1–V10: quando nenhum artigo é encontrado na URL principal, testa `{slug}-V1-{SETCODE}{num}`, `V2...`, até V10. Helper `injectVersion` insere o sufixo `-V{n}` antes do código de set no slug. Scraper mantido no código mas não registrado no main.go (pokewallet.io é o primário).
- `cmd/api/main.go` — removidos imports `tcgplayer`, `cardmarket`, `pricing`; adicionado `pokewallet`. Scrapers: `ligapokemon`, `pwTCG`, `pwCM`, `ebay`. FlareSolverr removido.
- `docker-compose.yml` — serviço `flaresolverr` removido; `POKEWALLET_API_KEY` e `POKEMON_TCG_API_KEY` adicionados ao serviço `api`. Dependência `flaresolverr: service_started` removida.
- `internal/config/config.go` — campo `PokeWalletAPIKey string` adicionado; lê `POKEWALLET_API_KEY`.
- **Frontend** (`frontend/`) — Next.js 16, App Router, Tailwind CSS 4. Componentes: `SetCombobox` (combobox filtrável carregando sets da pokemontcg.io), `SearchForm`, `CardInfo`, `PriceMatrix` (tabela condição × fonte), `GradedSection` (vendas eBay gradeadas), `SourceCard` (accordion por fonte), `ConditionBadge`. `lib/api.ts` → `searchCard(number, set)` para `GET /api/v1/external-search`. `lib/sets.ts` → `useSets()` com cache em memória (uma fetch por sessão). `.env.local` → `NEXT_PUBLIC_API_URL=http://localhost:8080`.

Adicionado em 2026-05-09 (fase 4, lookup multi-fonte multi-condição):

- `card.CardWithSet` — view de leitura juntando carta + set, usada em endpoints de busca.
- `CardRepo.LookupCards(name, number, setCode, limit)` — query única com filtros opcionais combinados (NULLIF). Resolve cards ambíguos pelo número quando o nome se repete em múltiplos sets. Ordena por similaridade do nome (pg_trgm) e depois por release_date do set DESC.
- `pricesignal.Service.ByConditions(variantID, window)` — single SQL `GROUP BY condition, source` devolvendo a matriz inteira (NM/LP/MP/HP/DMG × LigaPokemon/TCGplayer/eBay) numa só query. Estrutura `SignalsByCondition{Conditions: []ConditionSignal{Sources: []PerSourceSignal}}`.
- Handler `GET /api/v1/cards/lookup` — endpoint rico aceitando `name`, `number`, `set`, `with_signal`, `window`, `limit`. Pelo menos um de name/number/set obrigatório (evita scan da tabela inteira). Resposta agrupa carta+set+variantes+matriz de preços.
- Seed atualizado para popular 3 condições (NM, LP, MP) com fatores realistas (LP ≈ 85% NM, MP ≈ 70% NM) e volumes decrescentes — ~1620 linhas de price_daily geradas.

Adicionado em 2026-05-09 (fase 3, lojas + estoque + matching):

- Migration 000005 — `stores`, `stock_items`, `stock_movements` (+ ENUM `stock_movement_kind`), `external_card_refs`. Adiciona `'ligapokemon'` ao ENUM `price_source`.
- Correção de schema: trocadas as `UNIQUE` com `COALESCE` por `CREATE UNIQUE INDEX` em `card_variants` e `stock_items` (Postgres não aceita expressão em UNIQUE constraint).
- Domain types: `store.Store`, `store.StockItem`, `store.StockMovement` (+ `MovementKind` enum), `matching.ExternalCardRef`. Atualizado `pricing.Source` com `SourceLigaPokemon`.
- `StoreRepo` — CRUD básico de lojas (Create, GetByID, GetBySlug, ListByOwner). `slug` global UNIQUE.
- `StockRepo` — operações **transacionais** com `SELECT … FOR UPDATE`:
  - `RegisterPurchase` — cria/atualiza item, soma quantidade, recalcula `cost_avg_brl` (média ponderada com 4 casas), grava movimento `purchase`.
  - `RegisterSale` — bloqueia item, valida disponibilidade (`ErrInsufficientStock`), subtrai quantidade, grava movimento `sale`. Não mexe em custo.
  - `RegisterAdjustment` — delta livre (+/-), valida não-negatividade, grava movimento `adjustment`.
  - `GetItemByID`, `ListItemsByStore` (paginado, filtro `onlyInStock`), `ListMovementsForItem`.
- `ExternalRefRepo` — `Create`, `GetBySourceID`, `ListByVariant`. Colisão `(source, external_id)` → `ErrAlreadyExists`.
- `internal/service/pricesignal` — `Service.For(ctx, variantID, condition)` agrega `price_daily` dos últimos 30 dias agrupando por fonte: devolve `WeightedAvg` (ponderada por volume), `Min`, `Max`, `SalesCount`, `LastSaleDay`. Janela configurável via `ForWindow`.

## 6. Próximos Passos (priorizados)

Ordem atualizada em 2026-05-10.

1. **Matching service** — `internal/service/matching`: dada uma observação raw (title, set, number) tenta achar variant_id por (set.code + cards.number + finish). Se sucesso com confidence alta, cria automaticamente o `external_card_ref`. Senão, fila de revisão manual.
2. **Handlers HTTP — store/stock**:
   - `POST /stores` (criar loja)
   - `POST /stores/{id}/stock/purchase` (registrar compra)
   - `POST /stock-items/{id}/sale` (registrar venda)
   - `GET  /stores/{id}/stock` (lista paginada, opcional `?with_signal=true`)
   - `GET  /variants/{id}/signal` (PriceSignal das 3 fontes)
3. **Frontend — evolução**: adicionar listagem de estoque da loja ao frontend, integrar `/api/v1/cards/lookup` para histórico de preços, adicionar gráfico de série temporal (price_daily).
4. **Job de agregação diária** — `cmd/aggregate` (ou cron-job em `cmd/scraper`) chama `PriceDailyRepo.RebuildDay(today)` e `RebuildDay(today-1)`. Sem isso, `pricesignal` fica desatualizado.
5. **Testes integrados** — `tests/integration` com `testcontainers-go` para os repos críticos (`StockRepo` com transações, `PriceDailyRepo.RebuildDay`, `forex.Service` com fake Provider).
6. **Auth + tabela `users`** — destrava FK em `listings.seller_id` e em `stores.owner_id`. Decidir: Supabase Auth ou tabela própria.
7. **Marketplace público** — handlers e UI para comprar listings de outras lojas. Aqui entra a integração de pagamentos (seção 9).

## 7. Convenções

- **Nomes em Go**: pacotes em `lowercase`, exportados em `CamelCase`. Sem stutter (`card.Card`, não `card.CardModel`).
- **Erros**: sempre embrulhados com `fmt.Errorf("...: %w", err)`. Sentinelas exportados (`ErrNotFound`) por pacote de domínio quando relevante.
- **SQL**: snake_case sempre, ENUMs no plural natural do conceito (`card_condition`, `price_source`).
- **Migrations**: pares `.up.sql` / `.down.sql` numerados em sequência. Toda alteração de ENUM exige migration nova — nunca editar uma já aplicada.
- **Frontend**: App Router; server components por padrão, client component (`"use client"`) só quando há interatividade (estado, eventos, hooks). `NEXT_PUBLIC_API_URL` via `.env.local` aponta para o backend Go.

## 8. O que NÃO está pronto (lembretes para sessões futuras)

- Tabela `users` ainda não existe — `listings.seller_id` e `stores.owner_id` estão sem FK proposital até decidirmos se Auth fica no Supabase Auth ou em tabela própria.
- Não há job criando partições futuras de `price_history`. Hardcoded até 2026-Q4.
- Sem testes ainda. Repositórios das fases 1–3 + serviços da fase 2 precisam nascer com `testcontainers-go` antes do primeiro deploy. Crítico: `StockRepo` (transações, custo médio ponderado).
- `PriceHistoryRepo.InsertBatch` (CopyFrom) **não respeita ON CONFLICT** — a estratégia atual para deduplicação é o pipeline a montante; se acoplarmos COPY a um stage table + `INSERT ... ON CONFLICT DO NOTHING`, é uma migration nova.
- `ListingRepo` ainda não existe — virá junto com a história de marketplace.
- `forex.BCBProvider` usa `decimal.NewFromFloat` no parsing do JSON — aceitável porque o BCB devolve número com 4 casas; se passarmos a fontes que enviam string (Cardmarket, OXR), trocar para `decimal.NewFromString` direto.
- Cartas gradeadas (PSA/Beckett/CGC) com cert numbers únicos não têm rastreio individual hoje — `stock_items` agrega por `grade` mas não distingue dois "PSA 10" (ver ADR-009).
- Reservas de estoque (`reservation`/`release`) são ENUMs declarados em `stock_movement_kind` mas o código não emite esses movimentos ainda. Quando entrar a reserva no checkout, vamos precisar de uma coluna `quantity_available` derivada (ou uma tabela `stock_reservations`).
- Pipeline de scraping → matching → `price_history` ainda não existe. Próximo passo na fase 4.

## 9. Integração de Pagamentos (planejada)

A plataforma ainda não cobra nada. Quando ativarmos o fluxo "comprar listing", precisaremos de PSP. **Premissas e decisões a tomar quando chegar a hora:**

### 9.1 Escolha do PSP

- **Mercado Pago** é a escolha default — cobre PIX (instantâneo, fee baixo), cartão e boleto, é dominante no público alvo (BR), e tem SDK Go razoável (ou via REST). Ideal para o MVP.
- **Stripe** entra num momento 2, se abrirmos para compradores internacionais — paga em USD/EUR e tem fluxo de payout dedicado.
- **Não usar** múltiplos PSPs simultâneos no MVP. Multiplica conciliação contábil sem ganho proporcional.

### 9.2 Modelo de dados (a criar)

Migration nova vai introduzir, no mínimo:

- `payment_intents` — uma linha por tentativa de pagamento. Campos chave: `id` (UUID), `listing_id` (FK), `buyer_id` (FK users), `amount_brl NUMERIC(14,2)`, `psp` (ENUM `'mercado_pago' | 'stripe'`), `psp_payment_id` (texto, único por PSP), `status` (ENUM `'pending'|'authorized'|'captured'|'refunded'|'failed'|'expired'`), `idempotency_key` (texto, único), `created_at`, `updated_at`.
- `payment_events` — log append-only de webhooks recebidos do PSP, com `payment_intent_id`, `event_type`, `payload JSONB`, `received_at`. Necessário para auditoria e replay.
- `payouts` — quanto e quando pagamos cada vendedor. `seller_id`, `amount_brl`, `psp_transfer_id`, `status`, `paid_at`.

### 9.3 Regras a respeitar

- **Idempotência.** Webhook do PSP pode chegar 2× ou fora de ordem. Toda mutação derivada de webhook usa `idempotency_key` (PSP) ou `psp_payment_id` + `event_type` como guarda.
- **Escrow lógico.** Cobramos o comprador, mas só liberamos payout depois da confirmação de recebimento (ou prazo máximo). `listings.status` migra `active → reserved → sold`. Se o comprador abre disputa, congelamos o payout.
- **Decimal sempre.** `amount_brl` é `NUMERIC(14,2)` no banco e `decimal.Decimal` em Go — mesma regra do resto do projeto. Nunca multiplicar centavos por floats.
- **Fees registradas.** Cada `payment_intent` deve guardar `fee_brl` (o que o PSP cobrou) separado do bruto, para sabermos o líquido sem precisar reconsultar a API depois.
- **Webhooks assinados.** Validar HMAC do PSP antes de qualquer mutação. Mercado Pago usa `x-signature`; Stripe usa `Stripe-Signature`. Rejeitar 401 se falhar.
- **Reconciliação diária.** Job que compara `payment_intents` com a API do PSP para detectar desincronizações silenciosas.

### 9.4 Onde mora o código

- `internal/payment/` — interface `Provider` (parecida com `forex.Provider`), implementações `mercadopago.go`, `stripe.go`, e `service.go` orquestrando. Mantém o handler HTTP independente do PSP.
- Webhook handler em `internal/handler/payment_webhook.go`. URL pública `POST /webhooks/payments/{psp}`.

### 9.5 O que NÃO fazer no primeiro corte

- Não construir cartão tokenizado interno — sempre redirecionar / usar checkout do PSP.
- Não armazenar PAN / CVV em hipótese alguma. Nem em log.
- Não reaproveitar o `pricing.Service.NormalizeBRL` para "preço do checkout". Comprador paga em BRL, o que congela é o valor do listing no momento da reserva.
