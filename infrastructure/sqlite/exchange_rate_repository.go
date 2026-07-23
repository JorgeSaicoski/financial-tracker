package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/application/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
	"github.com/JorgeSaicoski/financial-tracker/pkg/id"
)

type exchangeRateRepository struct {
	db *sql.DB
}

// NewExchangeRateRepository returns the application interface type, not
// the concrete struct, so callers depend only on the contract.
func NewExchangeRateRepository(db *sql.DB) repositories.ExchangeRateRepository {
	return &exchangeRateRepository{db: db}
}

const exchangeRateColumns = `id, user_id, currency, units_per_usd, effective_from, created_at`

// Create upserts on (user_id, currency, effective_from): a second POST for
// the same date replaces UnitsPerUSD in place, keeping the original id and
// created_at, which is what makes fixing a typo in history idempotent
// rather than a growing pile of duplicate rows.
func (r *exchangeRateRepository) Create(ctx context.Context, rate *dto.ExchangeRateDTO) (*dto.ExchangeRateDTO, error) {
	if rate.ID == "" {
		rate.ID = id.NewUUID()
	}
	row := r.db.QueryRowContext(ctx,
		`INSERT INTO exchange_rates (`+exchangeRateColumns+`) VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(user_id, currency, effective_from) DO UPDATE SET units_per_usd = excluded.units_per_usd
		 RETURNING id, created_at`,
		rate.ID, rate.UserID, rate.Currency, rate.UnitsPerUSD,
		formatTime(rate.EffectiveFrom), formatTime(rate.CreatedAt))

	var (
		returnedID, createdAt string
	)
	if err := row.Scan(&returnedID, &createdAt); err != nil {
		return nil, fmt.Errorf("sqlite: upsert exchange rate: %w", err)
	}
	parsed, err := parseTime(createdAt)
	if err != nil {
		return nil, fmt.Errorf("sqlite: parse exchange rate created_at: %w", err)
	}
	rate.ID = returnedID
	rate.CreatedAt = parsed
	return rate, nil
}

func (r *exchangeRateRepository) ListByUser(ctx context.Context, userID string) ([]*dto.ExchangeRateDTO, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+exchangeRateColumns+` FROM exchange_rates WHERE user_id = ?
		 ORDER BY currency ASC, effective_from DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: query exchange rates: %w", err)
	}
	defer rows.Close()

	out := make([]*dto.ExchangeRateDTO, 0)
	for rows.Next() {
		rate, err := scanExchangeRate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rate)
	}
	return out, rows.Err()
}

func (r *exchangeRateRepository) RateAt(ctx context.Context, userID, currency string, at time.Time) (*dto.ExchangeRateDTO, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+exchangeRateColumns+` FROM exchange_rates
		 WHERE user_id = ? AND currency = ? AND effective_from <= ?
		 ORDER BY effective_from DESC LIMIT 1`,
		userID, currency, formatTime(at))
	rate, err := scanExchangeRate(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	return rate, err
}

func (r *exchangeRateRepository) Delete(ctx context.Context, userID, rateID string) error {
	result, err := r.db.ExecContext(ctx,
		`DELETE FROM exchange_rates WHERE id = ? AND user_id = ?`, rateID, userID)
	if err != nil {
		return fmt.Errorf("sqlite: delete exchange rate: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: delete exchange rate rows affected: %w", err)
	}
	if n == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}

// scanExchangeRate adapts one exchange_rates row to the application
// layer's ExchangeRateDTO — the contract this repository implements.
func scanExchangeRate(row scannable) (*dto.ExchangeRateDTO, error) {
	var (
		rate                     dto.ExchangeRateDTO
		effectiveFrom, createdAt string
	)
	err := row.Scan(&rate.ID, &rate.UserID, &rate.Currency, &rate.UnitsPerUSD, &effectiveFrom, &createdAt)
	if err != nil {
		return nil, err
	}
	if rate.EffectiveFrom, err = parseTime(effectiveFrom); err != nil {
		return nil, fmt.Errorf("sqlite: parse exchange rate effective_from: %w", err)
	}
	if rate.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, fmt.Errorf("sqlite: parse exchange rate created_at: %w", err)
	}
	return &rate, nil
}
