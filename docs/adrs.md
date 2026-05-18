# ADRs — MercadoTCG

## ADR-001 — SQL puro versionado em vez de Prisma/ORM
**Decisão:** Migrations escritas à mão em SQL com `golang-migrate`.
**Razão:** Particionamento por range, índices BRIN, ENUMs nativos e GIN com `pg_trgm`. ORMs escondem esses recursos.

## ADR-002 — `shopspring/decimal` para todo valor monetário
**Decisão:** Nenhum `float32`/`float64` em campo de preço em qualquer camada Go.
**Razão:** Erros de arredondamento são inaceitáveis em marketplace. Mapeia para `NUMERIC(14,2)`.

## ADR-003 — Variantes como tabela própria (`card_variants`)
**Decisão:** price_history, listings apontam para `card_variants.id`, não `cards.id`.
**Razão:** Master Ball × Poke Ball × Holo × Reverse Holo são séries históricas distintas.

## ADR-004 — Particionamento de `price_history` por trimestre
**Decisão:** `PARTITION BY RANGE (observed_at)`, partições trimestrais criadas previamente.
**Trade-off:** PK inclui `observed_at` (composta). Hardcoded até 2026-Q4.

## ADR-005 — Tabela quente `price_daily`
**Decisão:** Pré-agregamos diariamente min/avg/max/median/p25/p75 em `price_daily`. Gráficos sempre leem dela.
**Razão:** Calcular percentis em runtime sobre milhões de linhas mata latência.

## ADR-006 — Auditoria cambial dupla
**Decisão:** `price_history` armazena `price_original + currency` e `price_brl + fx_rate_used`. Conversão única na ingestão.
**Razão:** Preserva auditabilidade histórica.

## ADR-007 — BRIN em `observed_at`
**Decisão:** BRIN para varreduras temporais + BTree composto `(variant_id, observed_at DESC)` para última observação por variante.

## ADR-008 — Multi-tenant desde a fundação
**Decisão:** `stores` desde o dia 1, `owner_id` em toda store, `store_id` em todo `stock_item`.

## ADR-009 — Estoque agregado + log de movimentos
**Decisão:** `stock_items` = uma linha por (store, variant, condition, language, grade) com `quantity` e `cost_avg_brl`. Toda alteração → linha em `stock_movements` (append-only).
**Trade-off:** Cartas gradeadas com cert numbers únicos não têm rastreio individual.

## ADR-010 — Matching strict via `external_card_refs`
**Decisão:** Observação de scraper só vira `price_history` se existir linha em `external_card_refs` para `(source, external_id)`. Sem match → quarentena.
**Trade-off:** Bootstrap exige matching manual para popular as primeiras refs.

## ADR-011 — Estratégia por fonte
**Decisão:** LigaPokemon = scraping HTML (goquery). TCGPlayer + Cardmarket = pokewallet.io. eBay = Scrydex (graded). Scrapers legados `tcgplayer/` e `cardmarket/` no código mas não registrados no `main.go`.
**Trade-off:** pokewallet.io free tier: 100 req/hora.

## ADR-012 — (SUPERSEDIDO pelo ADR-015)
Scraper `internal/scraper/tcgplayer/` existe mas não é registrado.

## ADR-013 — Preços TCGPlayer por condição via multiplicadores
NM → LP=80%, MP=64%, HP=40%, DMG=24%. São estimativas.

## ADR-014 — Cardmarket: multiplicadores por condição
Preço base `low` NM → LP=70%, MP=45%, HP=25%, DMG=10%.

## ADR-015 — pokewallet.io como fonte primária para TCGPlayer + Cardmarket
**Decisão:** `New(apiKey, timeout)` devolve dois `scraper.Source` com `*Client` + `requestCache` (TTL 60s) compartilhados. `pickBestMatch`: set code+number exatos > set name+number > number only.

