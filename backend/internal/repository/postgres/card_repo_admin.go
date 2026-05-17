package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/card"
)

// SetPatch contém campos editáveis de um set via PATCH admin. Nil = não alterar.
type SetPatch struct {
	Name         *string
	NamePT       *string
	NameEN       *string
	SeriesID     *uuid.UUID
	ReleaseDate  *time.Time
	TotalCards   *int
	PrintedTotal *int
}

// CardPatch contém campos editáveis de uma carta via PATCH admin. Nil = não alterar.
type CardPatch struct {
	Name            *string
	NamePT          *string
	CollectorNumber *string
	Rarity          *string
	Supertype       *string
	Subtypes        *[]string
	Types           *[]string
	HP              *int
	Illustrator     *string
}

// VariantPatch contém campos editáveis de uma variante via PATCH admin. Nil = não alterar.
type VariantPatch struct {
	Finish  *card.Finish
	Label   *string
	IsPromo *bool
	Notes   *string
}

// ---- Reads por UUID ---------------------------------------------------------

const selectSetByIDSQL = `
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
WHERE cs.id = $1`

// GetSetByID busca um set pelo seu UUID.
func (r *CardRepo) GetSetByID(ctx context.Context, id uuid.UUID) (card.Set, error) {
	var s card.Set
	var lang string
	err := r.pool.QueryRow(ctx, selectSetByIDSQL, id).Scan(
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
		return card.Set{}, fmt.Errorf("get set by id: %w", err)
	}
	s.Language = card.Language(lang)
	return s, nil
}

// GetVariantByID busca uma variante pelo seu UUID.
func (r *CardRepo) GetVariantByID(ctx context.Context, id uuid.UUID) (card.Variant, error) {
	const q = `
	SELECT id, card_id, finish::text, COALESCE(label, ''), is_promo, COALESCE(notes, ''), created_at
	FROM card_variants WHERE id = $1`

	var v card.Variant
	var finish string
	err := r.pool.QueryRow(ctx, q, id).Scan(
		&v.ID, &v.CardID, &finish, &v.Label, &v.IsPromo, &v.Notes, &v.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return card.Variant{}, ErrNotFound
	}
	if err != nil {
		return card.Variant{}, fmt.Errorf("get variant by id: %w", err)
	}
	v.Finish = card.Finish(finish)
	return v, nil
}

// ---- Admin creates ----------------------------------------------------------

// CreateSetAdmin insere um set marcado como import_source='manual'.
// Retorna ErrAlreadyExists se code já existir.
func (r *CardRepo) CreateSetAdmin(ctx context.Context, s *card.Set) error {
	const q = `
	INSERT INTO card_sets
	    (code, name, name_pt, name_en, series_id, tcg, language, release_date, total_cards, printed_total, import_source)
	VALUES
	    ($1, $2, NULLIF($3, ''), NULLIF($4, ''), $5, $6, $7, $8, NULLIF($9, 0), NULLIF($10, 0), 'manual')
	RETURNING id, created_at, updated_at`

	tcg := s.TCG
	if tcg == "" {
		tcg = "pokemon"
	}

	err := r.pool.QueryRow(ctx, q,
		s.Code, s.Name, s.NamePT, s.NameEN, s.SeriesID,
		tcg, string(s.Language),
		s.ReleaseDate, s.TotalCards, s.PrintedTotal,
	).Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == PgUniqueViolation {
			return ErrAlreadyExists
		}
		return fmt.Errorf("create set admin: %w", err)
	}
	s.ImportSource = "manual"
	return nil
}

// CreateCardAdmin insere uma carta marcada como import_source='manual'.
// Retorna ErrAlreadyExists se (set_id, collector_number) já existir.
func (r *CardRepo) CreateCardAdmin(ctx context.Context, c *card.Card) error {
	const q = `
	INSERT INTO cards
	    (set_id, collector_number, name, name_pt, rarity, supertype, subtypes, types, hp, illustrator, external_ids, import_source)
	VALUES
	    ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), $7, $8, NULLIF($9, 0), NULLIF($10, ''), '{}'::jsonb, 'manual')
	RETURNING id, created_at, updated_at`

	subtypes := c.Subtypes
	if subtypes == nil {
		subtypes = []string{}
	}
	types := c.Types
	if types == nil {
		types = []string{}
	}

	err := r.pool.QueryRow(ctx, q,
		c.SetID, c.CollectorNumber, c.Name, c.NamePT,
		c.Rarity, c.Supertype, subtypes, types,
		c.HP, c.Illustrator,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == PgUniqueViolation {
			return ErrAlreadyExists
		}
		return fmt.Errorf("create card admin: %w", err)
	}
	c.ImportSource = "manual"
	c.ExternalIDs = map[string]string{}
	return nil
}

