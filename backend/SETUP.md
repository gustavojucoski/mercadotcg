# MercadoTCG — Setup local

Dois caminhos. **O caminho A é o recomendado** se você não tem Go instalado.

---

## Caminho A — Tudo via Docker (sem Go local) ★

### A.1. Pré-requisito único

**Docker Desktop** instalado e **rodando** — https://www.docker.com/products/docker-desktop

Confirme que o engine Linux está ativo:
```powershell
docker info
```
Se aparecer "Server Version:" com um número, está OK.  
Se der erro de pipe (`open //./pipe/dockerDesktopLinuxEngine: ...`), abra o Docker Desktop pelo menu Iniciar e espere o ícone da baleia ficar fixo na bandeja.

### A.2. Subir tudo (1 comando)

Na **raiz** do projeto (`MercadoTCG/`, não dentro de `backend/`):

```powershell
cd C:\Users\gusta\OneDrive\Documentos\Claude\Projects\MercadoTCG
copy .env.example .env   # edite e preencha JWT_SECRET no mínimo
docker compose up --build
```

Esse comando:

1. Faz build da imagem Go (compila `api`, `migrate`, `seed` dentro de um container `golang:1.22-alpine`).
2. Sobe o **Postgres 16**.
3. Sobe o **Adminer** (UI web do banco em http://localhost:8081).
4. Roda o **migrate** automaticamente — aplica o schema.
5. Roda o **seed** automaticamente — popula dados de demo.
6. Sobe a **API** em http://localhost:8080.

Na primeira vez demora uns 2–4 minutos (download de imagens + compilação).  
Nas próximas, segundos.

Você verá no log a sequência:
```
mercadotcg-db        | database system is ready to accept connections
mercadotcg-migrate   | ok: migrations aplicadas
mercadotcg-seed      | ==> Seed completo!
mercadotcg-api       | api iniciada port=8080
```

### A.3. Confirmar que subiu

Em outro PowerShell (não fecha o que está rodando o compose):

```powershell
curl http://localhost:8080/healthz
```

Resposta esperada: `{"status":"ok"}`.

Para ver os UUIDs gerados pelo seed (sua loja, primeira variante etc.):
```powershell
docker compose logs seed
```

### A.4. Comandos do dia a dia

Todos os comandos abaixo devem ser rodados na **raiz** do projeto (`MercadoTCG/`).

| Quero | Comando |
|---|---|
| Ver logs da API | `docker compose logs -f api` |
| Ver logs do frontend | `docker compose logs -f frontend` |
| Subir só o banco (sem rebuild) | `docker compose up -d db adminer` |
| Reaplicar migrations | `docker compose run --rm migrate /app/migrate up` |
| Reverter 1 migration | `docker compose run --rm migrate /app/migrate down 1` |
| Re-rodar seed manualmente | `docker compose run --rm seed` |
| Parar tudo (mantém dados) | `docker compose down` |
| Parar e zerar o banco | `docker compose down -v` |
| Rebuildar após mudar código Go | `docker compose up --build api` |
| Rebuildar após mudar frontend | `docker compose up --build frontend` |

### A.5. Acessar o banco visualmente

Abra http://localhost:8081 e logue:

- **System:** `PostgreSQL`
- **Server:** `db`
- **Username:** `mercadotcg`
- **Password:** `mercadotcg`
- **Database:** `mercadotcg`

---

## Caminho B — Go local (se já tem ou vai instalar)

Mais rápido pra desenvolvimento iterativo (não precisa rebuildar imagem a cada mudança).

### B.1. Pré-requisitos

- **Go 1.22+** — https://go.dev/dl/
- **Docker Desktop** (apenas para o Postgres)

### B.2. Passos

```powershell
cd C:\Users\gusta\OneDrive\Documentos\Claude\Projects\MercadoTCG

# Copia as variáveis de ambiente
copy .env.example .env   # ajuste JWT_SECRET e DATABASE_URL (aponte para localhost:5432)

# Entre no backend para rodar os comandos Go
cd backend

# Sobe só o Postgres + Adminer
docker compose up -d db adminer

# Resolve dependências Go
go mod tidy

# Aplica schema
go run ./cmd/migrate up

# Popula dados de demo
go run ./cmd/seed

# Sobe a API
go run ./cmd/api
```

---

## Endpoints disponíveis

Pegue os UUIDs no log do seed (`docker compose logs seed`) e substitua nos exemplos abaixo. **Os UUIDs são determinísticos** — sempre os mesmos entre re-execuções do seed.

### Health check
```powershell
curl http://localhost:8080/healthz
```

### Buscar cartas pelo nome (busca tolerante a typos)
```powershell
curl "http://localhost:8080/api/v1/cards/search?q=charizard"
curl "http://localhost:8080/api/v1/cards/search?q=pikahcu"   # com typo, ainda acha
```

### ★★ Busca AO VIVO em LigaPokemon + TCGplayer + eBay

Faz fan-out paralelo nas 3 fontes externas e devolve o que cada uma respondeu.
**Não depende do nosso catálogo interno** — funciona pra qualquer carta.

```powershell
# Por nome
curl "http://localhost:8080/api/v1/external-search?name=Charizard"

# Nome + número (recomendado pra desambiguar)
curl "http://localhost:8080/api/v1/external-search?name=Charizard&number=199/191"
```

Resposta:
```json
{
  "query": {"name": "Charizard", "number": "199/191"},
  "fetched_at": "2026-05-09T20:00:00Z",
  "sources": [
    {
      "source": "ebay",
      "duration_ms": 0,
      "error": "scraper: source não configurada",
      "results": []
    },
    {
      "source": "ligapokemon",
      "duration_ms": 850,
      "results": [
        {
          "title": "Charizard ex 199/191 (Surging Sparks)",
          "url": "https://www.ligapokemon.com.br/?view=cards/card&card=...",
          "price": "298.40",
          "currency": "BRL",
          "condition": "NM",
          "language": "Inglês"
        }
      ]
    },
    {
      "source": "tcgplayer",
      "duration_ms": 0,
      "error": "scraper: source não configurada",
      "results": []
    }
  ]
}
```

**Importante:**

- **LigaPokemon** funciona sem credencial mas é scraping HTML — os seletores
  CSS no código (`internal/scraper/ligapokemon/ligapokemon.go`) foram
  escritos sem acesso ao HTML real. Se vier vazio na primeira execução,
  abra o site no navegador, inspecione, e ajuste os seletores das funções
  `parseSearchResults` e `parseProductCard`. Me chame com o HTML que vejo
  e ajusto.

- **TCGplayer** e **eBay** vão devolver `error: "scraper: source não configurada"`
  até você cadastrar credenciais no `.env`:
  - TCGplayer: https://docs.tcgplayer.com/docs/welcome (gratuito)
  - eBay: https://developer.ebay.com/ (gratuito)
  Coloca as keys no `.env` da pasta `backend/` e rebuilda: `docker compose up --build api`.

### Importar catálogo completo de cartas (Pokemon TCG API)

Pra ter qualquer carta aparecendo nas buscas internas (`/cards/lookup`,
`/cards/search`), importe o catálogo oficial:

```powershell
# Tudo (~30 mil cards, demora 5-10 min)
docker compose --profile catalog run --rm import-catalog

# Só os 5 sets mais recentes (mais rápido, ~2 min)
docker compose --profile catalog run --rm import-catalog --recent 5

# Só um set específico
docker compose --profile catalog run --rm import-catalog --set sv8
```

Idempotente — pode rodar várias vezes, cards já existentes são pulados.

### ★ Lookup completo (nome E/OU número E/OU set, com matriz preço × condição × fonte)

Busca rica que combina filtros e devolve cartas, suas variantes, e a matriz
condição (NM/LP/MP/HP/DMG) × fonte (LigaPokemon/TCGplayer/eBay) numa única
resposta. Pelo menos um filtro é obrigatório.

```powershell
# Por nome (tolera typo)
curl "http://localhost:8080/api/v1/cards/lookup?name=charizard&with_signal=true"

# Por número exato dentro de um set (resolve nomes ambíguos)
curl "http://localhost:8080/api/v1/cards/lookup?set=sv8&number=199/191&with_signal=true"

# Só por número, em qualquer set
curl "http://localhost:8080/api/v1/cards/lookup?number=199/191&with_signal=true"

# Janela customizada (default 30 dias)
curl "http://localhost:8080/api/v1/cards/lookup?name=lucario&with_signal=true&window=14"
```

Resposta (resumida):
```json
{
  "query": {"name": "charizard", "with_signal": true, "window_days": 30},
  "matches": [{
    "card": {"id": "...", "name": "Charizard ex", "number": "199/191", "rarity": "Hyper Rare", ...},
    "set":  {"id": "...", "code": "sv8", "name": "Surging Sparks", ...},
    "variants": [{
      "variant": {"id": "...", "finish": "holo"},
      "signals_by_condition": {
        "variant_id": "...",
        "window_days": 30,
        "conditions": [
          {"condition": "NM",  "sources": [
            {"source": "ebay",        "sales_count": 110, "weighted_avg_brl": "315.20"},
            {"source": "ligapokemon", "sales_count":  98, "weighted_avg_brl": "298.40"},
            {"source": "tcgplayer",   "sales_count": 102, "weighted_avg_brl": "335.60"}
          ]},
          {"condition": "LP",  "sources": [...]},
          {"condition": "MP",  "sources": [...]}
        ]
      }
    }]
  }]
}
```

Sem `with_signal=true`, devolve cartas+variantes sem os preços (mais rápido).

### Detalhe da carta + suas variantes
```powershell
curl http://localhost:8080/api/v1/cards/CARD_ID
curl http://localhost:8080/api/v1/cards/CARD_ID/variants
```

### Sinal de preço de uma variante (★ rota mais útil)
```powershell
curl "http://localhost:8080/api/v1/variants/VARIANT_ID/signal"
curl "http://localhost:8080/api/v1/variants/VARIANT_ID/signal?condition=NM&window=14"
```

Resposta (resumida):
```json
{
  "variant_id": "...",
  "condition": "NM",
  "window_days": 30,
  "sources": [
    {"source": "ebay",        "sales_count": 110, "weighted_avg_brl": "315.20", "min_brl": "270.00", "max_brl": "365.00"},
    {"source": "ligapokemon", "sales_count":  98, "weighted_avg_brl": "298.40", "min_brl": "260.00", "max_brl": "340.00"},
    {"source": "tcgplayer",   "sales_count": 102, "weighted_avg_brl": "335.60", "min_brl": "295.00", "max_brl": "375.00"}
  ]
}
```

### Detalhe da loja
```powershell
curl http://localhost:8080/api/v1/stores/STORE_ID
```

### Estoque da loja, com sinal de preço por fonte
```powershell
curl "http://localhost:8080/api/v1/stores/STORE_ID/stock?with_signal=true"
```

### Registrar uma compra
```powershell
curl -X POST http://localhost:8080/api/v1/stores/STORE_ID/stock/purchase `
  -H "Content-Type: application/json" `
  -d '{
    "variant_id": "VARIANT_ID",
    "condition": "NM",
    "language": "en",
    "quantity": 3,
    "unit_cost_brl": "180.00",
    "notes": "comprei do João"
  }'
```

### Registrar uma venda
```powershell
curl -X POST http://localhost:8080/api/v1/stock-items/STOCK_ITEM_ID/sale `
  -H "Content-Type: application/json" `
  -d '{
    "quantity": 1,
    "unit_price_brl": "320.00",
    "notes": "vendido na loja física"
  }'
```

---

## O que já funciona vs o que ainda não

**Funciona:**
- CRUD de loja, cartas, variantes
- Compra/venda de estoque com transação e custo médio ponderado
- Busca de cartas por nome (tolerante a typo via `pg_trgm`)
- Sinal de preço por fonte agregando 30 dias de `price_daily`

**Ainda não funciona** (próximas fases):
- Scrapers reais — todo o `price_daily` que você está vendo é **simulado** pelo seed. Quando o scraper LigaPokemon for plugado, os números ficam reais sem mudar nenhuma rota.
- TCGplayer/eBay — vão usar API oficial, requerem credenciais que ainda não cadastramos.
- Auth + tabela `users` — `owner_id` hoje é um UUID determinístico do seed.
- Frontend Next.js — ainda não foi inicializado.
- Pagamentos — ver seção 9 do `CLAUDE.md`.

---

## Reset rápido

Se quiser zerar tudo:

```powershell
docker compose down -v       # apaga banco
docker compose up --build    # sobe limpo (build + migrate + seed + api)
```

---

## Problemas comuns

| Sintoma | Causa | Fix |
|---|---|---|
| `error during connect: ... pipe/dockerDesktopLinuxEngine` | Docker Desktop não está rodando | Abrir Docker Desktop, esperar o ícone da baleia ficar fixo |
| `Switch to Linux containers...` no menu do Docker | Está em modo Windows containers | Clicar nessa opção |
| `connection refused` ao acessar API | API ainda não terminou de subir | `docker compose logs -f api` e esperar |
| `migrate: Dirty database version N` | Migration anterior falhou | `docker compose run --rm migrate /app/migrate force <N-1>` e tenta de novo |
| Porta 5432, 8080 ou 8081 ocupada | Outra coisa rodando | Mudar a porta no `docker compose.yml` (lado esquerdo do `:`) |
| Build do Go demora demais | Primeira vez baixa todas as deps | Normal — só na primeira |
| `no such image` em `seed` ou `migrate` | Build não rodou | `docker compose build` antes de `up` |

Qualquer outro erro, me chama com a mensagem do log (`docker compose logs <serviço>`).
