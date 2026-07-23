package postgresql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/JorgeSaicoski/financial-tracker/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
	"github.com/JorgeSaicoski/financial-tracker/pkg/id"
)

type accountRepository struct {
	db *sql.DB
}

// NewAccountRepository returns the domain interface type, not the
// concrete struct, so callers depend only on the contract.
func NewAccountRepository(db *sql.DB) repositories.AccountRepository {
	return &accountRepository{db: db}
}

const accountColumns = `id, user_id, name, type, currency, created_at`

func (r *accountRepository) Create(ctx context.Context, account *entities.Account) (*entities.Account, error) {
	if account.ID == "" {
		account.ID = id.NewUUID()
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO accounts (`+accountColumns+`) VALUES ($1, $2, $3, $4, $5, $6)`,
		account.ID, account.UserID, account.Name, string(account.Type),
		account.Currency, account.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("postgresql: insert account: %w", err)
	}
	return account, nil
}

func (r *accountRepository) GetByID(ctx context.Context, accountID string) (*entities.Account, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+accountColumns+` FROM accounts WHERE id = $1`, accountID)
	a, err := scanAccount(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	return a, err
}

func (r *accountRepository) ListByUser(ctx context.Context, userID string) ([]*entities.Account, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+accountColumns+` FROM accounts WHERE user_id = $1 ORDER BY name ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("postgresql: query accounts: %w", err)
	}
	defer rows.Close()

	out := make([]*entities.Account, 0)
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *accountRepository) AddSnapshot(ctx context.Context, snapshot *entities.AccountSnapshot) (*entities.AccountSnapshot, error) {
	if snapshot.ID == "" {
		snapshot.ID = id.NewUUID()
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO account_snapshots (id, account_id, balance, timestamp, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		snapshot.ID, snapshot.AccountID, snapshot.Balance,
		snapshot.Timestamp, snapshot.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("postgresql: insert snapshot: %w", err)
	}
	return snapshot, nil
}

func (r *accountRepository) LatestSnapshots(ctx context.Context, accountID string, n int) ([]*entities.AccountSnapshot, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, account_id, balance, timestamp, created_at FROM account_snapshots
		 WHERE account_id = $1 ORDER BY timestamp DESC, created_at DESC LIMIT $2`,
		accountID, n)
	if err != nil {
		return nil, fmt.Errorf("postgresql: query snapshots: %w", err)
	}
	defer rows.Close()

	out := make([]*entities.AccountSnapshot, 0, n)
	for rows.Next() {
		var s entities.AccountSnapshot
		if err := rows.Scan(&s.ID, &s.AccountID, &s.Balance, &s.Timestamp, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &s)
	}
	return out, rows.Err()
}

func scanAccount(row scannable) (*entities.Account, error) {
	var (
		a       entities.Account
		accType string
	)
	err := row.Scan(&a.ID, &a.UserID, &a.Name, &accType, &a.Currency, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	a.Type = entities.AccountType(accType)
	return &a, nil
}
