// Package store define os tipos de domínio para lojas e estoque.
//
// O modelo é multi-tenant: cada Store pertence a um owner (futuro users),
// e todo StockItem carrega store_id. StockMovement é um log append-only
// que preserva histórico de aquisições, vendas, ajustes e perdas para
// contabilidade (custo médio ponderado, margem real, FIFO se necessário).
package store

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// DocumentType espelha o ENUM document_type do banco.
type DocumentType string

const (
	DocumentTypeCPF  DocumentType = "cpf"
	DocumentTypeCNPJ DocumentType = "cnpj"
)

// DocumentStatus espelha o ENUM document_status do banco.
type DocumentStatus string

const (
	DocumentStatusPending          DocumentStatus = "pending"
	DocumentStatusAutoVerified     DocumentStatus = "auto_verified"
	DocumentStatusManuallyVerified DocumentStatus = "manually_verified"
)

// Store representa um lojista cadastrado na plataforma.
type Store struct {
	ID          uuid.UUID `json:"id"`
	OwnerID     uuid.UUID `json:"owner_id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description,omitempty"`
	LogoURL     string    `json:"logo_url,omitempty"`
	IsActive    bool      `json:"is_active"`

	DocumentType      *DocumentType  `json:"document_type,omitempty"`
	DocumentNumber    *string        `json:"document_number,omitempty"`
	DocumentStatus    DocumentStatus `json:"document_status"`
	LegalName         *string        `json:"legal_name,omitempty"`
	DocumentVerifiedAt *time.Time    `json:"document_verified_at,omitempty"`
	DocumentVerifiedBy *uuid.UUID    `json:"document_verified_by,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// StockItem é a posição corrente de uma combinação (variante, condição,
// idioma, grade) numa loja. Quantidade é cumulativa; alterações fluem por
// StockMovement.
type StockItem struct {
	ID             uuid.UUID        `json:"id"`
	StoreID        uuid.UUID        `json:"store_id"`
	VariantID      uuid.UUID        `json:"variant_id"`
	Condition      string           `json:"condition"`             // espelha card_condition
	Language       string           `json:"language"`              // 'pt', 'en', 'jp'
	Grade          string           `json:"grade,omitempty"`       // ex.: "PSA 10"
	Quantity       int              `json:"quantity"`
	CostAvgBRL     *decimal.Decimal `json:"cost_avg_brl,omitempty"` // custo médio ponderado
	AskingPriceBRL *decimal.Decimal `json:"asking_price_brl,omitempty"`
	SKU            string           `json:"sku,omitempty"`
	Notes          string           `json:"notes,omitempty"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
}

// MovementKind reflete o ENUM stock_movement_kind do banco.
// Qualquer mudança aqui exige migration nova.
type MovementKind string

const (
	MovementPurchase    MovementKind = "purchase"
	MovementSale        MovementKind = "sale"
	MovementAdjustment  MovementKind = "adjustment"
	MovementTransferIn  MovementKind = "transfer_in"
	MovementTransferOut MovementKind = "transfer_out"
	MovementReservation MovementKind = "reservation"
	MovementRelease     MovementKind = "release"
	MovementLoss        MovementKind = "loss"
)

// StockMovement é uma linha do log de movimentações.
//
// QuantityDelta sempre representa a alteração na quantity do StockItem:
//   - Entradas ("purchase", "transfer_in", ajuste positivo) → delta > 0.
//   - Saídas ("sale", "loss", "transfer_out", ajuste negativo) → delta < 0.
//   - Reservation/Release não mexem em quantity física, mas são logadas com
//     delta = 0 *não* — usamos delta com sinal e o caller decide se reflete
//     no estoque físico ou só em "available". Por padrão, reservation/release
//     são gravados com kind diferente e qty_delta = 0 sob CHECK constraint?
//     Não — o CHECK no banco é (quantity_delta <> 0). Reservas serão
//     modeladas em outra história (a tabela ainda não cobre). Por enquanto,
//     o código não emite movimentos de reservation.
type StockMovement struct {
	ID            uuid.UUID        `json:"id"`
	StockItemID   uuid.UUID        `json:"stock_item_id"`
	Kind          MovementKind     `json:"kind"`
	QuantityDelta int              `json:"quantity_delta"`
	UnitPriceBRL  *decimal.Decimal `json:"unit_price_brl,omitempty"`
	ReferenceType string           `json:"reference_type,omitempty"`
	ReferenceID   string           `json:"reference_id,omitempty"`
	Notes         string           `json:"notes,omitempty"`
	OccurredAt    time.Time        `json:"occurred_at"`
	CreatedAt     time.Time        `json:"created_at"`
}