// ---- Updates (PATCH) --------------------------------------------------------

// UpdateSet aplica um patch parcial em um set. Campos nil são preservados.
// Marca import_source='manual' para indicar edição via admin.
func (r *CardRepo) UpdateSet(ctx context.Context, id uuid.UUID, p SetPatch) (card.Set, error) {
	setClauses := []string{"updated_at = now()", "import_source = 'manual'"}
	args := []any{id}
	i := 2

	if p.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", i))
		args = append(args, *p.Name)
		i++
	}
	if p.NamePT != nil {
		setClauses = append(setClauses, fmt.Sprintf("name_pt = $%d", i))
		args = append(args, *p.NamePT)
		i++
	}
	if p.NameEN != nil {
		setClauses = append(setClauses, fmt.Sprintf("name_en = $%d", i))
		args = append(args, *p.NameEN)
		i++
	}
	if p.SeriesID != nil {
		setClauses = append(setClauses, fmt.Sprintf("series_id = $%d::uuid", i))
		args = append(args, *p.SeriesID)
		i++
	}
	if p.ReleaseDate != nil {
		setClauses = append(setClauses, fmt.Sprintf("release_date = $%d", i))
		args = append(args, *p.ReleaseDate)
		i++
	}
	if p.TotalCards != nil {
		setClauses = append(setClauses, fmt.Sprintf("total_cards = $%d", i))
		args = append(args, *p.TotalCards)
		i++
	}
	if p.PrintedTotal != nil {
		setClauses = append(setClauses, fmt.Sprintf("printed_total = $%d", i))
		args = append(args, *p.PrintedTotal)
		i++
	}
	_ = i

	q := fmt.Sprintf("UPDATE card_sets SET %s WHERE id = $1", strings.Join(setClauses, ", "))
	tag, err := r.pool.Exec(ctx, q, args...)
	if err != nil {
		return card.Set{}, fmt.Errorf("update set: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return card.Set{}, ErrNotFound
	}
	return r.GetSetByID(ctx, id)
}

// UpdateCard aplica um patch parcial em uma carta. Catch 23505 → ErrAlreadyExists.
// Marca import_source='manual' para indicar edição via admin.
func (r *CardRepo) UpdateCard(ctx context.Context, id uuid.UUID, p CardPatch) (card.Card, error) {
	setClauses := []string{"updated_at = now()", "import_source = 'manual'"}
	args := []any{id}
	i := 2

	if p.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", i))
		args = append(args, *p.Name)
		i++
	}
	if p.NamePT != nil {
		setClauses = append(setClauses, fmt.Sprintf("name_pt = $%d", i))
		args = append(args, *p.NamePT)
		i++
	}
	if p.CollectorNumber != nil {
		setClauses = append(setClauses, fmt.Sprintf("collector_number = $%d", i))
		args = append(args, *p.CollectorNumber)
		i++
	}
	if p.Rarity != nil {
		setClauses = append(setClauses, fmt.Sprintf("rarity = $%d", i))
		args = append(args, *p.Rarity)
		i++
	}
	if p.Supertype != nil {
		setClauses = append(setClauses, fmt.Sprintf("supertype = $%d", i))
		args = append(args, *p.Supertype)
		i++
	}
	if p.Subtypes != nil {
		setClauses = append(setClauses, fmt.Sprintf("subtypes = $%d", i))
		args = append(args, *p.Subtypes)
		i++
	}
	if p.Types != nil {
		setClauses = append(setClauses, fmt.Sprintf("types = $%d", i))
		args = append(args, *p.Types)
		i++
	}
	if p.HP != nil {
		setClauses = append(setClauses, fmt.Sprintf("hp = $%d", i))
		args = append(args, *p.HP)
		i++
	}
	if p.Illustrator != nil {
		setClauses = append(setClauses, fmt.Sprintf("illustrator = $%d", i))
		args = append(args, *p.Illustrator)
		i++
	}
	_ = i

	q := fmt.Sprintf("UPDATE cards SET %s WHERE id = $1", strings.Join(setClauses, ", "))
	tag, err := r.pool.Exec(ctx, q, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == PgUniqueViolation {
			return card.Card{}, ErrAlreadyExists
		}
		return card.Card{}, fmt.Errorf("update card: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return card.Card{}, ErrNotFound
	}
	return r.GetCardByID(ctx, id)
}