## ADR-016 — Auth próprio: JWT + Google OAuth + email/senha
**Decisão:** Access token JWT (HS256, 15 min) em memória. Refresh token (SHA-256 hash) em `localStorage` (`mtcg_rt`). Google OAuth com state HMAC (10 min). bcrypt cost 12.
**Razão:** Controle total do schema (RBAC de loja, FK, audit). Supabase Auth descartado.
**Trade-off:** Refresh em localStorage vulnerável a XSS. Mitigação: token curto + HTTPS + CSP.

## ADR-017 — Criação de lojas: apenas platform_admin
**Decisão:** `POST /api/v1/admin/stores` (RequirePlatformAdmin). Admin informa `owner_id`; backend adiciona owner em `store_members` como `admin`.

## ADR-018 — Validação de documento: CNPJ auto-verified, CPF manual
**Decisão:** CNPJ → checksum + ReceitaWS. `situacao == "ATIVA"` → `auto_verified`. CPF → checksum apenas → sempre `pending`.
**Gotcha pgx/v5:** ENUMs Postgres precisam de cast explícito no SQL (`$n::document_type`).

## ADR-019 — Navegação via hover dropdowns no SiteHeader
**Decisão:** "Minha Loja" e "Admin" com hover dropdown. `useState<string | null>` + `onMouseEnter`/`onMouseLeave`. Pathname via `usePathname()`, regex `/^\/lojas\/([^/]+)/`.
**Layouts:** `app/admin/layout.tsx` = guard único (filhas não duplicam). `app/lojas/[id]/layout.tsx` = barra com abas.

## ADR-020 — Registro em duas etapas: email-first
**Decisão:** `POST /auth/register` aceita só `email`. Link de verificação → `/auth/verify-email` recebe `{token, password, display_name}`, completa cadastro + retorna tokens (auto-login).
**Dev:** `verify_url` sempre logado no stdout.

## ADR-021 — `tcg` como VARCHAR(32) com CHECK constraint
**Decisão:** `VARCHAR(32) NOT NULL DEFAULT 'pokemon'` com CHECK em `card_sets`. Valores: `pokemon, pocket, magic, yugioh, onepiece, lorcana, fab`.
**Razão:** Mais fácil adicionar TCGs que ALTER ENUM. pgx não exige cast para VARCHAR.

## ADR-022 — Séries de sets como entidade própria (`card_series`)
**Decisão:** Tabela `card_series` (`name`, `name_pt`, `tcg`); `card_sets.series_id UUID FK` aponta para ela.
**Razão:** Série tem identidade própria. Desnormalizar em `card_sets` criaria inconsistência.

## ADR-023 — upload.Provider: interface polimórfica Local + S3
**Decisão:** `Put/PublicURL/Exists`. `LocalProvider` para dev. `S3Provider` (aws-sdk-go-v2) para prod. `NewFromEnv()` seleciona via `STORAGE_BACKEND=local|s3`.
**Chaves S3:** `{tcg}/cards/{setCode}/{localID}.webp` (cartas); `{tcg}/sets/{setID}_logo.png` / `_symbol.png` (sets); `logos/{uuid}.{ext}` (lojas).
**Gotcha S3:** Bucket novo tem ACLs desabilitadas — usar bucket policy, não ACL por objeto. `Put` bufferiza body via `io.ReadAll` (Content-Length obrigatório).

## ADR-024 — Slug de carta: `{setCode}-{collectorNumber}`
**Decisão:** Handler tenta UUID; falha → split no **primeiro** hífen. Query SQL normaliza `"001"` == `"1"` via `~ '^\d+$'`. `collector_number` nunca tem `"/"` — display composto no frontend.
**Trade-off:** Set codes nunca devem conter hífen.

