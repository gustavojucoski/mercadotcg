package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/store"
)

// Erros sentinela específicos de estoque.
var (
	// ErrInsufficientStock é retornado quando uma venda/saída excede a
	// quantidade disponível no stock_item.
	ErrInsufficientStock = errors.New("postgres: estoque insuficiente")

	// ErrInvalidMovement é retornado quando o movimento solicitado tem
	// parâmetros inconsistentes (ex.: quantidade <= 0 numa compra).
	ErrInvalidMovement = errors.New("postgres: movimento inválido")
)

// StockRepo lida com stock_items + stock_movements.
//
// Operações que alteram quantidade são *sempre* transacionais:
// stock_items é a verdade corrente, stock_movements é o log que justifica
// a verdade. Quem altera uma sem a outra está criando inconsistência.
type StockRepo struct {
	pool *pgxpool.Pool
}

// NewStockRepo devolve um repositório pronto para uso.
func NewStockRepo(pool *pgxpool.Pool) *StockRepo {
	return &StockRepo{pool: pool}
}

// PurchaseInput descreve a aquisição de N unidades de uma variante.
// Se o stock_item ainda não existe, é criado; se existe, quantidade soma
// e cost_avg_brl é recalculado por média ponderada.
type PurchaseInput struct {
	StoreID      uuid.UUID
	VariantID    uuid.UUID
	Condition    string
	Language     string
	Grade        string // pode ser ""
	Quantity     int
	UnitCostBRL  decimal.Decimal
	OccurredAt   time.Time
	ReferenceTyp string
	ReferenceID  string
	Notes        string
}

// SaleInput descreve a venda de N unidades de um stock_item específico.
// Identificamos o item por ID porque a venda já pressupõe que ele exista.
type SaleInput struct {
	StockItemID  uuid.UUID
	Quantity     int
	UnitPriceBRL decimal.Decimal
	OccurredAt   time.Time
	ReferenceTyp string
	ReferenceID  string
	Notes        string
}

// AdjustmentInput aplica um ajuste manual no estoque (positivo ou negativo).
// Não toca em cost_avg_brl — ajustes são contábeis, não de aquisição.
type AdjustmentInput struct {
	StockItemID    uuid.UUID
	QuantityDelta  int // pode ser positivo ou negativo, nunca zero
	OccurredAt     time.Time
	Notes          string
}

// ----------------------------------------------------------------------------
// Operações transacionais
// ----------------------------------------------------------------------------

