package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/JorgeSaicoski/financial-tracker/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/application/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
	"github.com/JorgeSaicoski/financial-tracker/pkg/id"
)

type accountRepository struct {
	db *sql.DB
}

// NewAccountRepository returns the application interface type, not the
// concrete struct, so callers depend only on the contract.
func NewAccountRepository(db *sql.DB) repositories.AccountRepository {
	return &accountRepository{db: db}
}

const accountColumns = `id, user_id, name, type, currency, created_at`

func (r *accountRepository) Create(ctx context.Context, account *dto.AccountDTO) (*dto.AccountDTO, error) {
	if account.ID == "" {
		account.ID = id.NewUUID()
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO accounts (`+accountColumns+`) VALUES (?, ?, ?, ?, ?, ?)`,
		account.ID, account.UserID, account.Name, account.Type,
		account.Currency, formatTime(account.CreatedAt))
	if err != nil {
		return nil, fmt.Errorf("sqlite: insert account: %w", err)
	}
	return account, nil
}

func (r *accountRepository) GetByID(ctx context.Context, accountID string) (*dto.AccountDTO, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+accountColumns+` FROM accounts WHERE id = ?`, accountID)
	a, err := scanAccount(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	return a, err
}

func (r *accountRepository) ListByUser(ctx context.Context, userID string) ([]*dto.AccountDTO, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+accountColumns+` FROM accounts WHERE user_id = ? ORDER BY name ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: query accounts: %w", err)
	}
	defer rows.Close()

	out := make([]*dto.AccountDTO, 0)
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *accountRepository) AddSnapshot(ctx context.Context, snapshot *dto.AccountSnapshotDTO) (*dto.AccountSnapshotDTO, error) {
	if snapshot.ID == "" {
		snapshot.ID = id.NewUUID()
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO account_snapshots (id, account_id, balance, timestamp, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		snapshot.ID, snapshot.AccountID, snapshot.Balance,
		formatTime(snapshot.Timestamp), formatTime(snapshot.CreatedAt))
	if err != nil {
		return nil, fmt.Errorf("sqlite: insert snapshot: %w", err)
	}
	return snapshot, nil
}

func (r *accountRepository) LatestSnapshots(ctx context.Context, accountID string, n int) ([]*dto.AccountSnapshotDTO, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, account_id, balance, timestamp, created_at FROM account_snapshots
		 WHERE account_id = ? ORDER BY timestamp DESC, created_at DESC LIMIT ?`,
		accountID, n)
	if err != nil {
		return nil, fmt.Errorf("sqlite: query snapshots: %w", err)
	}
	defer rows.Close()

	out := make([]*dto.AccountSnapshotDTO, 0, n)
	for rows.Next() {
		var (
			s                    dto.AccountSnapshotDTO
			timestamp, createdAt string
		)
		if err := rows.Scan(&s.ID, &s.AccountID, &s.Balance, &timestamp, &createdAt); err != nil {
			return nil, err
		}
		if s.Timestamp, err = parseTime(timestamp); err != nil {
			return nil, fmt.Errorf("sqlite: parse snapshot timestamp: %w", err)
		}
		if s.CreatedAt, err = parseTime(createdAt); err != nil {
			return nil, fmt.Errorf("sqlite: parse snapshot created_at: %w", err)
		}
		out = append(out, &s)
	}
	return out, rows.Err()
}

// scanAccount adapts one account row to the application layer's
// AccountDTO — the contract this repository implements.
func scanAccount(row scannable) (*dto.AccountDTO, error) {
	var (
		a         dto.AccountDTO
		createdAt string
	)
	err := row.Scan(&a.ID, &a.UserID, &a.Name, &a.Type, &a.Currency, &createdAt)
	if err != nil {
		return nil, err
	}
	if a.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, fmt.Errorf("sqlite: parse account created_at: %w", err)
	}
	return &a, nil
}
