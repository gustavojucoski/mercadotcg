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

// SeriesPatch contém campos editáveis de uma série via PATCH admin. Nil = não alterar.
type SeriesPatch struct {
	Name   *string
	NamePT *string
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
	if p.Name == nil && p.NamePT == nil && p.NameEN == nil && p.SeriesID == nil &&
		p.ReleaseDate == nil && p.TotalCards == nil && p.PrintedTotal == nil {
		return r.GetSetByID(ctx, id)
	}

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
	if p.Name == nil && p.NamePT == nil && p.CollectorNumber == nil && p.Rarity == nil &&
		p.Supertype == nil && p.Subtypes == nil && p.Types == nil && p.HP == nil && p.Illustrator == nil {
		return r.GetCardByID(ctx, id)
	}

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

// ErrDeleteBlocked é retornado quando um delete é bloqueado por registros dependentes.
// Carrega as contagens para que o handler construa a resposta 409.
type ErrDeleteBlocked struct {
	Stock    int
	Listings int
	Sets     int
}

func (e ErrDeleteBlocked) Error() string {
	return fmt.Sprintf("delete blocked: %d stock, %d listings, %d sets", e.Stock, e.Listings, e.Sets)
}

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

// DeleteSetWithCards atomicamente verifica bloqueios e apaga cartas + set em uma transação.
// Retorna ErrDeleteBlocked se há stock ou listings ativos, ErrNotFound se o set não existe.
func (r *CardRepo) DeleteSetWithCards(ctx context.Context, id uuid.UUID) error {
	return r.WithTx(ctx, func(tx pgx.Tx) error {
		var stock, listings int
		if err := tx.QueryRow(ctx, `
			SELECT COUNT(DISTINCT c.id)
			FROM cards c
			JOIN card_variants cv ON cv.card_id = c.id
			JOIN stock_items si ON si.variant_id = cv.id
			WHERE c.set_id = $1 AND si.quantity > 0`, id).Scan(&stock); err != nil {
			return fmt.Errorf("count stock in tx: %w", err)
		}
		if err := tx.QueryRow(ctx, `
			SELECT COUNT(DISTINCT c.id)
			FROM cards c
			JOIN card_variants cv ON cv.card_id = c.id
			JOIN listings l ON l.variant_id = cv.id
			WHERE c.set_id = $1 AND l.status = 'active'::listing_status`, id).Scan(&listings); err != nil {
			return fmt.Errorf("count listings in tx: %w", err)
		}
		if stock > 0 || listings > 0 {
			return ErrDeleteBlocked{Stock: stock, Listings: listings}
		}
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

// DeleteCard atomicamente verifica bloqueios e apaga a carta em uma transação.
// As variantes cascateiam (FK ON DELETE CASCADE). price_history/price_daily idem (ADR-031).
// Retorna ErrDeleteBlocked se há stock ou listings ativos.
func (r *CardRepo) DeleteCard(ctx context.Context, id uuid.UUID) error {
	return r.WithTx(ctx, func(tx pgx.Tx) error {
		var stock, listings int
		if err := tx.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM stock_items si
			JOIN card_variants cv ON cv.id = si.variant_id
			WHERE cv.card_id = $1 AND si.quantity > 0`, id).Scan(&stock); err != nil {
			return fmt.Errorf("count stock in tx: %w", err)
		}
		if err := tx.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM listings l
			JOIN card_variants cv ON cv.id = l.variant_id
			WHERE cv.card_id = $1 AND l.status = 'active'::listing_status`, id).Scan(&listings); err != nil {
			return fmt.Errorf("count listings in tx: %w", err)
		}
		if stock > 0 || listings > 0 {
			return ErrDeleteBlocked{Stock: stock, Listings: listings}
		}
		tag, err := tx.Exec(ctx, "DELETE FROM cards WHERE id = $1", id)
		if err != nil {
			return fmt.Errorf("delete card: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// DeleteVariant atomicamente verifica bloqueios e apaga a variante em uma transação.
// price_history e price_daily cascateiam (ADR-031).
// Retorna ErrDeleteBlocked se há stock ou listings ativos.
func (r *CardRepo) DeleteVariant(ctx context.Context, id uuid.UUID) error {
	return r.WithTx(ctx, func(tx pgx.Tx) error {
		var stock, listings int
		if err := tx.QueryRow(ctx, `
			SELECT COUNT(*) FROM stock_items WHERE variant_id = $1 AND quantity > 0`, id).Scan(&stock); err != nil {
			return fmt.Errorf("count stock in tx: %w", err)
		}
		if err := tx.QueryRow(ctx, `
			SELECT COUNT(*) FROM listings WHERE variant_id = $1 AND status = 'active'::listing_status`, id).Scan(&listings); err != nil {
			return fmt.Errorf("count listings in tx: %w", err)
		}
		if stock > 0 || listings > 0 {
			return ErrDeleteBlocked{Stock: stock, Listings: listings}
		}
		tag, err := tx.Exec(ctx, "DELETE FROM card_variants WHERE id = $1", id)
		if err != nil {
			return fmt.Errorf("delete variant: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
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

// ---- Series admin -----------------------------------------------------------

// GetSeriesByID busca uma série pelo seu UUID.
func (r *CardRepo) GetSeriesByID(ctx context.Context, id uuid.UUID) (card.Series, error) {
	const q = `SELECT id, name, COALESCE(name_pt, ''), tcg, created_at FROM card_series WHERE id = $1`
	var s card.Series
	err := r.pool.QueryRow(ctx, q, id).Scan(&s.ID, &s.Name, &s.NamePT, &s.TCG, &s.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return card.Series{}, ErrNotFound
	}
	if err != nil {
		return card.Series{}, fmt.Errorf("get series by id: %w", err)
	}
	return s, nil
}

// CreateSeriesAdmin insere uma série marcada via admin.
// Retorna ErrAlreadyExists se (name, tcg) já existir.
// Preenche s.ID e s.CreatedAt com os valores retornados pelo RETURNING.
func (r *CardRepo) CreateSeriesAdmin(ctx context.Context, s *card.Series) error {
	const q = `
	INSERT INTO card_series (name, name_pt, tcg)
	VALUES ($1, NULLIF($2, ''), $3)
	RETURNING id, created_at`

	err := r.pool.QueryRow(ctx, q, s.Name, s.NamePT, s.TCG).Scan(&s.ID, &s.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == PgUniqueViolation {
			return ErrAlreadyExists
		}
		return fmt.Errorf("create series admin: %w", err)
	}
	return nil
}

// UpdateSeries aplica um patch parcial em uma série. Campos nil são preservados.
func (r *CardRepo) UpdateSeries(ctx context.Context, id uuid.UUID, p SeriesPatch) (card.Series, error) {
	if p.Name == nil && p.NamePT == nil {
		return r.GetSeriesByID(ctx, id)
	}

	setClauses := []string{}
	args := []any{id}
	i := 2

	if p.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", i))
		args = append(args, *p.Name)
		i++
	}
	if p.NamePT != nil {
		// NULLIF aceita string vazia como "limpar o campo" (define NULL no banco).
		setClauses = append(setClauses, fmt.Sprintf("name_pt = NULLIF($%d, '')", i))
		args = append(args, *p.NamePT)
		i++
	}
	_ = i

	q := fmt.Sprintf("UPDATE card_series SET %s WHERE id = $1", strings.Join(setClauses, ", "))
	tag, err := r.pool.Exec(ctx, q, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == PgUniqueViolation {
			return card.Series{}, ErrAlreadyExists
		}
		return card.Series{}, fmt.Errorf("update series: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return card.Series{}, ErrNotFound
	}
	return r.GetSeriesByID(ctx, id)
}

// DeleteSeries apaga uma série pelo UUID dentro de uma transação.
// Retorna ErrNotFound se não existir.
// Retorna ErrDeleteBlocked{Sets: n} se houver sets vinculados.
func (r *CardRepo) DeleteSeries(ctx context.Context, id uuid.UUID) error {
	return r.WithTx(ctx, func(tx pgx.Tx) error {
		// Verificar existência dentro da tx.
		var exists uuid.UUID
		err := tx.QueryRow(ctx, `SELECT id FROM card_series WHERE id = $1`, id).Scan(&exists)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("check series exists: %w", err)
		}

		// Contar sets vinculados dentro da mesma tx.
		var count int64
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM card_sets WHERE series_id = $1`, id).Scan(&count); err != nil {
			return fmt.Errorf("count sets for series: %w", err)
		}
		if count > 0 {
			return ErrDeleteBlocked{Sets: int(count)}
		}

		_, err = tx.Exec(ctx, `DELETE FROM card_series WHERE id = $1`, id)
		if err != nil {
			return fmt.Errorf("delete series: %w", err)
		}
		return nil
	})
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
       COALESCE(cs.import_source, '') AS import_source,
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
			&s.ImportSource,
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
