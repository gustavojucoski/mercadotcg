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

// SetWithSeries junta um Set com os nomes da sua série (EN e PT-BR).
// Usado na listagem pública de sets para exibir o agrupamento por série.
type SetWithSeries struct {
	card.Set
	SeriesName   string `json:"series"`
	SeriesNamePT string `json:"series_pt"`
}

// CardWithVariants junta uma Card com todas as suas variantes.
// Usado na listagem de cartas de um set para evitar N+1 queries de variantes.
type CardWithVariants struct {
	card.Card
	Variants []card.Variant `json:"variants"`
}

// AutocompleteResult é o payload minimalista para a sugestão de busca rápida.
type AutocompleteResult struct {
	ID              uuid.UUID `json:"id"`
	Name            string    `json:"name"`
	NamePT          string    `json:"name_pt,omitempty"`
	CollectorNumber string    `json:"collector_number"`
	SetCode         string    `json:"set_code"`
	SetName         string    `json:"set_name"`
	ImageSmallURL   string    `json:"image_small_url,omitempty"`
	Slug            string    `json:"slug"` // "{set_code}-{collector_number}"
}

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
INSERT INTO card_sets (code, name, series, series_id, tcg, language, release_date, total_cards, printed_total, image_url, symbol_url)
VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, $7, NULLIF($8, 0), NULLIF($9, 0), NULLIF($10, ''), NULLIF($11, ''))
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
		s.ReleaseDate, s.TotalCards, s.PrintedTotal, s.ImageURL, s.SymbolURL,
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

const upsertSetSQL = `
INSERT INTO card_sets (code, name, name_pt, name_en, series, series_id, tcg, language, release_date, total_cards, printed_total, image_url, symbol_url, import_source)
VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), $6, $7, $8, $9, NULLIF($10, 0), NULLIF($11, 0), NULLIF($12, ''), NULLIF($13, ''), $14)
ON CONFLICT (code) DO UPDATE SET
    -- Preserve an already-English name when the incoming name is Japanese/CJK,
    -- so re-imports don't clobber DeepL-translated or manually-curated names.
    name          = CASE
                      WHEN card_sets.name ~ '[一-龯ぁ-んァ-ヾ]'
                        OR NOT (EXCLUDED.name ~ '[一-龯ぁ-んァ-ヾ]')
                      THEN EXCLUDED.name
                      ELSE card_sets.name
                    END,
    name_pt       = COALESCE(EXCLUDED.name_pt, card_sets.name_pt),
    name_en       = COALESCE(card_sets.name_en, EXCLUDED.name_en),
    series        = COALESCE(EXCLUDED.series, card_sets.series),
    series_id     = COALESCE(EXCLUDED.series_id, card_sets.series_id),
    tcg           = EXCLUDED.tcg,
    release_date  = COALESCE(EXCLUDED.release_date, card_sets.release_date),
    language      = EXCLUDED.language,
    total_cards   = COALESCE(EXCLUDED.total_cards, card_sets.total_cards),
    printed_total = COALESCE(EXCLUDED.printed_total, card_sets.printed_total),
    image_url     = COALESCE(EXCLUDED.image_url, card_sets.image_url),
    symbol_url    = COALESCE(EXCLUDED.symbol_url, card_sets.symbol_url),
    import_source = EXCLUDED.import_source,
    updated_at    = now()
WHERE card_sets.import_source <> 'manual'
RETURNING id, created_at, updated_at`

