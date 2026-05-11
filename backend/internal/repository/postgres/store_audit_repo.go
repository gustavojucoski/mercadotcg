package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/store"
)

// StoreAuditRepo persists and queries store_audit_log entries.
type StoreAuditRepo struct {
	pool *pgxpool.Pool
}

func NewStoreAuditRepo(pool *pgxpool.Pool) *StoreAuditRepo {
	return &StoreAuditRepo{pool: pool}
}

// Insert records a change event. Skips the insert when changes is empty.
func (r *StoreAuditRepo) Insert(
	ctx context.Context,
	storeID, changedBy uuid.UUID,
	changeType string,
	changes map[string]store.FieldChange,
) error {
	if len(changes) == 0 {
		return nil
	}
	data, err := json.Marshal(changes)
	if err != nil {
		return fmt.Errorf("marshal audit changes: %w", err)
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO store_audit_log (store_id, changed_by, change_type, changes)
		VALUES ($1, $2, $3, $4)`,
		storeID, changedBy, changeType, data,
	)
	return err
}

// ListByStore returns audit entries for a store, newest first.
func (r *StoreAuditRepo) ListByStore(
	ctx context.Context,
	storeID uuid.UUID,
	limit, offset int,
) ([]store.AuditEntry, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT al.id, al.store_id, al.changed_by,
		       u.display_name, u.email,
		       al.change_type, al.changes, al.created_at
		FROM store_audit_log al
		JOIN users u ON u.id = al.changed_by
		WHERE al.store_id = $1
		ORDER BY al.created_at DESC
		LIMIT $2 OFFSET $3`,
		storeID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list audit log: %w", err)
	}
	defer rows.Close()

	entries := []store.AuditEntry{}
	for rows.Next() {
		var e store.AuditEntry
		var raw []byte
		if err := rows.Scan(
			&e.ID, &e.StoreID, &e.ChangedBy,
			&e.ChangedByName, &e.ChangedByEmail,
			&e.ChangeType, &raw, &e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan audit entry: %w", err)
		}
		if err := json.Unmarshal(raw, &e.Changes); err != nil {
			return nil, fmt.Errorf("unmarshal audit changes: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
