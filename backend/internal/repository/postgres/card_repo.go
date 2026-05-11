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
INSERT INTO card_sets (code, name, series, series_id, tcg, language, release_date, total_cards, image_url)
VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, $7, NULLIF($8, 0), NULLIF($9, ''))
RETURNING id, created_at, updated_at`

// CreateSet insere um novo set e devolve o ID gerado.
// Retorna ErrAlreadyExists quando o code já existe.
func (r *CardRepo) CreateSet(ctx context.Context, s *card.Set) error {
	// Garante valor padrão para campo obrigatório.
	tcg := s.TCG
	if tcg == "" {
		tcg = "pokemon"
	}

	err := r.pool.QueryRow(ctx, insertSetSQL,
		s.Code, s.Name, s.Series, s.SeriesID, tcg, string(s.Language),
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
SELECT
    cs.id, cs.code, cs.name, COALESCE(cs.name_pt, ''),
    COALESCE(cr.name, cs.series, ''), COALESCE(cr.name_pt, ''),
    cs.series_id,
    COALESCE(cs.tcg, 'pokemon'), cs.language, cs.release_date,
    COALESCE(cs.total_cards, 0), COALESCE(cs.image_url, ''),
    cs.created_at, cs.updated_at
FROM card_sets cs
LEFT JOIN card_series cr ON cr.id = cs.series_id
WHERE cs.code = $1`

