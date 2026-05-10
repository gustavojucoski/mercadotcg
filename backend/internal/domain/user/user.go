package user

import (
	"time"

	"github.com/google/uuid"
)

type PlatformRole string

const (
	RolePlatformAdmin PlatformRole = "platform_admin"
	RoleUser          PlatformRole = "user"
)

type StoreRole string

const (
	StoreRoleAdmin        StoreRole = "admin"
	StoreRoleStockManager StoreRole = "stock_manager"
	StoreRoleViewer       StoreRole = "viewer"
)

// StoreRoleLevel returns a numeric weight for role comparison.
// Higher = more permissive. Used by RequireStoreRole middleware.
func StoreRoleLevel(r StoreRole) int {
	switch r {
	case StoreRoleAdmin:
		return 3
	case StoreRoleStockManager:
		return 2
	case StoreRoleViewer:
		return 1
	default:
		return 0
	}
}

type User struct {
	ID               uuid.UUID    `json:"id"`
	Email            string       `json:"email"`
	DisplayName      string       `json:"display_name"`
	AvatarURL        string       `json:"avatar_url,omitempty"`
	PasswordHash     string       `json:"-"`
	PlatformRole     PlatformRole `json:"platform_role"`
	EmailVerifiedAt  *time.Time   `json:"email_verified_at,omitempty"`
	IsActive         bool         `json:"is_active"`
	CreatedAt        time.Time    `json:"created_at"`
	UpdatedAt        time.Time    `json:"updated_at"`
}

func (u *User) IsVerified() bool {
	return u.EmailVerifiedAt != nil
}

type StoreMember struct {
	ID        uuid.UUID  `json:"id"`
	StoreID   uuid.UUID  `json:"store_id"`
	UserID    uuid.UUID  `json:"user_id"`
	Role      StoreRole  `json:"role"`
	InvitedBy *uuid.UUID `json:"invited_by,omitempty"`
	JoinedAt  time.Time  `json:"joined_at"`
}
