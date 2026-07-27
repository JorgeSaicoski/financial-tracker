package postgresql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

type ledgerPseudonymRepository struct {
	db *sql.DB
}

// NewLedgerPseudonymRepository returns the application interface type,
// not the concrete struct, so callers depend only on the contract.
func NewLedgerPseudonymRepository(db *sql.DB) repositories.LedgerPseudonymRepository {
	return &ledgerPseudonymRepository{db: db}
}

const ledgerPseudonymColumns = `user_id, pseudonym_id, created_at`

func (r *ledgerPseudonymRepository) Get(ctx context.Context, userID string) (*dto.LedgerPseudonymDTO, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+ledgerPseudonymColumns+` FROM user_ledger_pseudonyms WHERE user_id = $1`, userID)
	p, err := scanLedgerPseudonym(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgresql: get ledger pseudonym: %w", err)
	}
	return p, nil
}

// Create inserts userID's pseudonym. If a concurrent request already
// created one, the conflict is a no-op and the pre-existing row is
// returned instead — first writer wins, exactly one pseudonym per user.
func (r *ledgerPseudonymRepository) Create(ctx context.Context, p *dto.LedgerPseudonymDTO) (*dto.LedgerPseudonymDTO, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO user_ledger_pseudonyms (`+ledgerPseudonymColumns+`) VALUES ($1, $2, $3) ON CONFLICT (user_id) DO NOTHING`,
		p.UserID, p.PseudonymID, p.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("postgresql: insert ledger pseudonym: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return r.Get(ctx, p.UserID)
	}
	return p, nil
}

func scanLedgerPseudonym(row scannable) (*dto.LedgerPseudonymDTO, error) {
	var p dto.LedgerPseudonymDTO
	if err := row.Scan(&p.UserID, &p.PseudonymID, &p.CreatedAt); err != nil {
		return nil, err
	}
	return &p, nil
}