## ADR-025 — TCGDex como fonte primária de catálogo + bilíngue PT-BR/EN
**Decisão:** `cmd/import-catalog` usa TCGDex (`api.tcgdex.net/v2`). Rate limit 1 req/s, retry 429/5xx. EN autoritativo; PT-BR em paralelo (404 silenciado). Toggle PT/EN em `localStorage` (`mtcg_lang`, default `pt`). SEO sempre em EN.
**Trade-off:** PT-BR limitado a TCG Pocket (~1.100+ cartas). TCGDex sem preços — pokemontcgio mantido para external-search.
**Variantes:** `WPromo` → `FinishNormal`. Sem flags → `[FinishNormal, FinishReverseHolo]`.
**Supersedido parcialmente por ADR-027** (dimensão catálogo EN). Dimensão bilinguismo PT-BR permanece válida.

## ADR-026 — (reservado — ver histórico de conversas)

## ADR-027 — Scrydex como fonte primária de catálogo; TCGDex como complemento PT-BR
**Decisão:** `cmd/import-catalog` reescrito para usar a Scrydex API como fonte autoritativa de sets, cartas, variantes e imagens. TCGDex mantido exclusivamente para enriquecimento `name_pt` / `image_url_pt` (única fonte de PT-BR disponível). ADR-025 supersedido na dimensão de catálogo EN.
**Razão:** Scrydex provê `printed_total` explícito, variantes com nomes canônicos mapeáveis a `FinishXxx`, IDs de produto TCGPlayer embutidos em `variants[].marketplaces[]`, e imagens em alta resolução estáveis. TCGDex não tem nenhum desses dados estruturalmente. Decisão de produto de 2026-05-15.
**Trade-offs aceitos:** Re-importação completa (~30k créditos) requer plano Growth ($99/mo). PT-BR continua limitado ao que TCGDex indexa. Sets apenas no TCGDex ficam com `import_source='tcgdex_only'` — sem preços Scrydex. Scrydex é serviço pago com risco de mudança de preço/downtime.
**Alternativas rejeitadas:** Manter TCGDex como primário (sem variantes estruturadas, sem `printed_total`, sem IDs TCGPlayer); pokemontcg.io (sem Pocket, sem PT-BR, sem preços); catálogo manual (inviável).
**Revisão:** Se Scrydex ultrapassar $300/mo ou descontinuar endpoint de catálogo.

## ADR-028 — `import_source` em `card_sets` e `cards`
**Decisão:** Coluna `import_source VARCHAR(32) NOT NULL DEFAULT 'tcgdex_legacy'` adicionada em `card_sets` e `cards` via migration 000019. Valores: `'scrydex'`, `'tcgdex_only'`, `'tcgdex_legacy'`, `'manual'`.
**Razão:** Re-importação é operação progressiva. A coluna distingue registros migrados para Scrydex de legados TCGDex durante e após a migração, viabilizando reconciliação e rollback parcial identificável.
**Trade-offs aceitos:** Coluna extra em duas tabelas grandes (impacto mínimo — `VARCHAR(32) NOT NULL DEFAULT` não afeta queries existentes). UPSERTs de import precisam passar `import_source` explicitamente.
**Alternativas rejeitadas:** Tabela separada `import_log` (over-engineering); inferir via `external_card_refs` (indireto).
**Revisão:** Remover após 6 meses se todos os registros forem `'scrydex'`.

## ADR-031 — Cascade de séries de preços ao deletar variantes de catálogo
**Data:** 2026-05-17 · **Status:** Aceito

**Decisão:** Manter `ON DELETE CASCADE` em `price_history`, `price_daily` e `external_card_refs` quando uma `card_variant` é removida. Bloquear delete (HTTP 409) apenas quando existirem `stock_items` ou `listings` ativos apontando para a variante/carta.

**Razão:** Histórico de preço e refs externas são dados derivados, reconstrutíveis a partir do scraping. Bloquear curadoria de catálogo por causa de uma série de preços de variante mal cadastrada inviabiliza correção operacional. `stock_items` e `listings` representam valor de negócio irrecuperável — apagar variante referenciada por essas tabelas seria perda de dado real.

