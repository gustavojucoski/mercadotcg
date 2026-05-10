// Package postgres concentra os adaptadores de persistência sobre PostgreSQL.
// Cada repositório recebe um *pgxpool.Pool injetado e usa SQL escrito à mão —
// nunca um ORM. Este pacote também expõe os erros sentinela compartilhados.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Erros sentinela usados por todos os repositórios deste pacote.
// Convenção: handlers e services comparam com errors.Is(err, postgres.ErrNotFound).
var (
	ErrNotFound      = errors.New("postgres: registro não encontrado")
	ErrAlreadyExists = errors.New("postgres: registro já existe")
)

// PgUniqueViolation é o SQLSTATE retornado pelo Postgres quando uma UNIQUE
// constraint é violada. Mantido como const para evitar import cycle com pgconn
// nos repositórios.
const PgUniqueViolation = "23505"

// Connect monta um pool pgx com timeouts conservadores e devolve o handle.
// Recomendado chamar Close() do pool no shutdown da aplicação.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}

	cfg.MaxConns = 20
	cfg.MinConns = 2
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 10 * time.Minute
	cfg.HealthCheckPeriod = time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("conectar pool pgx: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return pool, nil
}