// UpsertSet inserts or updates a set by code. Returns the set with ID populated.
// Unlike CreateSet, this is idempotent and safe for repeated import runs.
// Sets with import_source = 'manual' are never modified by the ON CONFLICT path.
//
// When the SQL WHERE guard fires (existing row has import_source = 'manual'),
// the RETURNING clause produces no rows. We detect pgx.ErrNoRows here and fall
// back to GetSetByCode so the caller always gets a valid s.ID without an error.
func (r *CardRepo) UpsertSet(ctx context.Context, s *card.Set) error {
	tcg := s.TCG
	if tcg == "" {
		tcg = "pokemon"
	}
	importSource := s.ImportSource
	if importSource == "" {
		importSource = "tcgdex_legacy"
	}

	err := r.pool.QueryRow(ctx, upsertSetSQL,
		s.Code, s.Name, s.NamePT, s.NameEN, s.Series, s.SeriesID, tcg, string(s.Language),
		s.ReleaseDate, s.TotalCards, s.PrintedTotal, s.ImageURL, s.SymbolURL, importSource,
	).Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		// The manual guard in the SQL WHERE clause fired — the row was not
		// updated and RETURNING returned nothing. Fetch the existing row so the
		// caller receives a valid s.ID without treating this as an error.
		existing, fetchErr := r.GetSetByCode(ctx, s.Code)
		if fetchErr != nil {
			return fmt.Errorf("upsert blocked by manual guard, fetch fallback: %w", fetchErr)
		}
		*s = existing
		return nil
	}
	if err != nil {
		return fmt.Errorf("upsert card_set: %w", err)
	}
	return nil
}

const selectSetByCodeSQL = `
SELECT
    cs.id, cs.code, cs.name, COALESCE(cs.name_pt, ''), COALESCE(cs.name_en, ''),
    COALESCE(cr.name, cs.series, ''), COALESCE(cr.name_pt, ''),
    cs.series_id,
    COALESCE(cs.tcg, 'pokemon'), cs.language, cs.release_date,
    COALESCE(cs.total_cards, 0), COALESCE(cs.printed_total, 0),
    COALESCE(cs.image_url, ''), COALESCE(cs.symbol_url, ''),
    COALESCE(cs.import_source, ''),
    cs.created_at, cs.updated_at
FROM card_sets cs
LEFT JOIN card_series cr ON cr.id = cs.series_id
WHERE cs.code = $1`