**Alternativas rejeitadas:** RESTRICT em `price_history` (bloqueia operação por dado derivado); soft-delete (complexidade em todas as queries sem requisito de "lixeira"); snapshot em tabela de arquivo (over-engineering).

**Revisão:** Reabrir se houver requisito legal/fiscal de manter histórico de preço imutável, ou se volume de `price_history` por variante tornar o cascade lento o suficiente para impactar latência do endpoint de delete.

## ADR-029 — (adiado — fora de escopo desta feature)
Alteração do ENUM `price_source` para adicionar `'scrydex'` e `'tcgdex'` foi adiada indefinidamente. O MercadoTCG é um marketplace cujos preços reais são as listagens das próprias lojas. A sincronização de preços externos (`price_history`) não é necessária agora e deve ser avaliada separadamente quando o módulo de precificação assistida for priorizado.

## ADR-030 — (adiado — fora de escopo desta feature)
`cmd/price-sync` (job diário de sincronização de preços via Scrydex) foi adiado indefinidamente pelo mesmo motivo do ADR-029. Os preços externos (Scrydex, pokewallet.io) servem apenas como referência para o admin validar listings manualmente — não precisam ser armazenados localmente neste momento. Reavaliar quando o módulo de precificação assistida entrar no roadmap.

---

## Plano de Implementação — Reimportação de Catálogo via Scrydex (feat/multi-language-import)

**Escopo:** Reimportar o catálogo completo usando a Scrydex como fonte primária de sets, cartas,
variantes e imagens. TCGDex mantido como complemento exclusivo para `name_pt` e `image_url_pt`.
Preços externos, `price_history` e qualquer alteração em ENUMs de pricing estão fora de escopo.

### Resumo de tasks

| Task | O que é | Esforço estimado |
|---|---|---|
| A | Migration 000019 — coluna `import_source` | ~1h |
| B | `internal/catalog/scrydex/` — client HTTP com rate limiter | ~3h |
| C | Reescrita de `cmd/import-catalog` — Scrydex como primário | ~6h |
| D | Enriquecimento TCGDex — `name_pt`, `image_url_pt` | ~2h |

---

### Task A — Migration 000019: coluna `import_source`

**Arquivos a criar:**
- `backend/migrations/000019_import_source.up.sql`
- `backend/migrations/000019_import_source.down.sql`

**SQL (up):**
```sql
ALTER TABLE card_sets ADD COLUMN IF NOT EXISTS import_source VARCHAR(32) NOT NULL DEFAULT 'tcgdex_legacy';
ALTER TABLE cards     ADD COLUMN IF NOT EXISTS import_source VARCHAR(32) NOT NULL DEFAULT 'tcgdex_legacy';
```

**SQL (down):**
```sql
ALTER TABLE card_sets DROP COLUMN IF EXISTS import_source;
ALTER TABLE cards     DROP COLUMN IF EXISTS import_source;
```

**Valores permitidos (convenção, não CHECK constraint — evita migração futura):**
- `'scrydex'` — importado da Scrydex como fonte primária
- `'tcgdex_only'` — set existe apenas no TCGDex, sem correspondência na Scrydex
- `'tcgdex_legacy'` — importado antes desta feature (default retroativo)
- `'manual'` — inserido manualmente via admin panel

**Idempotência:** `ADD COLUMN IF NOT EXISTS` é seguro para re-aplicação. O `DEFAULT 'tcgdex_legacy'`
retroage em todos os registros existentes sem UPDATE separado.

**Impacto em queries existentes:** nenhum — nova coluna com DEFAULT, não afeta `SELECT *` via pgx
(pgx mapeia por nome de campo, não por posição). UPSERTs do import-catalog precisarão passar o valor
explicitamente após esta task.

---

### Task B — `internal/catalog/scrydex/`: client HTTP com rate limiter