// UpdateVariant aplica um patch parcial em uma variante. Catch 23505 → ErrAlreadyExists.
func (r *CardRepo) UpdateVariant(ctx context.Context, id uuid.UUID, p VariantPatch) (card.Variant, error) {
	if p.Finish == nil && p.Label == nil && p.IsPromo == nil && p.Notes == nil {
		return r.GetVariantByID(ctx, id)
	}

	setClauses := []string{}
	args := []any{id}
	i := 2

	if p.Finish != nil {
		setClauses = append(setClauses, fmt.Sprintf("finish = $%d::variant_finish", i))
		args = append(args, string(*p.Finish))
		i++
	}
	if p.Label != nil {
		setClauses = append(setClauses, fmt.Sprintf("label = NULLIF($%d, '')", i))
		args = append(args, *p.Label)
		i++
	}
	if p.IsPromo != nil {
		setClauses = append(setClauses, fmt.Sprintf("is_promo = $%d", i))
		args = append(args, *p.IsPromo)
		i++
	}
	if p.Notes != nil {
		setClauses = append(setClauses, fmt.Sprintf("notes = NULLIF($%d, '')", i))
		args = append(args, *p.Notes)
		i++
	}
	_ = i

	q := fmt.Sprintf("UPDATE card_variants SET %s WHERE id = $1", strings.Join(setClauses, ", "))
	tag, err := r.pool.Exec(ctx, q, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == PgUniqueViolation {
			return card.Variant{}, ErrAlreadyExists
		}
		return card.Variant{}, fmt.Errorf("update variant: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return card.Variant{}, ErrNotFound
	}
	return r.GetVariantByID(ctx, id)
}

// ---- Deletes ----------------------------------------------------------------

// WithTx executa fn dentro de uma transação. Faz rollback automaticamente em caso de erro.
func (r *CardRepo) WithTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

// DeleteCardsBySetIDTx apaga todas as cartas de um set dentro de uma transação existente.
// As variantes e price_history cascateiam automaticamente (FK ON DELETE CASCADE).
func (r *CardRepo) DeleteCardsBySetIDTx(ctx context.Context, tx pgx.Tx, setID uuid.UUID) error {
	_, err := tx.Exec(ctx, "DELETE FROM cards WHERE set_id = $1", setID)
	if err != nil {
		return fmt.Errorf("delete cards by set: %w", err)
	}
	return nil
}

// DeleteSet apaga o registro de card_sets pelo UUID.
// As cartas devem ter sido removidas antes (cards.set_id é ON DELETE RESTRICT).
func (r *CardRepo) DeleteSet(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, "DELETE FROM card_sets WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete set: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteSetWithCards apaga todas as cartas do set e depois o set, em uma única transação.
// Garante atomicidade frente ao RESTRICT de cards.set_id → card_sets.
func (r *CardRepo) DeleteSetWithCards(ctx context.Context, id uuid.UUID) error {
	return r.WithTx(ctx, func(tx pgx.Tx) error {
		if err := r.DeleteCardsBySetIDTx(ctx, tx, id); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, "DELETE FROM card_sets WHERE id = $1", id)
		if err != nil {
			return fmt.Errorf("delete set in tx: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// DeleteCard apaga uma carta. As variantes cascateiam (FK ON DELETE CASCADE).
// price_history e price_daily também cascateiam via card_variants (ADR-031).
func (r *CardRepo) DeleteCard(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, "DELETE FROM cards WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete card: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteVariant apaga uma variante. price_history e price_daily cascateiam (ADR-031).
func (r *CardRepo) DeleteVariant(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, "DELETE FROM card_variants WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete variant: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- Guards de delete -------------------------------------------------------

// CountActiveStockForCard conta stock_items com quantity > 0 para a carta (via variantes).
func (r *CardRepo) CountActiveStockForCard(ctx context.Context, cardID uuid.UUID) (int, error) {
	const q = `
	SELECT COUNT(*)
	FROM stock_items si
	JOIN card_variants cv ON cv.id = si.variant_id
	WHERE cv.card_id = $1 AND si.quantity > 0`

	var n int
	if err := r.pool.QueryRow(ctx, q, cardID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count active stock for card: %w", err)
	}
	return n, nil
}

// CountActiveListingsForCard conta listings com status='active' para a carta (via variantes).
func (r *CardRepo) CountActiveListingsForCard(ctx context.Context, cardID uuid.UUID) (int, error) {
	const q = `
	SELECT COUNT(*)
	FROM listings l
	JOIN card_variants cv ON cv.id = l.variant_id
	WHERE cv.card_id = $1 AND l.status = 'active'::listing_status`

	var n int
	if err := r.pool.QueryRow(ctx, q, cardID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count active listings for card: %w", err)
	}
	return n, nil
}

// CountActiveStockForVariant conta stock_items com quantity > 0 para a variante.
func (r *CardRepo) CountActiveStockForVariant(ctx context.Context, variantID uuid.UUID) (int, error) {
	const q = `SELECT COUNT(*) FROM stock_items WHERE variant_id = $1 AND quantity > 0`
	var n int
	if err := r.pool.QueryRow(ctx, q, variantID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count active stock for variant: %w", err)
	}
	return n, nil
}

// CountActiveListingsForVariant conta listings com status='active' para a variante.
func (r *CardRepo) CountActiveListingsForVariant(ctx context.Context, variantID uuid.UUID) (int, error) {
	const q = `SELECT COUNT(*) FROM listings WHERE variant_id = $1 AND status = 'active'::listing_status`
	var n int
	if err := r.pool.QueryRow(ctx, q, variantID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count active listings for variant: %w", err)
	}
	return n, nil
}

// CountCardsWithActiveStockInSet conta cartas distintas do set com stock_items (quantity > 0).
func (r *CardRepo) CountCardsWithActiveStockInSet(ctx context.Context, setID uuid.UUID) (int, error) {
	const q = `
	SELECT COUNT(DISTINCT c.id)
	FROM cards c
	JOIN card_variants cv ON cv.card_id = c.id
	JOIN stock_items si ON si.variant_id = cv.id
	WHERE c.set_id = $1 AND si.quantity > 0`

	var n int
	if err := r.pool.QueryRow(ctx, q, setID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count cards with active stock in set: %w", err)
	}
	return n, nil
}

// CountCardsWithActiveListingsInSet conta cartas distintas do set com listings ativos.
func (r *CardRepo) CountCardsWithActiveListingsInSet(ctx context.Context, setID uuid.UUID) (int, error) {
	const q = `
	SELECT COUNT(DISTINCT c.id)
	FROM cards c
	JOIN card_variants cv ON cv.card_id = c.id
	JOIN listings l ON l.variant_id = cv.id
	WHERE c.set_id = $1 AND l.status = 'active'::listing_status`

	var n int
	if err := r.pool.QueryRow(ctx, q, setID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count cards with active listings in set: %w", err)
	}
	return n, nil
}

// ---- Listagem filtrada para admin -------------------------------------------

const listSetsByTCGFilteredSQL = `
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
  AND ($3 = '' OR cs.code ILIKE '%' || $3 || '%' OR cs.name ILIKE '%' || $3 || '%')
ORDER BY cs.release_date DESC NULLS LAST, cs.name ASC
LIMIT $4 OFFSET $5`

// ListSetsByTCGFiltered lista sets com filtro opcional de texto (q busca em code e name).
// Usado pela UI admin de catálogo (GET /admin/sets).
func (r *CardRepo) ListSetsByTCGFiltered(ctx context.Context, tcg string, seriesID *uuid.UUID, q string, page, limit int) ([]SetWithSeries, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit

	rows, err := r.pool.Query(ctx, listSetsByTCGFilteredSQL, tcg, seriesID, q, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list sets filtered: %w", err)
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
			return nil, 0, fmt.Errorf("scan set filtered: %w", err)
		}
		s.Language = card.Language(lang)
		s.Series = s.SeriesName
		s.SeriesPT = s.SeriesNamePT
		out = append(out, s)
	}
	return out, total, rows.Err()
}
