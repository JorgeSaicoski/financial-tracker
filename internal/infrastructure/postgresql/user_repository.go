package postgresql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

type userRepository struct {
	db *sql.DB
}

// NewUserRepository returns the application interface type, not the
// concrete struct, so callers depend only on the contract.
func NewUserRepository(db *sql.DB) repositories.UserRepository {
	return &userRepository{db: db}
}

const userColumns = `id, provider, external_id, email, display_name, created_at, updated_at`

func (r *userRepository) Upsert(ctx context.Context, user *dto.UserDTO) (*dto.UserDTO, error) {
	now := time.Now().UTC()
	createdAt := user.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO users (`+userColumns+`) VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT(id) DO UPDATE SET
		   provider = excluded.provider,
		   external_id = excluded.external_id,
		   email = excluded.email,
		   display_name = excluded.display_name,
		   updated_at = excluded.updated_at`,
		user.ID, user.Provider, user.ExternalID, user.Email, user.DisplayName,
		createdAt, now)
	if err != nil {
		return nil, fmt.Errorf("postgresql: upsert user: %w", err)
	}
	return r.GetByID(ctx, user.ID)
}

func (r *userRepository) GetByID(ctx context.Context, userID string) (*dto.UserDTO, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, userID)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	return u, err
}

// scanUser adapts one user row to the application layer's UserDTO — the
// contract this repository implements.
func scanUser(row scannable) (*dto.UserDTO, error) {
	var u dto.UserDTO
	err := row.Scan(&u.ID, &u.Provider, &u.ExternalID, &u.Email, &u.DisplayName,
		&u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}
