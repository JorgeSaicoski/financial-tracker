package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
)

type userSettingsRepository struct {
	db *sql.DB
}

// NewUserSettingsRepository returns the application interface type, not
// the concrete struct, so callers depend only on the contract.
func NewUserSettingsRepository(db *sql.DB) repositories.UserSettingsRepository {
	return &userSettingsRepository{db: db}
}

const userSettingsColumns = `user_id, ledger_sync_entitled, ledger_sync_enabled, cloud_storage_entitled, created_at, updated_at`

func (r *userSettingsRepository) Get(ctx context.Context, userID string) (*dto.UserSettingsDTO, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+userSettingsColumns+` FROM user_settings WHERE user_id = ?`, userID)
	s, err := scanUserSettings(row)
	if errors.Is(err, sql.ErrNoRows) {
		// Absence of a row means "everything true" — no backfill needed
		// for existing users (see the repository contract's doc comment).
		return dto.DefaultUserSettings(userID, time.Now().UTC()), nil
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: get user settings: %w", err)
	}
	return s, nil
}

// UpdateEnabled upserts ledger_sync_enabled, creating the row lazily on
// first write with the true/true/true defaults if it doesn't exist yet.
// On conflict, only ledger_sync_enabled and updated_at change — the
// entitlement fields are left exactly as they already are (operator/
// billing-controlled, never touched from here).
func (r *userSettingsRepository) UpdateEnabled(ctx context.Context, userID string, ledgerSyncEnabled bool) (*dto.UserSettingsDTO, error) {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO user_settings (`+userSettingsColumns+`)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET
		   ledger_sync_enabled = excluded.ledger_sync_enabled,
		   updated_at = excluded.updated_at`,
		userID, true, ledgerSyncEnabled, true, formatTime(now), formatTime(now))
	if err != nil {
		return nil, fmt.Errorf("sqlite: upsert user settings: %w", err)
	}
	return r.Get(ctx, userID)
}

func (r *userSettingsRepository) ListSyncDisabledUserIDs(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT user_id FROM user_settings WHERE ledger_sync_entitled = 0 OR ledger_sync_enabled = 0`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list sync-disabled users: %w", err)
	}
	defer rows.Close()

	out := make([]string, 0)
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		out = append(out, userID)
	}
	return out, rows.Err()
}

func scanUserSettings(row scannable) (*dto.UserSettingsDTO, error) {
	var (
		s                    dto.UserSettingsDTO
		createdAt, updatedAt string
	)
	err := row.Scan(&s.UserID, &s.LedgerSyncEntitled, &s.LedgerSyncEnabled, &s.CloudStorageEntitled, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	if s.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, fmt.Errorf("sqlite: parse user_settings created_at: %w", err)
	}
	if s.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return nil, fmt.Errorf("sqlite: parse user_settings updated_at: %w", err)
	}
	return &s, nil
}