**Arquivos a criar:**
- `backend/internal/catalog/scrydex/client.go` — client principal
- `backend/internal/catalog/scrydex/types.go` — structs de resposta da API
- `backend/internal/catalog/scrydex/ratelimit.go` — token bucket

**Interfaces e tipos principais:**

```go
// client.go
type Client struct {
    http      *http.Client
    apiKey    string
    baseURL   string
    limiter   *rateLimiter
}

func New(apiKey string, timeout time.Duration) *Client

// Endpoints de catálogo usados:
func (c *Client) ListExpansions(ctx context.Context) ([]Expansion, error)
func (c *Client) GetExpansion(ctx context.Context, id string) (*Expansion, error)
func (c *Client) ListCards(ctx context.Context, expansionID string) ([]CardSummary, error)
func (c *Client) GetCard(ctx context.Context, id string) (*Card, error)
```

```go
// types.go — structs mapeadas do JSON da Scrydex
type Expansion struct {
    ID           string   `json:"id"`           // ex: "A1"
    Name         string   `json:"name"`          // nome EN (autoritativo)
    Language     string   `json:"language"`      // "en", "ja", "ko"
    Region       string   `json:"region"`        // "international", "japan"
    ReleaseDate  string   `json:"releaseDate"`
    PrintedTotal int      `json:"printedTotal"`
    LogoURL      string   `json:"logoUrl"`
    SymbolURL    string   `json:"symbolUrl"`
    Series       string   `json:"series"`
}

type CardSummary struct {
    ID     string `json:"id"`     // ex: "tcgp-A1-35"
    Number string `json:"number"` // ex: "35"
    Name   string `json:"name"`
    Image  string `json:"imageUrl"`
}

type Card struct {
    CardSummary
    Variants   []CardVariant `json:"variants"`
    // campos adicionais conforme documentação Scrydex
}

type CardVariant struct {
    ID         string `json:"id"`
    Finish     string `json:"finish"`   // "Normal", "Holo", "ReverseHolo", etc.
    Marketplaces []Marketplace `json:"marketplaces"`
}

type Marketplace struct {
    Name       string `json:"name"`       // "TCGPlayer", "Cardmarket"
    ExternalID string `json:"externalId"` // ID do produto no marketplace
}
```

**Estratégia de rate limiting (ratelimit.go):**

A Scrydex não publica o limite exato do plano Starter — estimar conservadoramente
**2 req/s** (120 req/min) até confirmação via headers `X-RateLimit-*` em produção.
Implementar usando `golang.org/x/time/rate` (já está disponível como dependência transitiva
via `golang.org/x/oauth2`; verificar se está no `go.sum` antes — caso não, é necessário
`go get golang.org/x/time`):

```go
// ratelimit.go
import "golang.org/x/time/rate"

type rateLimiter struct {
    lim *rate.Limiter
}

// NewRateLimiter cria um limiter com burst=1 (sem rajadas) e rate configurável.
func NewRateLimiter(reqPerSec float64) *rateLimiter {
    return &rateLimiter{lim: rate.NewLimiter(rate.Limit(reqPerSec), 1)}
}

func (r *rateLimiter) Wait(ctx context.Context) error {
    return r.lim.Wait(ctx)
}
```

O client chama `c.limiter.Wait(ctx)` antes de toda requisição HTTP. Se a resposta for
HTTP 429, o client faz backoff exponencial: `500ms * 2^retries`, máximo 3 tentativas,
depois retorna erro. Erros 5xx seguem a mesma política de retry.

**Configuração via variáveis de ambiente** (adicionar ao `.env.example`):
```
SCRYDEX_API_KEY=       # obrigatório para importação
SCRYDEX_BASE_URL=https://api.scrydex.io  # sobrescrevível para testes
SCRYDEX_RATE_LIMIT=2   # req/s, padrão conservador
```

