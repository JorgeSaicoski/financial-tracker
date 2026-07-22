package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/domain/repositories"
)

type currencyRepository struct {
	db *sql.DB
}

// NewCurrencyRepository returns the domain interface type, not the
// concrete struct, so callers depend only on the contract.
func NewCurrencyRepository(db *sql.DB) repositories.CurrencyRepository {
	return &currencyRepository{db: db}
}

func (r *currencyRepository) List(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT code FROM currencies ORDER BY code ASC`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: query currencies: %w", err)
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
		`INSERT OR IGNORE INTO currencies (code, created_at) VALUES (?, ?)`,
		code, formatTime(time.Now()))
	if err != nil {
		return fmt.Errorf("sqlite: insert currency: %w", err)
	}
	return nil
}
