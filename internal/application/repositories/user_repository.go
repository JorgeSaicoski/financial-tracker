package repositories

import (
	"context"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
)

// UserRepository persists users, expressed in application/dto types —
// infrastructure adapts its rows to these at the boundary.
type UserRepository interface {
	// Upsert inserts a user on first sight of its ID, or refreshes its
	// provider/external_id/email/display_name on repeat sight (an
	// identity provider's profile fields can change between logins).
	// CloudSyncEnabled is never touched by Upsert — it's the one field a
	// user (not the identity provider) owns, once something exposes it.
	// Always returns the row as currently stored, so a caller sees the
	// existing CloudSyncEnabled/CreatedAt rather than the zero-valued
	// fields it upserted with.
	Upsert(ctx context.Context, user *dto.UserDTO) (*dto.UserDTO, error)
	GetByID(ctx context.Context, id string) (*dto.UserDTO, error)
}
