package postgresql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
)

type localArchiveSettingsRepository struct {
	db *sql.DB
}

// NewLocalArchiveSettingsRepository returns the application interface
// type, not the concrete struct, so callers depend only on the contract.
func NewLocalArchiveSettingsRepository(db *sql.DB) repositories.LocalArchiveSettingsRepository {
	return &localArchiveSettingsRepository{db: db}
}

func (r *localArchiveSettingsRepository) IsEnabled(ctx context.Context, userID string) (bool, error) {
	var enabled bool
	err := r.db.QueryRowContext(ctx,
		`SELECT local_archive_enabled FROM user_local_archive_settings WHERE user_id = $1`, userID).
		Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("postgresql: query local archive setting: %w", err)
	}
	return enabled, nil
}

func (r *localArchiveSettingsRepository) SetEnabled(ctx context.Context, userID string, enabled bool) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO user_local_archive_settings (user_id, local_archive_enabled, updated_at)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (user_id) DO UPDATE SET local_archive_enabled = excluded.local_archive_enabled, updated_at = excluded.updated_at`,
		userID, enabled, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("postgresql: upsert local archive setting: %w", err)
	}
	return nil
}
