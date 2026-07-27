package repositories

import (
	"context"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
)

// UserDataKeyRepository persists BACK-16's per-user envelope-encryption
// data keys, wrapped under the server's master key. A key is minted
// lazily, once, on first encrypted write for a user (see
// infrastructure/crypto.FieldCryptor) — never backfilled.
type UserDataKeyRepository interface {
	// Get returns userID's wrapped data key row, or apperrors.ErrNotFound
	// if none exists yet.
	Get(ctx context.Context, userID string) (*dto.UserDataKeyDTO, error)
	// Create inserts a new wrapped data key row for userID. Race-safe:
	// if a concurrent request already created one, Create returns that
	// existing row instead of erroring or overwriting it — first writer
	// wins, exactly one key per user for the life of the deployment (key
	// rotation is out of scope for v1, see BACK-16).
	Create(ctx context.Context, row *dto.UserDataKeyDTO) (*dto.UserDataKeyDTO, error)
}
