# Multi-stage build: compila os três binários (api, migrate, seed) na imagem
# golang e copia só os artefatos para uma imagem alpine slim no runtime.
#
# Build context é a pasta backend/ (mesma do docker-compose.yml).

# --- Stage 1 — builder ------------------------------------------------------
FROM golang:1.25-alpine AS builder

# git é necessário para `go mod tidy` resolver alguns módulos via VCS.
RUN apk add --no-cache git

WORKDIR /src

# Cache de dependências: copia apenas o go.mod primeiro.
COPY go.mod ./
RUN go mod download || true

# Copia o restante do projeto e gera/sincroniza o go.sum.
COPY . .
RUN go mod tidy

# CGO desabilitado garante binário totalmente estático (roda em alpine sem libc extra).
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/api             ./cmd/api  && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/migrate         ./cmd/migrate && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/seed            ./cmd/seed && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/import-catalog  ./cmd/import-catalog

# --- Stage 2 — runtime ------------------------------------------------------
FROM alpine:3.20

# Postgres client é útil para debug ad-hoc dentro do container; certificados
# de raiz para chamadas HTTPS (ex.: BCB PTAX).
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /out/api             /app/api
COPY --from=builder /out/migrate         /app/migrate
COPY --from=builder /out/seed            /app/seed
COPY --from=builder /out/import-catalog  /app/import-catalog

# As migrations precisam estar no FS para o cmd/migrate encontrar.
COPY migrations /app/migrations

ENV MIGRATIONS_PATH=/app/migrations \
    HTTP_PORT=8080 \
    LOG_LEVEL=info

EXPOSE 8080

# Default: a API. Override com `command:` no docker-compose para rodar
# migrate ou seed.
ENTRYPOINT []
CMD ["/app/api"]
