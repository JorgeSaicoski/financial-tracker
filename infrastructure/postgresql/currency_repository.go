package postgresql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/application/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
)

type currencyRepository struct {
	db *sql.DB
}

// NewCurrencyRepository returns the application interface type, not the
// concrete struct, so callers depend only on the contract.
func NewCurrencyRepository(db *sql.DB) repositories.CurrencyRepository {
	return &currencyRepository{db: db}
}

func (r *currencyRepository) List(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT code FROM currencies ORDER BY code ASC`)
	if err != nil {
		return nil, fmt.Errorf("postgresql: query currencies: %w", err)
	}
	defer rows.Close()

	out := make([]string, 0)
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		out = append(out, code)
	}
	return out, rows.Err()
}

func (r *currencyRepository) Add(ctx context.Context, code string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO currencies (code, created_at) VALUES ($1, $2) ON CONFLICT (code) DO NOTHING`,
		code, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("postgresql: insert currency: %w", err)
	}
	return nil
}

func (r *currencyRepository) Decimals(ctx context.Context, code string) (int, error) {
	var decimals int
	err := r.db.QueryRowContext(ctx, `SELECT decimals FROM currencies WHERE code = $1`, code).Scan(&decimals)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, apperrors.ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("postgresql: query currency decimals: %w", err)
	}
	return decimals, nil
}