// GetSetByCode busca um set pelo seu code (ex.: "sv7").
func (r *CardRepo) GetSetByCode(ctx context.Context, code string) (card.Set, error) {
	var s card.Set
	var lang string

	err := r.pool.QueryRow(ctx, selectSetByCodeSQL, code).Scan(
		&s.ID, &s.Code, &s.Name, &s.NamePT,
		&s.Series, &s.SeriesPT, &s.SeriesID,
		&s.TCG, &lang, &s.ReleaseDate,
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

const updateSetNamePTSQL = `
UPDATE card_sets SET name_pt = $2, updated_at = now() WHERE id = $1`

// UpdateSetNamePT atualiza a tradução PT-BR de um set.
func (r *CardRepo) UpdateSetNamePT(ctx context.Context, id uuid.UUID, namePT string) error {
	tag, err := r.pool.Exec(ctx, updateSetNamePTSQL, id, namePT)
	if err != nil {
		return fmt.Errorf("update set name_pt: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ----------------------------------------------------------------------------
// Series
// ----------------------------------------------------------------------------

const upsertSeriesSQL = `
INSERT INTO card_series (name, tcg)
VALUES ($1, $2)
ON CONFLICT (name, tcg) DO UPDATE SET name = EXCLUDED.name
RETURNING id, name, COALESCE(name_pt, ''), tcg, created_at`

// UpsertSeries garante que a série existe e retorna o objeto com ID.
func (r *CardRepo) UpsertSeries(ctx context.Context, name, tcg string) (card.Series, error) {
	var s card.Series
	err := r.pool.QueryRow(ctx, upsertSeriesSQL, name, tcg).Scan(
		&s.ID, &s.Name, &s.NamePT, &s.TCG, &s.CreatedAt,
	)
	if err != nil {
		return card.Series{}, fmt.Errorf("upsert series: %w", err)
	}
	return s, nil
}

const updateSeriesNamePTSQL = `
UPDATE card_series SET name_pt = $2 WHERE id = $1`

// UpdateSeriesNamePT atualiza a tradução PT-BR de uma série.
func (r *CardRepo) UpdateSeriesNamePT(ctx context.Context, id uuid.UUID, namePT string) error {
	tag, err := r.pool.Exec(ctx, updateSeriesNamePTSQL, id, namePT)
	if err != nil {
		return fmt.Errorf("update series name_pt: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

const listSeriesSQL = `
SELECT id, name, COALESCE(name_pt, ''), tcg, created_at
FROM card_series
WHERE ($1 = '' OR tcg = $1)
ORDER BY tcg, name`

// ListSeries lista séries, opcionalmente filtradas por TCG.
func (r *CardRepo) ListSeries(ctx context.Context, tcg string) ([]card.Series, error) {
	rows, err := r.pool.Query(ctx, listSeriesSQL, tcg)
	if err != nil {
		return nil, fmt.Errorf("list series: %w", err)
	}
	defer rows.Close()

	var out []card.Series
	for rows.Next() {
		var s card.Series
		if err := rows.Scan(&s.ID, &s.Name, &s.NamePT, &s.TCG, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan series: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ----------------------------------------------------------------------------
// Cards
// ----------------------------------------------------------------------------

const insertCardSQL = `
INSERT INTO cards (
    set_id, number, collector_number, name, name_pt, rarity, supertype, subtypes, types,
    hp, illustrator, image_small_url, image_large_url, external_ids
) VALUES (
    $1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), $8, $9,
    NULLIF($10, 0), NULLIF($11, ''), NULLIF($12, ''), NULLIF($13, ''), $14
)
RETURNING id, created_at, updated_at`

// CreateCard insere uma carta. (set_id, number) é UNIQUE — colisão vira ErrAlreadyExists.
func (r *CardRepo) CreateCard(ctx context.Context, c *card.Card) error {
	external := c.ExternalIDs
	if external == nil {
		external = map[string]string{}
	}

	err := r.pool.QueryRow(ctx, insertCardSQL,
		c.SetID, c.Number, c.CollectorNumber, c.Name, c.NamePT,
		c.Rarity, c.Supertype, c.Subtypes, c.Types,
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
SELECT id, set_id, number, COALESCE(collector_number, ''), name::text, COALESCE(name_pt, ''),
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
		&c.ID, &c.SetID, &c.Number, &c.CollectorNumber, &c.Name, &c.NamePT,
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
SELECT id, set_id, number, COALESCE(collector_number, ''), name::text, COALESCE(name_pt, ''),
       COALESCE(rarity, ''), COALESCE(supertype, ''),
       COALESCE(subtypes, '{}'::text[]), COALESCE(types, '{}'::text[]),
       COALESCE(hp, 0), COALESCE(illustrator, ''),
       COALESCE(image_small_url, ''), COALESCE(image_large_url, ''),
       external_ids, created_at, updated_at
FROM cards
WHERE name % $1
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
    c.id, c.set_id, c.number, COALESCE(c.collector_number, ''), c.name::text, COALESCE(c.name_pt, ''),
    COALESCE(c.rarity, ''), COALESCE(c.supertype, ''),
    COALESCE(c.subtypes, '{}'::text[]), COALESCE(c.types, '{}'::text[]),
    COALESCE(c.hp, 0), COALESCE(c.illustrator, ''),
    COALESCE(c.image_small_url, ''), COALESCE(c.image_large_url, ''),
    c.external_ids, c.created_at, c.updated_at,
    s.id, s.code, s.name, COALESCE(s.name_pt, ''),
    COALESCE(cr.name, s.series, ''), COALESCE(cr.name_pt, ''),
    s.series_id,
    COALESCE(s.tcg, 'pokemon'),
    s.language, s.release_date,
    COALESCE(s.total_cards, 0), COALESCE(s.image_url, ''), s.created_at, s.updated_at
FROM cards c
JOIN card_sets s ON s.id = c.set_id
LEFT JOIN card_series cr ON cr.id = s.series_id
WHERE
    (NULLIF($1, '') IS NULL OR c.name % $1)
    AND (NULLIF($2, '') IS NULL OR c.number = $2 OR c.collector_number = $2)
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
			&cw.Card.ID, &cw.Card.SetID, &cw.Card.Number, &cw.Card.CollectorNumber,
			&cw.Card.Name, &cw.Card.NamePT,
			&cw.Card.Rarity, &cw.Card.Supertype, &cw.Card.Subtypes, &cw.Card.Types,
			&cw.Card.HP, &cw.Card.Illustrator, &cw.Card.ImageSmallURL, &cw.Card.ImageLargeURL,
			&cw.Card.ExternalIDs, &cw.Card.CreatedAt, &cw.Card.UpdatedAt,
			&cw.Set.ID, &cw.Set.Code, &cw.Set.Name, &cw.Set.NamePT,
			&cw.Set.Series, &cw.Set.SeriesPT, &cw.Set.SeriesID,
			&cw.Set.TCG,
			&lang, &cw.Set.ReleaseDate,
			&cw.Set.TotalCards, &cw.Set.ImageURL,
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
			&c.ID, &c.SetID, &c.Number, &c.CollectorNumber, &c.Name, &c.NamePT,
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

const updateCardImagesSQL = `
UPDATE cards SET image_small_url = $2, image_large_url = $3 WHERE id = $1`

// UpdateCardImages atualiza as URLs de imagem de uma carta.
// Usado pelo import-catalog após baixar imagens localmente.
// Retorna ErrNotFound se o cardID não existir no banco.
func (r *CardRepo) UpdateCardImages(ctx context.Context, cardID uuid.UUID, smallURL, largeURL string) error {
	tag, err := r.pool.Exec(ctx, updateCardImagesSQL, cardID, smallURL, largeURL)
	if err != nil {
		return fmt.Errorf("update card images: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

const updateCardNamePTSQL = `
UPDATE cards SET name_pt = $2, updated_at = now() WHERE id = $1`

// UpdateCardNamePT atualiza a tradução PT-BR de uma carta.
func (r *CardRepo) UpdateCardNamePT(ctx context.Context, id uuid.UUID, namePT string) error {
	tag, err := r.pool.Exec(ctx, updateCardNamePTSQL, id, namePT)
	if err != nil {
		return fmt.Errorf("update card name_pt: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ----------------------------------------------------------------------------
// Variant display — batch lookup for stock enrichment
// ----------------------------------------------------------------------------

// VariantDisplay reúne os campos de exibição de uma variante para uso no
// endpoint de estoque, evitando N+1 queries.
type VariantDisplay struct {
	VariantID     uuid.UUID
	Finish        string
	Label         string
	CardName      string
	CardNumber    string
	SetName       string
	SetCode       string
	ImageSmallURL string
}

const getVariantDisplayBatchSQL = `
SELECT
    cv.id,
    cv.finish::text,
    COALESCE(cv.label, ''),
    c.name::text,
    c.number,
    cs.name,
    cs.code,
    COALESCE(c.image_small_url, '')
FROM card_variants cv
JOIN cards c     ON c.id  = cv.card_id
JOIN card_sets cs ON cs.id = c.set_id
WHERE cv.id = ANY($1)`

// GetVariantDisplayBatch busca os dados de exibição de um conjunto de variantes
// em uma única query. Retorna um map keyed por variant_id.
// IDs não encontrados simplesmente não aparecem no map (sem erro).
func (r *CardRepo) GetVariantDisplayBatch(
	ctx context.Context,
	variantIDs []uuid.UUID,
) (map[uuid.UUID]VariantDisplay, error) {
	if len(variantIDs) == 0 {
		return map[uuid.UUID]VariantDisplay{}, nil
	}

	rows, err := r.pool.Query(ctx, getVariantDisplayBatchSQL, variantIDs)
	if err != nil {
		return nil, fmt.Errorf("get variant display batch: %w", err)
	}
	defer rows.Close()

	out := make(map[uuid.UUID]VariantDisplay, len(variantIDs))
	for rows.Next() {
		var d VariantDisplay
		if err := rows.Scan(
			&d.VariantID, &d.Finish, &d.Label,
			&d.CardName, &d.CardNumber,
			&d.SetName, &d.SetCode,
			&d.ImageSmallURL,
		); err != nil {
			return nil, fmt.Errorf("scan variant display: %w", err)
		}
		out[d.VariantID] = d
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