**Sem novas dependências obrigatórias:** `golang.org/x/time/rate` está disponível indiretamente
(confirmar no `go.sum`). Se não estiver: `go get golang.org/x/time@latest`. Não adicionar
bibliotecas de HTTP client externas — `net/http` padrão é suficiente com o limiter acima.

---

### Task C — Reescrita de `cmd/import-catalog`

**Arquivos a modificar/criar:**
- `backend/cmd/import-catalog/main.go` — pipeline principal reescrito
- `backend/cmd/import-catalog/scrydex_importer.go` — lógica de importação Scrydex
- `backend/cmd/import-catalog/image_worker.go` — pool de workers de download (extraído do main atual)
- `backend/internal/repository/postgres/card_repo.go` — adicionar `import_source` aos UPSERTs existentes

**Flags de CLI mantidas (retrocompatibilidade):**
```
--set <code>            importa um set específico pelo ID Scrydex (ex: A1)
--series <prefix>       importa todos os sets com prefixo de série
--recent <n>            importa os N sets mais recentes
--download-images       baixa imagens para o storage provider
--lang <code>           mantido mas restrito a "en" na importação Scrydex primária
--dry-run               lista o que seria importado sem persistir (novo)
--skip-images           alias para --download-images=false (já é o padrão)
```

**Pipeline principal (pseudocódigo de alto nível):**

```
1. Inicializar: DB, Scrydex client, TCGDex client, upload.Provider (se --download-images)
2. Scrydex.ListExpansions() → lista completa de sets
3. Aplicar filtros (--set, --series, --recent)
4. Para cada expansion (com tratamento de erros parciais — ver abaixo):
   a. Scrydex.GetExpansion(id) → metadados completos
   b. detectLanguage(expansion) → "en" | "ja" | "ko" (ver Task C.1 abaixo)
   c. UpsertSeries(expansion.Series, tcg)
   d. UpsertSet(expansion → card.Set{import_source: "scrydex"})
   e. Se --download-images: downloadSetImages(logo, symbol)
   f. Scrydex.ListCards(expansion.ID)
   g. Para cada card (em batches de 50):
      i.  Scrydex.GetCard(card.ID) → variantes + imagem
      ii. UpsertCard(card → card.Card{import_source: "scrydex"})
      iii. UpsertVariants(card.Variants → []card.Variant)
      iv. Se --download-images: enfileirar imgJob no worker pool
5. Aguardar worker pool de imagens finalizar
6. Se --enrich-pt (flag nova, opcional): rodar Task D
7. Log final: sets novos, sets atualizados, cartas, variantes, imagens
```

**Task C.1 — Sets japoneses: detecção de idioma e preenchimento de `name`/`name_en`**

A Scrydex indexa sets JA dentro da mesma API, distinguindo pelo campo `expansion.language`
(`"ja"`) ou `expansion.region` (`"japan"`). A lógica de mapeamento para o banco:

```go
func buildSet(exp scrydex.Expansion) card.Set {
    s := card.Set{
        Code:         exp.ID,
        Name:         exp.Name,       // padrão: nome EN
        NameEN:       exp.Name,       // sempre EN vindo da Scrydex
        Series:       exp.Series,
        TCG:          detectTCG(exp),
        Language:     card.LanguageEnglish,
        PrintedTotal: exp.PrintedTotal,
        ReleaseDate:  parseDate(exp.ReleaseDate),
        ImageURL:     exp.LogoURL,
        SymbolURL:    exp.SymbolURL,
        ImportSource: "scrydex",
    }

    if exp.Language == "ja" || exp.Region == "japan" {
        // Para sets JA: name = nome nativo JA (que a Scrydex retorna em `name`
        // quando o endpoint é consultado sem parâmetro de idioma explícito).
        // name_en = o mesmo valor EN que está em `name` para sets internacionais.
        // Enquanto a Scrydex não fornecer o nome JA nativo separadamente,
        // usamos o nome EN em ambos os campos e marcamos para preenchimento manual.
        //
        // NOTA: Verificar na PoC se a Scrydex tem campo separado para nome JA nativo.
        // Se não tiver: name=exp.Name (EN), name_en=exp.Name, e o admin preenche
        // o nome nativo via PUT /api/v1/admin/sets/{id}.
        s.Language = card.LanguageJapanese
        // name_en já está correto (nome EN da Scrydex)
        // name fica igual a name_en até preenchimento manual
    }

    return s
}
```