// RegisterPurchase grava uma aquisição: cria/atualiza stock_item e adiciona
// stock_movement do tipo 'purchase'. Recalcula custo médio ponderado.
//
// Devolve o stock_item resultante (com IDs e cost_avg atualizados).
func (r *StockRepo) RegisterPurchase(ctx context.Context, in PurchaseInput) (store.StockItem, error) {
	if in.Quantity <= 0 {
		return store.StockItem{}, fmt.Errorf("%w: quantity deve ser > 0", ErrInvalidMovement)
	}
	if in.UnitCostBRL.IsNegative() {
		return store.StockItem{}, fmt.Errorf("%w: unit_cost negativo", ErrInvalidMovement)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return store.StockItem{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	item, err := r.getOrCreateItemForUpdate(ctx, tx, in.StoreID, in.VariantID, in.Condition, in.Language, in.Grade)
	if err != nil {
		return store.StockItem{}, err
	}

	newQty := item.Quantity + in.Quantity
	newAvg := weightedAverage(item.CostAvgBRL, item.Quantity, in.UnitCostBRL, in.Quantity)

	if err := updateItemQuantityAndCost(ctx, tx, item.ID, newQty, &newAvg); err != nil {
		return store.StockItem{}, err
	}

	occurredAt := orNow(in.OccurredAt)
	if err := insertMovement(ctx, tx, store.StockMovement{
		StockItemID:   item.ID,
		Kind:          store.MovementPurchase,
		QuantityDelta: in.Quantity,
		UnitPriceBRL:  &in.UnitCostBRL,
		ReferenceType: in.ReferenceTyp,
		ReferenceID:   in.ReferenceID,
		Notes:         in.Notes,
		OccurredAt:    occurredAt,
	}); err != nil {
		return store.StockItem{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return store.StockItem{}, fmt.Errorf("commit purchase: %w", err)
	}

	item.Quantity = newQty
	item.CostAvgBRL = &newAvg
	return item, nil
}

// RegisterSale grava uma venda: subtrai quantidade do stock_item e adiciona
// stock_movement do tipo 'sale'. cost_avg_brl não é alterado (custo médio
// vale só para entradas).
//
// Falha com ErrInsufficientStock se quantity em estoque é menor que a venda.
func (r *StockRepo) RegisterSale(ctx context.Context, in SaleInput) (store.StockItem, error) {
	if in.Quantity <= 0 {
		return store.StockItem{}, fmt.Errorf("%w: quantity deve ser > 0", ErrInvalidMovement)
	}
	if in.UnitPriceBRL.IsNegative() {
		return store.StockItem{}, fmt.Errorf("%w: unit_price negativo", ErrInvalidMovement)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return store.StockItem{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	item, err := getItemForUpdateByID(ctx, tx, in.StockItemID)
	if err != nil {
		return store.StockItem{}, err
	}

	if item.Quantity < in.Quantity {
		return store.StockItem{}, fmt.Errorf("%w: %d em estoque, tentou vender %d",
			ErrInsufficientStock, item.Quantity, in.Quantity)
	}

	newQty := item.Quantity - in.Quantity
	if err := updateItemQuantityAndCost(ctx, tx, item.ID, newQty, item.CostAvgBRL); err != nil {
		return store.StockItem{}, err
	}

	occurredAt := orNow(in.OccurredAt)
	if err := insertMovement(ctx, tx, store.StockMovement{
		StockItemID:   item.ID,
		Kind:          store.MovementSale,
		QuantityDelta: -in.Quantity,
		UnitPriceBRL:  &in.UnitPriceBRL,
		ReferenceType: in.ReferenceTyp,
		ReferenceID:   in.ReferenceID,
		Notes:         in.Notes,
		OccurredAt:    occurredAt,
	}); err != nil {
		return store.StockItem{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return store.StockItem{}, fmt.Errorf("commit sale: %w", err)
	}

	item.Quantity = newQty
	return item, nil
}

// RegisterAdjustment aplica um delta arbitrário (positivo ou negativo) ao
// stock_item, sem mexer em cost_avg_brl. Útil para corrigir erros de contagem.
func (r *StockRepo) RegisterAdjustment(ctx context.Context, in AdjustmentInput) (store.StockItem, error) {
	if in.QuantityDelta == 0 {
		return store.StockItem{}, fmt.Errorf("%w: delta não pode ser zero", ErrInvalidMovement)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return store.StockItem{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	item, err := getItemForUpdateByID(ctx, tx, in.StockItemID)
	if err != nil {
		return store.StockItem{}, err
	}

	newQty := item.Quantity + in.QuantityDelta
	if newQty < 0 {
		return store.StockItem{}, fmt.Errorf("%w: ajuste deixaria quantity negativa", ErrInsufficientStock)
	}

	if err := updateItemQuantityAndCost(ctx, tx, item.ID, newQty, item.CostAvgBRL); err != nil {
		return store.StockItem{}, err
	}

	occurredAt := orNow(in.OccurredAt)
	if err := insertMovement(ctx, tx, store.StockMovement{
		StockItemID:   item.ID,
		Kind:          store.MovementAdjustment,
		QuantityDelta: in.QuantityDelta,
		Notes:         in.Notes,
		OccurredAt:    occurredAt,
	}); err != nil {
		return store.StockItem{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return store.StockItem{}, fmt.Errorf("commit adjustment: %w", err)
	}

	item.Quantity = newQty
	return item, nil
}

// ----------------------------------------------------------------------------
// Leituras
// ----------------------------------------------------------------------------

const selectStockItemByIDSQL = `
SELECT id, store_id, variant_id, condition, language, COALESCE(grade, ''),
       quantity, cost_avg_brl, asking_price_brl, COALESCE(sku, ''), COALESCE(notes, ''),
       created_at, updated_at
FROM stock_items WHERE id = $1`

// GetItemByID busca um stock_item pelo ID.
func (r *StockRepo) GetItemByID(ctx context.Context, id uuid.UUID) (store.StockItem, error) {
	row := r.pool.QueryRow(ctx, selectStockItemByIDSQL, id)
	return scanStockItem(row)
}

const listStockItemsByStoreSQL = `
SELECT id, store_id, variant_id, condition, language, COALESCE(grade, ''),
       quantity, cost_avg_brl, asking_price_brl, COALESCE(sku, ''), COALESCE(notes, ''),
       created_at, updated_at
FROM stock_items
WHERE store_id = $1
  AND ($2 = FALSE OR quantity > 0)
ORDER BY updated_at DESC
LIMIT $3 OFFSET $4`

// ListItemsByStore lista itens de uma loja. Se onlyInStock=true, filtra
// quantity > 0. Pagina por limit/offset.
func (r *StockRepo) ListItemsByStore(
	ctx context.Context,
	storeID uuid.UUID,
	onlyInStock bool,
	limit, offset int,
) ([]store.StockItem, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := r.pool.Query(ctx, listStockItemsByStoreSQL, storeID, onlyInStock, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list stock items: %w", err)
	}
	defer rows.Close()

	var out []store.StockItem
	for rows.Next() {
		item, err := scanStockItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

const listMovementsSQL = `
SELECT id, stock_item_id, kind, quantity_delta, unit_price_brl,
       COALESCE(reference_type, ''), COALESCE(reference_id, ''), COALESCE(notes, ''),
       occurred_at, created_at
FROM stock_movements
WHERE stock_item_id = $1
ORDER BY occurred_at DESC
LIMIT $2`

// ListMovementsForItem devolve o log de movimentos mais recente para um item.
func (r *StockRepo) ListMovementsForItem(
	ctx context.Context,
	itemID uuid.UUID,
	limit int,
) ([]store.StockMovement, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	rows, err := r.pool.Query(ctx, listMovementsSQL, itemID, limit)
	if err != nil {
		return nil, fmt.Errorf("list movements: %w", err)
	}
	defer rows.Close()

	var out []store.StockMovement
	for rows.Next() {
		var m store.StockMovement
		var kind string
		if err := rows.Scan(
			&m.ID, &m.StockItemID, &kind, &m.QuantityDelta, &m.UnitPriceBRL,
			&m.ReferenceType, &m.ReferenceID, &m.Notes,
			&m.OccurredAt, &m.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan movement: %w", err)
		}
		m.Kind = store.MovementKind(kind)
		out = append(out, m)
	}
	return out, rows.Err()
}

// ----------------------------------------------------------------------------
// Helpers internos (transação)
// ----------------------------------------------------------------------------

const selectItemForUpdateByNaturalKeySQL = `
SELECT id, store_id, variant_id, condition, language, COALESCE(grade, ''),
       quantity, cost_avg_brl, asking_price_brl, COALESCE(sku, ''), COALESCE(notes, ''),
       created_at, updated_at
FROM stock_items
WHERE store_id = $1 AND variant_id = $2 AND condition = $3
  AND language = $4 AND COALESCE(grade, '') = $5
FOR UPDATE`

const insertStockItemSQL = `
INSERT INTO stock_items (
    store_id, variant_id, condition, language, grade, quantity, cost_avg_brl
)
VALUES ($1, $2, $3, $4, NULLIF($5, ''), 0, NULL)
RETURNING id, created_at, updated_at`

// getOrCreateItemForUpdate aplica SELECT … FOR UPDATE; se a linha não existe,
// cria uma com quantity=0 e re-seleciona com lock. Garante que o caller
// pode somar quantidade sem race.
func (r *StockRepo) getOrCreateItemForUpdate(
	ctx context.Context,
	tx pgx.Tx,
	storeID, variantID uuid.UUID,
	condition, language, grade string,
) (store.StockItem, error) {
	row := tx.QueryRow(ctx, selectItemForUpdateByNaturalKeySQL,
		storeID, variantID, condition, language, grade)

	item, err := scanStockItem(row)
	if err == nil {
		return item, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return store.StockItem{}, err
	}

	// Não existe — cria.
	var newID uuid.UUID
	var createdAt, updatedAt time.Time
	if err := tx.QueryRow(ctx, insertStockItemSQL,
		storeID, variantID, condition, language, grade,
	).Scan(&newID, &createdAt, &updatedAt); err != nil {
		return store.StockItem{}, fmt.Errorf("insert stock_item: %w", err)
	}

	return store.StockItem{
		ID:        newID,
		StoreID:   storeID,
		VariantID: variantID,
		Condition: condition,
		Language:  language,
		Grade:     grade,
		Quantity:  0,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}

const selectItemForUpdateByIDSQL = `
SELECT id, store_id, variant_id, condition, language, COALESCE(grade, ''),
       quantity, cost_avg_brl, asking_price_brl, COALESCE(sku, ''), COALESCE(notes, ''),
       created_at, updated_at
FROM stock_items WHERE id = $1 FOR UPDATE`

func getItemForUpdateByID(ctx context.Context, tx pgx.Tx, id uuid.UUID) (store.StockItem, error) {
	row := tx.QueryRow(ctx, selectItemForUpdateByIDSQL, id)
	return scanStockItem(row)
}

const updateItemQuantityCostSQL = `
UPDATE stock_items
SET quantity = $2, cost_avg_brl = $3
WHERE id = $1`

func updateItemQuantityAndCost(
	ctx context.Context,
	tx pgx.Tx,
	id uuid.UUID,
	newQty int,
	newCost *decimal.Decimal,
) error {
	if _, err := tx.Exec(ctx, updateItemQuantityCostSQL, id, newQty, newCost); err != nil {
		return fmt.Errorf("update stock_item: %w", err)
	}
	return nil
}

const insertMovementSQL = `
INSERT INTO stock_movements (
    stock_item_id, kind, quantity_delta, unit_price_brl,
    reference_type, reference_id, notes, occurred_at
) VALUES (
    $1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), $8
)`

func insertMovement(ctx context.Context, tx pgx.Tx, m store.StockMovement) error {
	_, err := tx.Exec(ctx, insertMovementSQL,
		m.StockItemID, string(m.Kind), m.QuantityDelta, m.UnitPriceBRL,
		m.ReferenceType, m.ReferenceID, m.Notes, m.OccurredAt,
	)
	if err != nil {
		return fmt.Errorf("insert stock_movement: %w", err)
	}
	return nil
}

// ----------------------------------------------------------------------------
// Helpers de scan
// ----------------------------------------------------------------------------

// rowScanner abstrai pgx.Row e pgx.Rows para reutilizar scanStockItem.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanStockItem(row rowScanner) (store.StockItem, error) {
	var item store.StockItem
	err := row.Scan(
		&item.ID, &item.StoreID, &item.VariantID, &item.Condition, &item.Language, &item.Grade,
		&item.Quantity, &item.CostAvgBRL, &item.AskingPriceBRL, &item.SKU, &item.Notes,
		&item.CreatedAt, &item.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.StockItem{}, ErrNotFound
	}
	if err != nil {
		return store.StockItem{}, fmt.Errorf("scan stock_item: %w", err)
	}
	return item, nil
}

// ----------------------------------------------------------------------------
// Aritmética
// ----------------------------------------------------------------------------

// weightedAverage calcula o custo médio ponderado após adicionar `addQty`
// unidades a `addUnitCost`. Se o estoque atual era zero ou o avg era nil,
// devolve apenas o `addUnitCost`.
func weightedAverage(
	currentAvg *decimal.Decimal,
	currentQty int,
	addUnitCost decimal.Decimal,
	addQty int,
) decimal.Decimal {
	if currentAvg == nil || currentQty <= 0 {
		return addUnitCost
	}
	cur := *currentAvg
	currentTotal := cur.Mul(decimal.NewFromInt(int64(currentQty)))
	addTotal := addUnitCost.Mul(decimal.NewFromInt(int64(addQty)))
	totalQty := decimal.NewFromInt(int64(currentQty + addQty))
	return currentTotal.Add(addTotal).Div(totalQty).Round(4)
}

// orNow devolve `t` se não for zero, senão time.Now().
// Usado para deixar o caller passar OccurredAt opcional.
func orNow(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now()
	}
	return t
}
