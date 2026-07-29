package postgresql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

type limitsRepository struct {
	db *sql.DB
}

// NewLimitsRepository returns the application interface type, not the
// concrete struct, so callers depend only on the contract.
func NewLimitsRepository(db *sql.DB) repositories.LimitsRepository {
	return &limitsRepository{db: db}
}

func (r *limitsRepository) GetValue(ctx context.Context, name string) (int, error) {
	var value int
	err := r.db.QueryRowContext(ctx, `SELECT value FROM limits WHERE name = $1`, name).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, apperrors.ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("postgresql: query limit %q: %w", name, err)
	}
	return value, nil
}