**Detecção do TCG a partir da Scrydex:**

```go
func detectTCG(exp scrydex.Expansion) string {
    // Scrydex usa campo "game" ou estrutura de ID para distinguir.
    // Pocket: IDs começam com "tcgp-" ou série "TCGP".
    // Ajustar conforme documentação real da API.
    if strings.HasPrefix(exp.ID, "tcgp") || exp.Series == "TCGP" {
        return "pocket"
    }
    return "pokemon"
}
```

**Tratamento de erros parciais (resiliência por série):**

Uma falha em um set individual não deve abortar sets subsequentes. O padrão:

```go
for _, exp := range expansions {
    if err := importExpansion(ctx, exp); err != nil {
        log.Error().Err(err).Str("expansion", exp.ID).Msg("importação falhou — continuando")
        stats.failedSets++
        continue  // próximo set
    }
    stats.processedSets++
}

// Ao final, se stats.failedSets > 0: exit code 1 (permite CI detectar importações parciais)
if stats.failedSets > 0 {
    log.Error().Msgf("%d sets falharam — verificar logs acima", stats.failedSets)
    os.Exit(1)
}
```

**Idempotência (UPSERT, não DELETE+INSERT):**

Todos os UPSERTs existentes no `CardRepo` já são idempotentes por `set.code` e
`(set_id, collector_number)`. A reescrita precisa apenas garantir que:
1. `import_source` seja passado como parâmetro explícito nos UPSERTs (após Task A).
2. A query de upsert de set não sobrescreva `import_source='manual'` com `'scrydex'`
   — usar `ON CONFLICT DO UPDATE SET import_source = EXCLUDED.import_source
   WHERE import_source != 'manual'` para proteger entradas manuais.

**Variáveis de ambiente adicionais ao `.env.example`:**
```
SCRYDEX_API_KEY=
SCRYDEX_BASE_URL=https://api.scrydex.io
SCRYDEX_RATE_LIMIT=2
```

---

### Task D — Enriquecimento TCGDex: `name_pt` e `image_url_pt`

**Arquivos a modificar:**
- `backend/cmd/import-catalog/main.go` — adicionar flag `--enrich-pt` e lógica de enriquecimento
- O client TCGDex (`internal/tcgdex/`) já existe; não reescrever.

**Flag nova:**
```
--enrich-pt   após importação Scrydex, busca name_pt e image_url_pt no TCGDex
              para cartas que ainda não têm esses campos preenchidos
```

**Lógica de enriquecimento:**

O enriquecimento PT-BR pode rodar como segunda fase do mesmo binário (separado do loop
principal Scrydex) ou como passagem incremental separada. A segunda opção é preferível
para evitar que uma falha no TCGDex bloqueie a importação principal:

```
1. SELECT id, set_code, collector_number FROM cards WHERE name_pt IS NULL OR image_url_pt IS NULL
   AND import_source = 'scrydex'  -- só cartas já migradas
2. Para cada carta (rate limit: 1 req/s — TCGDex não documentado mas conservador):
   a. Construir ID TCGDex: "{set_code}-{collector_number}" (convenção atual)
   b. TCGDex.GetCard(ctx, "pt-br", cardID)
   c. Se 404: silenciar (PT-BR limitado a TCG Pocket no TCGDex)
   d. Se 200: UPDATE cards SET name_pt=$1, image_url_pt=$2 WHERE id=$3
   e. Se --download-images: baixar image_url_pt para S3 (chave: "{tcg}/cards/{setCode}/{localID}_pt.webp")
3. Log: cartas enriquecidas, 404 silenciados, erros reais
```

