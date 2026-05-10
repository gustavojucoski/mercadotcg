# MercadoTCG

Rastreador de preços e marketplace de Pokémon TCG. Agrega listagens ao vivo de LigaPokemon, TCGPlayer e eBay, resolve nomes e IDs de cartas automaticamente via [pokemontcg.io](https://pokemontcg.io/), e mantém histórico de preços por variante (holo, reverse holo, Master Ball Mirror, etc.).

## Quick Start

### Docker (recomendado)

```bash
# 1. Clone e entre no diretório
git clone https://github.com/gustavojucoski/mercadotcg
cd mercadotcg

# 2. Copie e ajuste as variáveis de ambiente
cp backend/.env.example backend/.env

# 3. Suba o stack (Postgres + API)
docker compose up --build

# 4. Aplique as migrations
docker compose exec api go run ./cmd/migrate up
```

A API ficará disponível em `http://localhost:8080`.

### Local (sem Docker)

```bash
cd backend
cp .env.example .env       # ajuste DATABASE_URL para seu Postgres local
go run ./cmd/migrate up    # cria o schema
go run ./cmd/api           # inicia o servidor
```

## Variáveis de Ambiente

Arquivo: `backend/.env` (baseado em `backend/.env.example`)

| Variável | Obrigatória | Descrição |
|---|---|---|
| `DATABASE_URL` | Sim | `postgres://user:pass@host:5432/dbname` |
| `HTTP_PORT` | Não (default 8080) | Porta do servidor HTTP |
| `LOG_LEVEL` | Não (default `info`) | `debug`, `info`, `warn`, `error` |
| `POKEMON_TCG_API_KEY` | Não | Eleva rate limit de 1k → 20k req/dia |
| `EBAY_CLIENT_ID` | Não | Credencial OAuth da eBay Browse API |
| `EBAY_CLIENT_SECRET` | Não | Credencial OAuth da eBay Browse API |

Sem `EBAY_CLIENT_ID`/`SECRET`, o scraper de eBay retorna `ErrNotConfigured` (os outros scrapers continuam funcionando normalmente).

## Endpoints

### Saúde

```bash
curl http://localhost:8080/healthz
# {"status":"ok"}
```

---

### Busca ao vivo — `/api/v1/external-search`

Agrega listagens em tempo real de LigaPokemon, TCGPlayer e eBay.

**Parâmetros:**

| Parâmetro | Descrição |
|---|---|
| `number` | Número da carta no set (ex: `276`) |
| `set` | ptcgoCode do set (ex: `ASC`) |
| `name` | Nome da carta (alternativo a number+set) |
| `limit` | Máximo de resultados por fonte (default `10`) |

Quando `number` + `set` são fornecidos, o nome e o TCGPlayer product ID são resolvidos automaticamente via pokemontcg.io.

```bash
# Buscar Pikachu ex 276/217 do set Ascended Heroes (ASC)
curl "http://localhost:8080/api/v1/external-search?number=276&set=ASC&limit=5"
```

```json
{
  "card": {
    "id": "me2pt5-276",
    "name": "Pikachu ex",
    "number": "276",
    "set_code": "ASC",
    "set_name": "Ascended Heroes"
  },
  "query": {
    "Name": "Pikachu ex",
    "Number": "276",
    "SetCode": "ASC",
    "Limit": 5
  },
  "fetched_at": "2026-05-10T12:00:00Z",
  "sources": [
    {
      "source": "ligapokemon",
      "duration_ms": 1240,
      "error": "",
      "results": [
        {
          "title": "Pikachu ex 276/217 - Ascended Heroes",
          "price": "480.00",
          "currency": "BRL",
          "condition": "NM",
          "url": "https://www.ligapokemon.com.br/..."
        }
      ]
    },
    {
      "source": "tcgplayer",
      "duration_ms": 890,
      "error": "",
      "results": [
        {
          "title": "Pikachu ex - Holofoil",
          "price": "85.00",
          "currency": "USD",
          "condition": "Near Mint",
          "url": "https://www.tcgplayer.com/product/676088/..."
        }
      ]
    },
    {
      "source": "ebay",
      "duration_ms": 0,
      "error": "ebay: not configured",
      "results": []
    }
  ]
}
```

```bash
# Buscar só pelo nome (sem resolução de catálogo)
curl "http://localhost:8080/api/v1/external-search?name=Pikachu+ex&limit=3"
```

---

### Busca no catálogo interno — `/api/v1/cards/search`

```bash
curl "http://localhost:8080/api/v1/cards/search?q=Pikachu&limit=5"
```

---

### Lookup rico de carta — `/api/v1/cards/lookup`

Retorna carta + variantes + sinal de preço histórico por fonte e condição.

**Parâmetros:**

| Parâmetro | Descrição |
|---|---|
| `name` | Nome parcial da carta |
| `number` | Número no set |
| `set` | Código do set |
| `with_signal` | `true` para incluir matriz de preços |
| `window` | Janela em dias para o sinal (default `30`) |
| `limit` | Máximo de cartas retornadas (default `10`) |

```bash
curl "http://localhost:8080/api/v1/cards/lookup?number=276&set=ASC&with_signal=true"
```

---

### Carta por ID — `/api/v1/cards/{id}`

```bash
curl "http://localhost:8080/api/v1/cards/01234567-89ab-cdef-0123-456789abcdef"
```

---

### Variantes de uma carta — `/api/v1/cards/{id}/variants`

```bash
curl "http://localhost:8080/api/v1/cards/01234567-89ab-cdef-0123-456789abcdef/variants"
```

---

### Sinal de preço de uma variante — `/api/v1/variants/{id}/signal`

```bash
curl "http://localhost:8080/api/v1/variants/01234567-89ab-cdef-0123-456789abcdef/signal?window=30"
```

---

### Lojas

```bash
# Criar loja
curl -X POST http://localhost:8080/api/v1/stores \
  -H "Content-Type: application/json" \
  -d '{"name":"Minha Loja TCG","slug":"minha-loja"}'

# Listar estoque de uma loja
curl "http://localhost:8080/api/v1/stores/{id}/stock"

# Registrar compra
curl -X POST http://localhost:8080/api/v1/stores/{id}/stock/purchase \
  -H "Content-Type: application/json" \
  -d '{"variant_id":"...","condition":"NM","quantity":3,"cost_brl":"150.00"}'

# Registrar venda
curl -X POST http://localhost:8080/api/v1/stock-items/{id}/sale \
  -H "Content-Type: application/json" \
  -d '{"quantity":1,"price_brl":"220.00"}'
```

---

### Importar catálogo pokemontcg.io

```bash
# Importar todas as cartas de um set
docker compose run --rm api go run ./cmd/import-catalog --set ASC

# Importar os N sets mais recentes
docker compose run --rm api go run ./cmd/import-catalog --recent 5
```

## Testes

```bash
cd backend

# Todos os testes (requer variáveis de ambiente para DB)
go test ./...

# Testes do handler (sem DB — httptest + scrapers reais)
go test -v -timeout 60s ./internal/handler/...

# Testes do cliente pokemontcg.io (chamadas reais à API)
go test -v -timeout 60s ./internal/pokemontcgio/...

# Testes do scraper LigaPokemon
go test -v -timeout 30s ./internal/scraper/ligapokemon/...

# Testes do TCGPlayer (requer TCGPLAYER_PUBLIC_KEY + TCGPLAYER_PRIVATE_KEY)
go test -v -timeout 30s ./internal/scraper/tcgplayer/...

# Testes do eBay (requer EBAY_CLIENT_ID + EBAY_CLIENT_SECRET)
EBAY_CLIENT_ID=xxx EBAY_CLIENT_SECRET=yyy go test -v -timeout 30s ./internal/scraper/ebay/...
```

### Teste rápido do endpoint principal

```bash
# Inicie a API em background
go run ./cmd/api &

# Busca Pikachu ex 276/217 do set ASC
curl -s "http://localhost:8080/api/v1/external-search?number=276&set=ASC&limit=5" | jq .
```

## Fontes de Preço

| Fonte | Método | Credenciais |
|---|---|---|
| LigaPokemon | HTML scraping (goquery) | Não |
| TCGPlayer | API oficial (pricepoints) | Não (mas requer TCGPlayer product ID, resolvido via pokemontcg.io) |
| eBay | Browse API OAuth2 | Sim — `EBAY_CLIENT_ID` + `EBAY_CLIENT_SECRET` |

O TCGPlayer product ID é resolvido automaticamente: `prices.pokemontcg.io/tcgplayer/{card-id}` redireciona para `tcgplayer.com/product/{id}`. O ID é extraído do redirect e injetado na query do scraper.

## Estrutura do Projeto

```
MercadoTCG/
├── backend/
│   ├── cmd/
│   │   ├── api/            # servidor HTTP (entrypoint)
│   │   ├── import-catalog/ # importador pokemontcg.io → DB
│   │   └── migrate/        # CLI de migrations
│   ├── internal/
│   │   ├── domain/         # tipos puros (card, pricing, store, matching)
│   │   ├── handler/        # adaptadores HTTP (chi)
│   │   ├── pokemontcgio/   # cliente pokemontcg.io com cache 24h
│   │   ├── repository/     # acesso a dados (postgres, pgx/v5)
│   │   ├── scraper/        # interface + implementações por fonte
│   │   │   ├── ligapokemon/
│   │   │   ├── tcgplayer/
│   │   │   └── ebay/
│   │   ├── service/        # regras de negócio (pricing, pricesignal)
│   │   ├── forex/          # cotação BRL/USD (BCB PTAX)
│   │   └── config/         # carregamento de env com validação
│   └── migrations/         # SQL versionado (.up/.down)
└── README.md
```
