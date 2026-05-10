package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/card"
)

// CardRepo persiste e consulta sets, cards e variantes.
// Mantemos os três numa única struct porque as queries são pequenas e quase
// sempre cruzam essas três tabelas.
type CardRepo struct {
	pool *pgxpool.Pool
}

// NewCardRepo devolve um repositório pronto para uso.
func NewCardRepo(pool *pgxpool.Pool) *CardRepo {
	return &CardRepo{pool: pool}
}

// ----------------------------------------------------------------------------
// Sets
// ----------------------------------------------------------------------------

const insertSetSQL = `
INSERT INTO card_sets (code, name, series, language, release_date, total_cards, image_url)
VALUES ($1, $2, NULLIF($3, ''), $4, $5, NULLIF($6, 0), NULLIF($7, ''))
RETURNING id, created_at, updated_at`

// CreateSet insere um novo set e devolve o ID gerado.
// Retorna ErrAlreadyExists quando o code já existe.
func (r *CardRepo) CreateSet(ctx context.Context, s *card.Set) error {
	err := r.pool.QueryRow(ctx, insertSetSQL,
		s.Code, s.Name, s.Series, string(s.Language),
		s.ReleaseDate, s.TotalCards, s.ImageURL,
	).Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == PgUniqueViolation {
			return ErrAlreadyExists
		}
		return fmt.Errorf("insert card_set: %w", err)
	}
	return nil
}

const selectSetByCodeSQL = `
SELECT id, code, name, COALESCE(series, ''), language, release_date,
       COALESCE(total_cards, 0), COALESCE(image_url, ''), created_at, updated_at
FROM card_sets WHERE code = $1`

// GetSetByCode busca um set pelo seu code (ex.: "sv7").
func (r *CardRepo) GetSetByCode(ctx context.Context, code string) (card.Set, error) {
	var s card.Set
	var lang string

	err := r.pool.QueryRow(ctx, selectSetByCodeSQL, code).Scan(
		&s.ID, &s.Code, &s.Name, &s.Series, &lang, &s.ReleaseDate,
		&s.TotalCards, &s.ImageURL, &s.CreatedAt, &s.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return card.Set{}, ErrNotFound
	}
	if err != nil {
		return card.Set{}, fmt.Errorf("select card_set: %w", err)
	}
	s.Language = card.Language(lang)
	return s, nil
}

// ----------------------------------------------------------------------------
// Cards
// ----------------------------------------------------------------------------

const insertCardSQL = `
INSERT INTO cards (
    set_id, number, name, rarity, supertype, subtypes, types,
    hp, illustrator, image_small_url, image_large_url, external_ids
) VALUES (
    $1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6, $7,
    NULLIF($8, 0), NULLIF($9, ''), NULLIF($10, ''), NULLIF($11, ''), $12
)
RETURNING id, created_at, updated_at`

// CreateCard insere uma carta. (set_id, number) é UNIQUE — colisão vira ErrAlreadyExists.
func (r *CardRepo) CreateCard(ctx context.Context, c *card.Card) error {
	external := c.ExternalIDs
	if external == nil {
		external = map[string]string{}
	}

	err := r.pool.QueryRow(ctx, insertCardSQL,
		c.SetID, c.Number, c.Name, c.Rarity, c.Supertype, c.Subtypes, c.Types,
		c.HP, c.Illustrator, c.ImageSmallURL, c.ImageLargeURL, external,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == PgUniqueViolation {
			return ErrAlreadyExists
		}
		return fmt.Errorf("insert card: %w", err)
	}
	return nil
}

const selectCardByIDSQL = `
SELECT id, set_id, number, name::text,
       COALESCE(rarity, ''), COALESCE(supertype, ''),
       COALESCE(subtypes, '{}'::text[]), COALESCE(types, '{}'::text[]),
       COALESCE(hp, 0), COALESCE(illustrator, ''),
       COALESCE(image_small_url, ''), COALESCE(image_large_url, ''),
       external_ids, created_at, updated_at
FROM cards WHERE id = $1`

// GetCardByID busca uma carta pelo seu UUID.
func (r *CardRepo) GetCardByID(ctx context.Context, id uuid.UUID) (card.Card, error) {
	var c card.Card
	err := r.pool.QueryRow(ctx, selectCardByIDSQL, id).Scan(
		&c.ID, &c.SetID, &c.Number, &c.Name,
		&c.Rarity, &c.Supertype, &c.Subtypes, &c.Types,
		&c.HP, &c.Illustrator, &c.ImageSmallURL, &c.ImageLargeURL,
		&c.ExternalIDs, &c.CreatedAt, &c.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return card.Card{}, ErrNotFound
	}
	if err != nil {
		return card.Card{}, fmt.Errorf("select card: %w", err)
	}
	return c, nil
}

const searchCardsByNameSQL = `
SELECT id, set_id, number, name::text,
       COALESCE(rarity, ''), COALESCE(supertype, ''),
       COALESCE(subtypes, '{}'::text[]), COALESCE(types, '{}'::text[]),
       COALESCE(hp, 0), COALESCE(illustrator, ''),
       COALESCE(image_small_url, ''), COALESCE(image_large_url, ''),
       external_ids, created_at, updated_at
FROM cards
WHERE name % $1                       -- pg_trgm similarity
ORDER BY similarity(name, $1) DESC
LIMIT $2`