**Limitação conhecida:** TCGDex só tem PT-BR para TCG Pocket (~1.100+ cartas). Para sets
do jogo principal, `GetCard("pt-br", ...)` retorna 404 sistematicamente — esse comportamento
é esperado e silenciado (log level DEBUG, não WARN).

**Rate limit TCGDex:** manter 1 req/s (política conservadora atual do projeto, per ADR-025).
Para 23k+ cartas, o enriquecimento completo levará ~6,4h em execução sequencial. Usar
pool de workers controlado — **não mais que 3 workers simultâneos** para evitar bans.
Com 3 workers e 1 req/s por worker: ~2,2h para 23k cartas.

---

### Estratégia de download de imagens (~23k+ cartas + 200+ sets)

**Problema:** ~23.160 imagens EN + ~1.100 imagens PT-BR + ~400 imagens de set (logo + symbol)
= ~25k downloads. Com rate limiting rigoroso da Scrydex, isso pode levar horas.

**Princípios:**
1. `provider.Exists(ctx, key)` é chamado antes de todo download — imagens já no S3 são puladas.
   Para re-importações parciais ou re-runs, apenas imagens ausentes serão baixadas.
2. O worker pool de imagens é desacoplado do loop de importação de metadados via canal bufferizado
   (`chan imgJob`, capacidade 500) — o mesmo padrão do código atual.
3. Downloads de imagens de set (logo + symbol) ocorrem inline no loop principal (1 por set,
   ~200 total) — não entram no pool de workers.

**Concorrência recomendada:**

| Fonte | Workers simultâneos | Sleep entre requests |
|---|---|---|
| Scrydex (imagens de carta) | 3 | controlado pelo rate limiter do client (2 req/s global) |
| TCGDex (imagens PT-BR) | 2 | 500ms entre downloads no mesmo worker |
| S3 PUT | sem limite relevante | — |

O rate limiter do client Scrydex (Task B) é global entre workers — `rate.Limiter` é
goroutine-safe. Com 3 workers compartilhando 1 limiter de 2 req/s, a taxa efetiva de
download é de até 2 imagens/s = ~3,2h para 23k imagens (caso nenhuma já esteja no S3).

Para re-importações onde as imagens já existem no S3, `provider.Exists()` retorna `true`
e o download é pulado — o custo é apenas 1 `HeadObject` por carta, que é barato.

**Estimativa de créditos Scrydex para catálogo completo:**
- `ListExpansions`: 1 crédito
- `GetExpansion` por set: ~200 créditos
- `ListCards` por set: ~200 créditos
- `GetCard` por carta: ~23.000 créditos
- **Total estimado: ~23.400 créditos**

O plano Starter da Scrydex tem 5.000 créditos/mês — insuficiente para importação completa.
Re-importação completa requer plano Growth ($99/mo, 50k créditos). Para desenvolvimento
e testes, usar `--set <code>` para importar apenas 1-2 sets.

**Variável de ambiente para controle de concorrência (opcional):**
```
IMPORT_IMAGE_WORKERS=3  # padrão: 3
```

---

### Checklist de rollback / recuperação parcial

- Todo upsert é idempotente — re-rodar o comando para sets específicos é seguro.
- `import_source='tcgdex_legacy'` permanece em sets não processados — fácil identificar o que falta.
- Se a importação abortar no meio: re-rodar com `--series <prefix>` para retomar por série.
- Sets com `import_source='manual'` são protegidos contra sobrescrita (ver Task C, cláusula WHERE).
- Imagens já no S3 não são re-downloaded — re-runs são seguros e baratos.
- Migration 000019 tem `.down.sql` que reverte as colunas sem perda de dados funcionais.
