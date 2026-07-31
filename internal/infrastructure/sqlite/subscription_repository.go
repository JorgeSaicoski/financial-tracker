package sqlite

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

type subscriptionRepository struct {
	db *sql.DB
}

// NewSubscriptionRepository returns the application interface type, not
// the concrete struct, so callers depend only on the contract.
func NewSubscriptionRepository(db *sql.DB) repositories.SubscriptionRepository {
	return &subscriptionRepository{db: db}
}

const subscriptionColumns = `user_id, provider, provider_subscription_id, status, current_period_end, created_at, updated_at`

// Upsert replaces the caller's current subscription row — one per user,
// keyed on user_id, so a resubscribe overwrites rather than accumulates
// history (mirrors user_settings' "current state" shape).
func (r *subscriptionRepository) Upsert(ctx context.Context, sub *dto.SubscriptionDTO) (*dto.SubscriptionDTO, error) {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO subscriptions (`+subscriptionColumns+`)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET
		   provider = excluded.provider,
		   provider_subscription_id = excluded.provider_subscription_id,
		   status = excluded.status,
		   current_period_end = excluded.current_period_end,
		   updated_at = excluded.updated_at`,
		sub.UserID, sub.Provider, sub.ProviderSubscriptionID, sub.Status,
		formatTime(sub.CurrentPeriodEnd), formatTime(now), formatTime(now))
	if err != nil {
		return nil, fmt.Errorf("sqlite: upsert subscription: %w", err)
	}
	return r.GetByUserID(ctx, sub.UserID)
}

func (r *subscriptionRepository) GetByUserID(ctx context.Context, userID string) (*dto.SubscriptionDTO, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+subscriptionColumns+` FROM subscriptions WHERE user_id = ?`, userID)
	s, err := scanSubscription(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: get subscription: %w", err)
	}
	return s, nil
}

func (r *subscriptionRepository) ListLapsable(ctx context.Context, asOf time.Time, graceDays int) ([]*dto.SubscriptionDTO, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+subscriptionColumns+` FROM subscriptions
		 WHERE status IN ('past_due', 'canceled') AND datetime(current_period_end, ?) <= ?`,
		fmt.Sprintf("+%d days", graceDays), formatTime(asOf))
	if err != nil {
		return nil, fmt.Errorf("sqlite: list lapsable subscriptions: %w", err)
	}
	defer rows.Close()

	out := make([]*dto.SubscriptionDTO, 0)
	for rows.Next() {
		s, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func scanSubscription(row scannable) (*dto.SubscriptionDTO, error) {
	var (
		s                                      dto.SubscriptionDTO
		currentPeriodEnd, createdAt, updatedAt string
	)
	err := row.Scan(&s.UserID, &s.Provider, &s.ProviderSubscriptionID, &s.Status,
		&currentPeriodEnd, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	if s.CurrentPeriodEnd, err = parseTime(currentPeriodEnd); err != nil {
		return nil, fmt.Errorf("sqlite: parse subscription current_period_end: %w", err)
	}
	if s.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, fmt.Errorf("sqlite: parse subscription created_at: %w", err)
	}
	if s.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return nil, fmt.Errorf("sqlite: parse subscription updated_at: %w", err)
	}
	return &s, nil
}
