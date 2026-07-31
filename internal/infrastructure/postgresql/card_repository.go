package postgresql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/id"
)

type cardRepository struct {
	db *sql.DB
}

// NewCardRepository returns the application interface type, not the
// concrete struct, so callers depend only on the contract.
func NewCardRepository(db *sql.DB) repositories.CardRepository {
	return &cardRepository{db: db}
}

const cardColumns = `id, user_id, name, last_four, closing_day, due_day, credit_limit, monthly_budget, currency, created_at`

func (r *cardRepository) Create(ctx context.Context, c *dto.CardDTO) (*dto.CardDTO, error) {
	if c.ID == "" {
		c.ID = id.NewUUID()
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO cards (`+cardColumns+`) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		c.ID, c.UserID, c.Name, nullString(c.LastFour), c.ClosingDay, c.DueDay,
		nullInt64(c.CreditLimit), nullInt64(c.MonthlyBudget), c.Currency, c.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("postgresql: insert card: %w", err)
	}
	return c, nil
}

func (r *cardRepository) GetByID(ctx context.Context, userID, cardID string) (*dto.CardDTO, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+cardColumns+` FROM cards WHERE id = $1 AND user_id = $2`, cardID, userID)
	c, err := scanCard(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	return c, err
}

func (r *cardRepository) ListByUser(ctx context.Context, userID string) ([]*dto.CardDTO, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+cardColumns+` FROM cards WHERE user_id = $1 ORDER BY name ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("postgresql: query cards: %w", err)
	}
	defer rows.Close()

	out := make([]*dto.CardDTO, 0)
	for rows.Next() {
		c, err := scanCard(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *cardRepository) Update(ctx context.Context, userID, cardID string, c *dto.CardDTO) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE cards SET name = $1, last_four = $2, closing_day = $3, due_day = $4, credit_limit = $5, monthly_budget = $6
		 WHERE id = $7 AND user_id = $8`,
		c.Name, nullString(c.LastFour), c.ClosingDay, c.DueDay,
		nullInt64(c.CreditLimit), nullInt64(c.MonthlyBudget), cardID, userID)
	if err != nil {
		return fmt.Errorf("postgresql: update card: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("postgresql: update card rows affected: %w", err)
	}
	if n == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}

func (r *cardRepository) Delete(ctx context.Context, userID, cardID string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM cards WHERE id = $1 AND user_id = $2`, cardID, userID)
	if err != nil {
		return fmt.Errorf("postgresql: delete card: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("postgresql: delete card rows affected: %w", err)
	}
	if n == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}

func (r *cardRepository) IsReferenced(ctx context.Context, cardID string) (bool, error) {
	var referenced bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM movements WHERE card_id = $1 OR card_payment_for_card_id = $1
			UNION ALL
			SELECT 1 FROM credit_card_purchases WHERE card_id = $1
		)`, cardID).Scan(&referenced)
	if err != nil {
		return false, fmt.Errorf("postgresql: check card references: %w", err)
	}
	return referenced, nil
}

func nullInt64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func int64Ptr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

func scanCard(row scannable) (*dto.CardDTO, error) {
	var (
		c                          dto.CardDTO
		lastFour                   sql.NullString
		creditLimit, monthlyBudget sql.NullInt64
	)
	err := row.Scan(&c.ID, &c.UserID, &c.Name, &lastFour, &c.ClosingDay, &c.DueDay,
		&creditLimit, &monthlyBudget, &c.Currency, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	c.LastFour = lastFour.String
	c.CreditLimit = int64Ptr(creditLimit)
	c.MonthlyBudget = int64Ptr(monthlyBudget)
	return &c, nil
}