// GetSetByCode busca um set pelo seu code (ex.: "sv7").
func (r *CardRepo) GetSetByCode(ctx context.Context, code string) (card.Set, error) {
	var s card.Set
	var lang string

	err := r.pool.QueryRow(ctx, selectSetByCodeSQL, code).Scan(
		&s.ID, &s.Code, &s.Name, &s.NamePT, &s.NameEN,
		&s.Series, &s.SeriesPT, &s.SeriesID,
		&s.TCG, &lang, &s.ReleaseDate,
		&s.TotalCards, &s.PrintedTotal, &s.ImageURL, &s.SymbolURL,
		&s.ImportSource,
		&s.CreatedAt, &s.UpdatedAt,
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

const updateSetNameENSQL = `
UPDATE card_sets SET name_en = $2, updated_at = now() WHERE id = $1`

// UpdateSetNameEN atualiza o nome em inglês de um set não-EN (ex.: JA, KO, ZH-TW).
// Preenchido manualmente pelo admin via UI; nunca sobrescrito por imports automáticos.
func (r *CardRepo) UpdateSetNameEN(ctx context.Context, id uuid.UUID, nameEN string) error {
	tag, err := r.pool.Exec(ctx, updateSetNameENSQL, id, nameEN)
	if err != nil {
		return fmt.Errorf("update set name_en: %w", err)
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
ON CONFLICT (name, tcg) DO UPDATE SET name = EXCLUDED.name -- no-op: força RETURNING a devolver a linha existente
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

const upsertSeriesWithPTSQL = `
INSERT INTO card_series (name, name_pt, tcg)
VALUES ($1, NULLIF($2, ''), $3)
ON CONFLICT (name, tcg) DO UPDATE SET
    name_pt = COALESCE(EXCLUDED.name_pt, card_series.name_pt)
RETURNING id, name, COALESCE(name_pt, ''), tcg, created_at`

// UpsertSeriesWithPT upserts a series and sets name_pt when provided.
// Used by the TCGDex importer which has PT-BR series names for TCG Pocket sets.
func (r *CardRepo) UpsertSeriesWithPT(ctx context.Context, name, namePT, tcg string) (card.Series, error) {
	var s card.Series
	err := r.pool.QueryRow(ctx, upsertSeriesWithPTSQL, name, namePT, tcg).Scan(
		&s.ID, &s.Name, &s.NamePT, &s.TCG, &s.CreatedAt,
	)
	if err != nil {
		return card.Series{}, fmt.Errorf("upsert series with pt: %w", err)
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

// upsertCardSQL uses (set_id, number) as the natural key (matches the existing UNIQUE constraint).
// On conflict, updates name, name_pt, collector_number, image URLs, and import_source so re-runs are safe.
// import_source is included so that cards imported via Scrydex are correctly tagged
// and ListCardsForPTEnrichment (which filters on import_source = 'scrydex') works as expected.
const upsertCardSQL = `
INSERT INTO cards (
    set_id, number, collector_number, name, name_pt,
    rarity, supertype, subtypes, types,
    hp, illustrator, image_small_url, image_large_url, external_ids,
    import_source
) VALUES (
    $1, $2, $3, $4, NULLIF($5, ''),
    NULLIF($6, ''), NULLIF($7, ''), $8, $9,
    NULLIF($10, 0), NULLIF($11, ''), NULLIF($12, ''), NULLIF($13, ''), $14,
    $15
)
ON CONFLICT (set_id, number) DO UPDATE SET
    name            = EXCLUDED.name,
    name_pt         = COALESCE(EXCLUDED.name_pt, cards.name_pt),
    collector_number= EXCLUDED.collector_number,
    rarity          = COALESCE(EXCLUDED.rarity, cards.rarity),
    -- Never overwrite an existing image URL: UpdateCardImages is the sole owner
    -- of image_*_url after the initial insert. This prevents re-imports from
    -- clobbering S3 URLs with temporary Scrydex CDN placeholders.
    image_small_url = CASE WHEN cards.image_small_url IS NULL OR cards.image_small_url = '' THEN EXCLUDED.image_small_url ELSE cards.image_small_url END,
    image_large_url = CASE WHEN cards.image_large_url IS NULL OR cards.image_large_url = '' THEN EXCLUDED.image_large_url ELSE cards.image_large_url END,
    import_source   = EXCLUDED.import_source,
    updated_at      = now()
RETURNING id, created_at, updated_at`

// UpsertCard inserts or updates a card. The natural key is (set_id, number).
// Safe for repeated import runs. Populates c.ID, c.CreatedAt, c.UpdatedAt.
func (r *CardRepo) UpsertCard(ctx context.Context, c *card.Card) error {
	external := c.ExternalIDs
	if external == nil {
		external = map[string]string{}
	}
	importSource := c.ImportSource
	if importSource == "" {
		importSource = "tcgdex_legacy"
	}

	err := r.pool.QueryRow(ctx, upsertCardSQL,
		c.SetID, c.Number, c.CollectorNumber, c.Name, c.NamePT,
		c.Rarity, c.Supertype, c.Subtypes, c.Types,
		c.HP, c.Illustrator, c.ImageSmallURL, c.ImageLargeURL, external,
		importSource,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert card: %w", err)
	}
	return nil
}

// upsertVariantSQL silently skips duplicates.
// Uses ON CONFLICT DO NOTHING without a conflict target because the unique
// index idx_card_variants_natural_key is an expression index on
// (card_id, finish, COALESCE(label, '')), which PostgreSQL accepts for inference.
// Omitting the target is simpler and still safe — any unique violation is silently dropped.
const upsertVariantSQL = `
INSERT INTO card_variants (card_id, finish, label, is_promo, notes)
VALUES ($1, $2::variant_finish, NULLIF($3, ''), $4, NULLIF($5, ''))
ON CONFLICT DO NOTHING`

// UpsertVariant inserts a variant if it does not already exist.
// Silently skips duplicates (ON CONFLICT DO NOTHING) — safe for re-runs.
func (r *CardRepo) UpsertVariant(ctx context.Context, v *card.Variant) error {
	_, err := r.pool.Exec(ctx, upsertVariantSQL,
		v.CardID, string(v.Finish), v.Label, v.IsPromo, v.Notes,
	)
	if err != nil {
		return fmt.Errorf("upsert variant: %w", err)
	}
	return nil
}

const selectCardByIDSQL = `
SELECT id, set_id, number, COALESCE(collector_number, ''), name::text, COALESCE(name_pt, ''),
       COALESCE(rarity, ''), COALESCE(supertype, ''),
       COALESCE(subtypes, '{}'::text[]), COALESCE(types, '{}'::text[]),
       COALESCE(hp, 0), COALESCE(illustrator, ''),
       COALESCE(image_small_url, ''), COALESCE(image_large_url, ''),
       COALESCE(image_url_pt, ''),
       external_ids, created_at, updated_at
FROM cards WHERE id = $1`

// GetCardByID busca uma carta pelo seu UUID.
func (r *CardRepo) GetCardByID(ctx context.Context, id uuid.UUID) (card.Card, error) {
	var c card.Card
	err := r.pool.QueryRow(ctx, selectCardByIDSQL, id).Scan(
		&c.ID, &c.SetID, &c.Number, &c.CollectorNumber, &c.Name, &c.NamePT,
		&c.Rarity, &c.Supertype, &c.Subtypes, &c.Types,
		&c.HP, &c.Illustrator, &c.ImageSmallURL, &c.ImageLargeURL,
		&c.ImageURLPT,
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
    COALESCE(c.image_url_pt, ''),
    c.external_ids, c.created_at, c.updated_at,
    s.id, s.code, s.name, COALESCE(s.name_pt, ''), COALESCE(s.name_en, ''),
    COALESCE(cr.name, s.series, ''), COALESCE(cr.name_pt, ''),
    s.series_id,
    COALESCE(s.tcg, 'pokemon'),
    s.language, s.release_date,
    COALESCE(s.total_cards, 0), COALESCE(s.printed_total, 0),
    COALESCE(s.image_url, ''), COALESCE(s.symbol_url, ''),
    s.created_at, s.updated_at
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
			&cw.Card.ImageURLPT,
			&cw.Card.ExternalIDs, &cw.Card.CreatedAt, &cw.Card.UpdatedAt,
			&cw.Set.ID, &cw.Set.Code, &cw.Set.Name, &cw.Set.NamePT, &cw.Set.NameEN,
			&cw.Set.Series, &cw.Set.SeriesPT, &cw.Set.SeriesID,
			&cw.Set.TCG,
			&lang, &cw.Set.ReleaseDate,
			&cw.Set.TotalCards, &cw.Set.PrintedTotal,
			&cw.Set.ImageURL, &cw.Set.SymbolURL,
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

const updateCardImagePTSQL = `
UPDATE cards SET image_url_pt = $2, updated_at = now() WHERE id = $1`

// UpdateCardImagePT atualiza a URL da imagem PT-BR de uma carta.
// Usado pelo import-catalog após baixar a imagem PT localmente.
// Retorna ErrNotFound se o cardID não existir no banco.
func (r *CardRepo) UpdateCardImagePT(ctx context.Context, cardID uuid.UUID, imageURL string) error {
	tag, err := r.pool.Exec(ctx, updateCardImagePTSQL, cardID, imageURL)
	if err != nil {
		return fmt.Errorf("update card image_url_pt: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

const updateSetImageURLSQL = `
UPDATE card_sets SET image_url = $1, updated_at = NOW() WHERE id = $2`

// UpdateSetImageURL atualiza a URL da imagem (logo) de um set.
// Usado pelo import-catalog após baixar o logo localmente.
func (r *CardRepo) UpdateSetImageURL(ctx context.Context, setID uuid.UUID, imageURL string) error {
	tag, err := r.pool.Exec(ctx, updateSetImageURLSQL, imageURL, setID)
	if err != nil {
		return fmt.Errorf("update set image_url: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

const updateSetSymbolURLSQL = `
UPDATE card_sets SET symbol_url = $1, updated_at = NOW() WHERE id = $2`

// UpdateSetSymbolURL updates the symbol image URL of a set.
// Used by import-catalog after downloading the symbol locally.
func (r *CardRepo) UpdateSetSymbolURL(ctx context.Context, setID uuid.UUID, symbolURL string) error {
	tag, err := r.pool.Exec(ctx, updateSetSymbolURLSQL, symbolURL, setID)
	if err != nil {
		return fmt.Errorf("update set symbol_url: %w", err)
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

// updateCardPTSQL only writes non-empty values, preserving whatever is already
// stored. This lets the enricher run multiple times safely and fill each field
// independently as TCGDex data becomes available.
const updateCardPTSQL = `
UPDATE cards
SET
    name_pt      = CASE WHEN $2 <> '' THEN $2 ELSE name_pt END,
    image_url_pt = CASE WHEN $3 <> '' THEN $3 ELSE image_url_pt END,
    updated_at   = now()
WHERE id = $1`

// CardEnrichmentCandidate is the minimal projection needed by the PT enrichment
// loop: the DB id, the collector number to build the TCGDex card ID, and the
// set code to scope the TCGDex request and the S3 key.
type CardEnrichmentCandidate struct {
	ID              uuid.UUID
	CollectorNumber string
	SetCode         string
}

const listCardsForPTEnrichmentSQL = `
SELECT c.id, c.collector_number, cs.code AS set_code
FROM cards c
JOIN card_sets cs ON cs.id = c.set_id
WHERE c.import_source = 'scrydex'
  AND (c.name_pt IS NULL OR c.image_url_pt IS NULL)
LIMIT $1`

// ListCardsForPTEnrichment returns up to limit cards imported from Scrydex that
// are still missing a PT-BR name or PT-BR image.
func (r *CardRepo) ListCardsForPTEnrichment(ctx context.Context, limit int) ([]CardEnrichmentCandidate, error) {
	rows, err := r.pool.Query(ctx, listCardsForPTEnrichmentSQL, limit)
	if err != nil {
		return nil, fmt.Errorf("list cards for pt enrichment: %w", err)
	}
	defer rows.Close()

	var out []CardEnrichmentCandidate
	for rows.Next() {
		var c CardEnrichmentCandidate
		if err := rows.Scan(&c.ID, &c.CollectorNumber, &c.SetCode); err != nil {
			return nil, fmt.Errorf("scan enrichment candidate: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpdateCardPT writes namePT and/or imageURLPT only when the supplied strings
// are non-empty, leaving already-populated fields untouched.
func (r *CardRepo) UpdateCardPT(ctx context.Context, cardID uuid.UUID, namePT, imageURLPT string) error {
	tag, err := r.pool.Exec(ctx, updateCardPTSQL, cardID, namePT, imageURLPT)
	if err != nil {
		return fmt.Errorf("update card pt fields: %w", err)
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

// ----------------------------------------------------------------------------
// Public catalog — sets, cards, autocomplete
// ----------------------------------------------------------------------------

const listSetsByTCGSQL = `
SELECT cs.id, cs.code, cs.name, COALESCE(cs.name_pt, ''), COALESCE(cs.name_en, ''),
       cs.series_id,
       COALESCE(cr.name, cs.series, '') AS series_name,
       COALESCE(cr.name_pt, '') AS series_name_pt,
       COALESCE(cs.tcg, 'pokemon') AS tcg,
       cs.language, cs.release_date, COALESCE(cs.total_cards, 0), COALESCE(cs.printed_total, 0),
       COALESCE(cs.image_url, '') AS image_url,
       COALESCE(cs.symbol_url, '') AS symbol_url,
       cs.created_at, cs.updated_at,
       COUNT(*) OVER() AS total
FROM card_sets cs
LEFT JOIN card_series cr ON cr.id = cs.series_id
WHERE cs.tcg = $1
  AND ($2::uuid IS NULL OR cs.series_id = $2)
ORDER BY cs.release_date DESC NULLS LAST, cs.name ASC
LIMIT $3 OFFSET $4`

// ListSetsByTCG lista sets paginados, filtrados por TCG e opcionalmente por série.
// Retorna os sets e o total de linhas (para paginação do caller).
func (r *CardRepo) ListSetsByTCG(ctx context.Context, tcg string, seriesID *uuid.UUID, page, limit int) ([]SetWithSeries, int, error) {
	if limit <= 0 || limit > 500 {
		limit = 30
	}
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit

	rows, err := r.pool.Query(ctx, listSetsByTCGSQL, tcg, seriesID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list sets by tcg: %w", err)
	}
	defer rows.Close()

	var out []SetWithSeries
	var total int
	for rows.Next() {
		var s SetWithSeries
		var lang string
		if err := rows.Scan(
			&s.ID, &s.Code, &s.Name, &s.NamePT, &s.NameEN,
			&s.SeriesID,
			&s.SeriesName, &s.SeriesNamePT,
			&s.TCG,
			&lang, &s.ReleaseDate, &s.TotalCards, &s.PrintedTotal,
			&s.ImageURL, &s.SymbolURL,
			&s.CreatedAt, &s.UpdatedAt,
			&total,
		); err != nil {
			return nil, 0, fmt.Errorf("scan set with series: %w", err)
		}
		s.Language = card.Language(lang)
		// Expõe series no campo herdado de card.Set para retrocompatibilidade.
		s.Series = s.SeriesName
		s.SeriesPT = s.SeriesNamePT
		out = append(out, s)
	}
	return out, total, rows.Err()
}

const listCardsBySetCodeSQL = `
SELECT c.id, c.set_id, c.number, COALESCE(c.collector_number, ''), c.name::text, COALESCE(c.name_pt, ''),
       COALESCE(c.rarity, ''), COALESCE(c.supertype, ''),
       COALESCE(c.subtypes, '{}'::text[]), COALESCE(c.types, '{}'::text[]),
       COALESCE(c.hp, 0), COALESCE(c.illustrator, ''),
       COALESCE(c.image_small_url, ''), COALESCE(c.image_large_url, ''),
       COALESCE(c.image_url_pt, ''),
       c.external_ids, c.created_at, c.updated_at,
       COUNT(*) OVER() AS total
FROM cards c
JOIN card_sets s ON s.id = c.set_id AND s.code = $1
ORDER BY NULLIF(regexp_replace(c.collector_number, '\D', '', 'g'), '')::int NULLS LAST, c.collector_number
LIMIT $2 OFFSET $3`

const listVariantsByCardsSQL = `
SELECT id, card_id, finish::text, COALESCE(label, ''), is_promo, COALESCE(notes, ''), created_at
FROM card_variants
WHERE card_id = ANY($1)
ORDER BY card_id, finish, label`

// ListCardsBySetCode lista cartas paginadas de um set, ordenadas pelo collector_number
// numérico. Cada carta vem acompanhada de suas variantes (via segunda query batch).
func (r *CardRepo) ListCardsBySetCode(ctx context.Context, setCode string, page, limit int) ([]CardWithVariants, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 60
	}
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit

	rows, err := r.pool.Query(ctx, listCardsBySetCodeSQL, setCode, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list cards by set: %w", err)
	}
	defer rows.Close()

	var cards []CardWithVariants
	var cardIDs []uuid.UUID
	var total int

	for rows.Next() {
		var cw CardWithVariants
		if err := rows.Scan(
			&cw.ID, &cw.SetID, &cw.Number, &cw.CollectorNumber, &cw.Name, &cw.NamePT,
			&cw.Rarity, &cw.Supertype, &cw.Subtypes, &cw.Types,
			&cw.HP, &cw.Illustrator, &cw.ImageSmallURL, &cw.ImageLargeURL,
			&cw.ImageURLPT,
			&cw.ExternalIDs, &cw.CreatedAt, &cw.UpdatedAt,
			&total,
		); err != nil {
			return nil, 0, fmt.Errorf("scan card row: %w", err)
		}
		cards = append(cards, cw)
		cardIDs = append(cardIDs, cw.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if len(cards) == 0 {
		return []CardWithVariants{}, total, nil
	}

	// Busca as variantes de todas as cartas numa única query.
	vrows, err := r.pool.Query(ctx, listVariantsByCardsSQL, cardIDs)
	if err != nil {
		return nil, 0, fmt.Errorf("list variants batch: %w", err)
	}
	defer vrows.Close()

	// Index por cardID para atribuir variantes às cartas corretas.
	idx := make(map[uuid.UUID]int, len(cards))
	for i, cw := range cards {
		idx[cw.ID] = i
	}

	for vrows.Next() {
		var v card.Variant
		var finish string
		if err := vrows.Scan(&v.ID, &v.CardID, &finish, &v.Label, &v.IsPromo, &v.Notes, &v.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan variant batch: %w", err)
		}
		v.Finish = card.Finish(finish)
		if i, ok := idx[v.CardID]; ok {
			cards[i].Variants = append(cards[i].Variants, v)
		}
	}
	if err := vrows.Err(); err != nil {
		return nil, 0, err
	}

	return cards, total, nil
}

const getCardBySetAndNumberSQL = `
SELECT c.id, c.set_id, c.number, COALESCE(c.collector_number, ''), c.name::text, COALESCE(c.name_pt, ''),
       COALESCE(c.rarity, ''), COALESCE(c.supertype, ''),
       COALESCE(c.subtypes, '{}'::text[]), COALESCE(c.types, '{}'::text[]),
       COALESCE(c.hp, 0), COALESCE(c.illustrator, ''),
       COALESCE(c.image_small_url, ''), COALESCE(c.image_large_url, ''),
       COALESCE(c.image_url_pt, ''),
       c.external_ids, c.created_at, c.updated_at,
       s.id, s.code, s.name, COALESCE(s.name_pt, ''), COALESCE(s.name_en, ''),
       COALESCE(cr.name, s.series, '') AS series_name,
       COALESCE(cr.name_pt, '') AS series_name_pt,
       s.series_id,
       COALESCE(s.tcg, 'pokemon'), s.language, s.release_date,
       COALESCE(s.total_cards, 0), COALESCE(s.printed_total, 0),
       COALESCE(s.image_url, ''), COALESCE(s.symbol_url, ''),
       s.created_at, s.updated_at
FROM cards c
JOIN card_sets s ON s.id = c.set_id
LEFT JOIN card_series cr ON cr.id = s.series_id
WHERE s.code = $1 AND (
    c.collector_number = $2
    OR (
        c.collector_number ~ '^\d+$'
        AND $2 ~ '^\d+$'
        AND c.collector_number::int = $2::int
    )
)
LIMIT 1`

// GetCardBySetAndNumber busca uma carta pelo código do set e collector_number.
// Retorna ErrNotFound quando a combinação não existe.
func (r *CardRepo) GetCardBySetAndNumber(ctx context.Context, setCode, collectorNumber string) (card.Card, card.Set, error) {
	var c card.Card
	var s card.Set
	var lang string

	err := r.pool.QueryRow(ctx, getCardBySetAndNumberSQL, setCode, collectorNumber).Scan(
		&c.ID, &c.SetID, &c.Number, &c.CollectorNumber, &c.Name, &c.NamePT,
		&c.Rarity, &c.Supertype, &c.Subtypes, &c.Types,
		&c.HP, &c.Illustrator, &c.ImageSmallURL, &c.ImageLargeURL,
		&c.ImageURLPT,
		&c.ExternalIDs, &c.CreatedAt, &c.UpdatedAt,
		&s.ID, &s.Code, &s.Name, &s.NamePT, &s.NameEN,
		&s.Series, &s.SeriesPT, &s.SeriesID,
		&s.TCG, &lang, &s.ReleaseDate,
		&s.TotalCards, &s.PrintedTotal,
		&s.ImageURL, &s.SymbolURL,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return card.Card{}, card.Set{}, ErrNotFound
	}
	if err != nil {
		return card.Card{}, card.Set{}, fmt.Errorf("get card by set+number: %w", err)
	}
	s.Language = card.Language(lang)
	return c, s, nil
}

// autocompleteSQL usa UNION ALL para priorizar matches de prefixo sobre matches
// de similaridade (trgm). O UNION é necessário porque ORDER BY não pode ser
// aplicado internamente em cada ramo antes da combinação sem subquery.
const autocompleteSQL = `
SELECT id, name, name_pt, collector_number, set_code, set_name, image_small_url, priority
FROM (
    (
      SELECT c.id, c.name::text AS name, COALESCE(c.name_pt, '') AS name_pt,
             COALESCE(c.collector_number, '') AS collector_number,
             s.code AS set_code, s.name::text AS set_name,
             COALESCE(c.image_small_url, '') AS image_small_url,
             1 AS priority
      FROM cards c
      JOIN card_sets s ON s.id = c.set_id
      WHERE (c.name ILIKE $1 || '%' OR c.name_pt ILIKE $1 || '%'
             OR (c.collector_number ILIKE SPLIT_PART($1, '/', 1) || '%'
                 AND (SPLIT_PART($1, '/', 2) = '' OR COALESCE(s.printed_total, s.total_cards)::text LIKE SPLIT_PART($1, '/', 2) || '%')))
        AND ($3 = '' OR s.tcg = $3)
      LIMIT $2
    )
    UNION ALL
    (
      SELECT c.id, c.name::text, COALESCE(c.name_pt, ''), COALESCE(c.collector_number, ''),
             s.code, s.name::text,
             COALESCE(c.image_small_url, ''),
             2 AS priority
      FROM cards c
      JOIN card_sets s ON s.id = c.set_id
      WHERE c.id NOT IN (
          SELECT c2.id FROM cards c2
          JOIN card_sets s2 ON s2.id = c2.set_id
          WHERE (c2.name ILIKE $1 || '%' OR c2.name_pt ILIKE $1 || '%'
                 OR (c2.collector_number ILIKE SPLIT_PART($1, '/', 1) || '%'
                     AND (SPLIT_PART($1, '/', 2) = '' OR COALESCE(s2.printed_total, s2.total_cards)::text LIKE SPLIT_PART($1, '/', 2) || '%')))
            AND ($3 = '' OR s2.tcg = $3)
          -- sem LIMIT aqui: excluir TODOS os matches de prefixo para evitar duplicatas
      )
      AND (c.name::text % $1 OR c.name_pt % $1)
      AND ($3 = '' OR s.tcg = $3)
      ORDER BY GREATEST(similarity(c.name::text, $1), similarity(COALESCE(c.name_pt, ''), $1)) DESC
      LIMIT $2
    )
) sub
ORDER BY priority, name
LIMIT $2`

// AutocompleteCards busca cartas para sugestão de busca rápida.
// Prioriza matches de prefixo; usa similaridade trgm como fallback.
// limit é capped a 20 — valores maiores são truncados no caller (handler usa 8).
func (r *CardRepo) AutocompleteCards(ctx context.Context, q, tcg string, limit int) ([]AutocompleteResult, error) {
	if limit <= 0 || limit > 20 {
		limit = 8
	}

	rows, err := r.pool.Query(ctx, autocompleteSQL, q, limit, tcg)
	if err != nil {
		return nil, fmt.Errorf("autocomplete cards: %w", err)
	}
	defer rows.Close()

	var out []AutocompleteResult
	for rows.Next() {
		var a AutocompleteResult
		var priority int
		if err := rows.Scan(
			&a.ID, &a.Name, &a.NamePT, &a.CollectorNumber,
			&a.SetCode, &a.SetName, &a.ImageSmallURL, &priority,
		); err != nil {
			return nil, fmt.Errorf("scan autocomplete row: %w", err)
		}
		a.Slug = fmt.Sprintf("%s-%s", a.SetCode, a.CollectorNumber)
		out = append(out, a)
	}
	return out, rows.Err()
}