// LookupCards combina filtros opcionais por nome (pg_trgm), número exato e
// código do set. Pelo menos um precisa ser informado — caller é responsável
// por essa validação (handler bloqueia antes).
//
// Ordenação: similaridade do nome primeiro (se houver), depois data de
// release do set DESC. Útil para a UI mostrar resultados mais recentes
// primeiro quando o usuário digita só o nome.
const lookupCardsSQL = `
SELECT
    c.id, c.set_id, c.number, c.name::text,
    COALESCE(c.rarity, ''), COALESCE(c.supertype, ''),
    COALESCE(c.subtypes, '{}'::text[]), COALESCE(c.types, '{}'::text[]),
    COALESCE(c.hp, 0), COALESCE(c.illustrator, ''),
    COALESCE(c.image_small_url, ''), COALESCE(c.image_large_url, ''),
    c.external_ids, c.created_at, c.updated_at,
    s.id, s.code, s.name, COALESCE(s.series, ''), s.language, s.release_date,
    COALESCE(s.total_cards, 0), COALESCE(s.image_url, ''), s.created_at, s.updated_at
FROM cards c
JOIN card_sets s ON s.id = c.set_id
WHERE
    (NULLIF($1, '') IS NULL OR c.name % $1)
    AND (NULLIF($2, '') IS NULL OR c.number = $2)
    AND (NULLIF($3, '') IS NULL OR s.code = $3)
ORDER BY
    CASE WHEN $1 <> '' THEN similarity(c.name::text, $1) ELSE 1.0 END DESC,
    s.release_date DESC NULLS LAST,
    c.number ASC
LIMIT $4`

// LookupCards executa o lookup combinado.
// Strings vazias significam "ignorar esse filtro".
func (r *CardRepo) LookupCards(
	ctx context.Context,
	name, number, setCode string,
	limit int,
) ([]card.CardWithSet, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	rows, err := r.pool.Query(ctx, lookupCardsSQL, name, number, setCode, limit)
	if err != nil {
		return nil, fmt.Errorf("lookup cards: %w", err)
	}
	defer rows.Close()

	var out []card.CardWithSet
	for rows.Next() {
		var cw card.CardWithSet
		var lang string
		if err := rows.Scan(
			&cw.Card.ID, &cw.Card.SetID, &cw.Card.Number, &cw.Card.Name,
			&cw.Card.Rarity, &cw.Card.Supertype, &cw.Card.Subtypes, &cw.Card.Types,
			&cw.Card.HP, &cw.Card.Illustrator, &cw.Card.ImageSmallURL, &cw.Card.ImageLargeURL,
			&cw.Card.ExternalIDs, &cw.Card.CreatedAt, &cw.Card.UpdatedAt,
			&cw.Set.ID, &cw.Set.Code, &cw.Set.Name, &cw.Set.Series, &lang,
			&cw.Set.ReleaseDate, &cw.Set.TotalCards, &cw.Set.ImageURL,
			&cw.Set.CreatedAt, &cw.Set.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan card+set: %w", err)
		}
		cw.Set.Language = card.Language(lang)
		out = append(out, cw)
	}
	return out, rows.Err()
}

// SearchCardsByName usa o índice GIN (pg_trgm) para busca tolerante a typos.
// Retorna no máximo `limit` resultados ordenados por similaridade.
func (r *CardRepo) SearchCardsByName(ctx context.Context, q string, limit int) ([]card.Card, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	rows, err := r.pool.Query(ctx, searchCardsByNameSQL, q, limit)
	if err != nil {
		return nil, fmt.Errorf("search cards by name: %w", err)
	}
	defer rows.Close()

	var out []card.Card
	for rows.Next() {
		var c card.Card
		if err := rows.Scan(
			&c.ID, &c.SetID, &c.Number, &c.Name,
			&c.Rarity, &c.Supertype, &c.Subtypes, &c.Types,
			&c.HP, &c.Illustrator, &c.ImageSmallURL, &c.ImageLargeURL,
			&c.ExternalIDs, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan card row: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ----------------------------------------------------------------------------
// Variants
// ----------------------------------------------------------------------------

const insertVariantSQL = `
INSERT INTO card_variants (card_id, finish, label, is_promo, notes)
VALUES ($1, $2, NULLIF($3, ''), $4, NULLIF($5, ''))
RETURNING id, created_at`

// CreateVariant insere uma variante. (card_id, finish, label) é UNIQUE.
func (r *CardRepo) CreateVariant(ctx context.Context, v *card.Variant) error {
	err := r.pool.QueryRow(ctx, insertVariantSQL,
		v.CardID, string(v.Finish), v.Label, v.IsPromo, v.Notes,
	).Scan(&v.ID, &v.CreatedAt)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == PgUniqueViolation {
			return ErrAlreadyExists
		}
		return fmt.Errorf("insert variant: %w", err)
	}
	return nil
}

const listVariantsByCardSQL = `
SELECT id, card_id, finish, COALESCE(label, ''), is_promo, COALESCE(notes, ''), created_at
FROM card_variants
WHERE card_id = $1
ORDER BY finish, label`

// ListVariantsByCard devolve todas as variantes de uma carta.
func (r *CardRepo) ListVariantsByCard(ctx context.Context, cardID uuid.UUID) ([]card.Variant, error) {
	rows, err := r.pool.Query(ctx, listVariantsByCardSQL, cardID)
	if err != nil {
		return nil, fmt.Errorf("list variants: %w", err)
	}
	defer rows.Close()

	var out []card.Variant
	for rows.Next() {
		var v card.Variant
		var finish string
		if err := rows.Scan(&v.ID, &v.CardID, &finish, &v.Label, &v.IsPromo, &v.Notes, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan variant: %w", err)
		}
		v.Finish = card.Finish(finish)
		out = append(out, v)
	}
	return out, rows.Err()
}
