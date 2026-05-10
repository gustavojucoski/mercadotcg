package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/user"
)

// StoreMemberRepo gerencia vínculos usuário ↔ loja (store_members).
type StoreMemberRepo struct {
	pool *pgxpool.Pool
}

func NewStoreMemberRepo(pool *pgxpool.Pool) *StoreMemberRepo {
	return &StoreMemberRepo{pool: pool}
}

// GetMembership retorna o store_role do usuário na loja.
// Retorna ErrNotFound se o usuário não for membro da loja.
func (r *StoreMemberRepo) GetMembership(ctx context.Context, storeID, userID uuid.UUID) (user.StoreRole, error) {
	const q = `SELECT role FROM store_members WHERE store_id = $1 AND user_id = $2`
	var roleStr string
	err := r.pool.QueryRow(ctx, q, storeID, userID).Scan(&roleStr)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get store membership: %w", err)
	}
	return user.StoreRole(roleStr), nil
}

// AddMember insere uma linha em store_members.
// Retorna ErrAlreadyExists se o usuário já for membro.
func (r *StoreMemberRepo) AddMember(ctx context.Context, storeID, userID uuid.UUID, role user.StoreRole, invitedBy *uuid.UUID) error {
	const q = `
		INSERT INTO store_members (store_id, user_id, role, invited_by)
		VALUES ($1, $2, $3, $4)`
	_, err := r.pool.Exec(ctx, q, storeID, userID, string(role), invitedBy)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == PgUniqueViolation {
			return ErrAlreadyExists
		}
		return fmt.Errorf("add store member: %w", err)
	}
	return nil
}

// UpdateMemberRole altera o papel de um membro existente.
func (r *StoreMemberRepo) UpdateMemberRole(ctx context.Context, storeID, userID uuid.UUID, role user.StoreRole) error {
	const q = `UPDATE store_members SET role = $1 WHERE store_id = $2 AND user_id = $3`
	tag, err := r.pool.Exec(ctx, q, string(role), storeID, userID)
	if err != nil {
		return fmt.Errorf("update store member role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RemoveMember remove um membro da loja.
func (r *StoreMemberRepo) RemoveMember(ctx context.Context, storeID, userID uuid.UUID) error {
	const q = `DELETE FROM store_members WHERE store_id = $1 AND user_id = $2`
	tag, err := r.pool.Exec(ctx, q, storeID, userID)
	if err != nil {
		return fmt.Errorf("remove store member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// StoreMemberRow é a view de leitura para listagens de membros.
type StoreMemberRow struct {
	user.StoreMember
	UserEmail       string `json:"user_email"`
	UserDisplayName string `json:"user_display_name"`
}

// ListMembers retorna todos os membros de uma loja com dados básicos do usuário.
func (r *StoreMemberRepo) ListMembers(ctx context.Context, storeID uuid.UUID) ([]StoreMemberRow, error) {
	const q = `
		SELECT sm.id, sm.store_id, sm.user_id, sm.role, sm.invited_by, sm.joined_at,
		       u.email, u.display_name
		FROM store_members sm
		JOIN users u ON u.id = sm.user_id
		WHERE sm.store_id = $1
		ORDER BY sm.joined_at`
	rows, err := r.pool.Query(ctx, q, storeID)
	if err != nil {
		return nil, fmt.Errorf("list store members: %w", err)
	}
	defer rows.Close()
	var members []StoreMemberRow
	for rows.Next() {
		var m StoreMemberRow
		var roleStr string
		err := rows.Scan(
			&m.ID, &m.StoreID, &m.UserID, &roleStr, &m.InvitedBy, &m.JoinedAt,
			&m.UserEmail, &m.UserDisplayName,
		)
		if err != nil {
			return nil, fmt.Errorf("scan store member: %w", err)
		}
		m.Role = user.StoreRole(roleStr)
		members = append(members, m)
	}
	return members, rows.Err()
}
