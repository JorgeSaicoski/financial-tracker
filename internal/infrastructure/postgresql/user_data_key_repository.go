package postgresql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

type userDataKeyRepository struct {
	db *sql.DB
}

// NewUserDataKeyRepository returns the application interface type, not
// the concrete struct, so callers depend only on the contract.
func NewUserDataKeyRepository(db *sql.DB) repositories.UserDataKeyRepository {
	return &userDataKeyRepository{db: db}
}

const userDataKeyColumns = `user_id, wrapped_key, created_at`

func (r *userDataKeyRepository) Get(ctx context.Context, userID string) (*dto.UserDataKeyDTO, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+userDataKeyColumns+` FROM user_data_keys WHERE user_id = $1`, userID)
	k, err := scanUserDataKey(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgresql: get user data key: %w", err)
	}
	return k, nil
}

// Create inserts userID's wrapped data key. If a concurrent request
// already created one, the conflict is a no-op and the pre-existing row
// is returned instead — first writer wins, exactly one key per user.
func (r *userDataKeyRepository) Create(ctx context.Context, k *dto.UserDataKeyDTO) (*dto.UserDataKeyDTO, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO user_data_keys (`+userDataKeyColumns+`) VALUES ($1, $2, $3) ON CONFLICT (user_id) DO NOTHING`,
		k.UserID, k.WrappedKey, k.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("postgresql: insert user data key: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return r.Get(ctx, k.UserID)
	}
	return k, nil
}

func scanUserDataKey(row scannable) (*dto.UserDataKeyDTO, error) {
	var k dto.UserDataKeyDTO
	if err := row.Scan(&k.UserID, &k.WrappedKey, &k.CreatedAt); err != nil {
		return nil, err
	}
	return &k, nil
}
