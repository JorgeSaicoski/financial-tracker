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

type creditCardPurchaseRepository struct {
	db *sql.DB
}

// NewCreditCardPurchaseRepository returns the domain interface type, not
// the concrete struct, so callers depend only on the contract.
func NewCreditCardPurchaseRepository(db *sql.DB) repositories.CreditCardPurchaseRepository {
	return &creditCardPurchaseRepository{db: db}
}

const purchaseColumns = `id, user_id, description, category, total_amount, currency,
	installment_count, purchase_date, status, created_at`

func (r *creditCardPurchaseRepository) CreateWithInstallments(ctx context.Context, purchase *entities.CreditCardPurchase, installments []*entities.Movement) (*entities.CreditCardPurchase, []*entities.Movement, error) {
	if purchase.ID == "" {
		purchase.ID = id.NewUUID()
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("postgresql: begin purchase: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO credit_card_purchases (`+purchaseColumns+`)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		purchase.ID, purchase.UserID, nullString(purchase.Description), string(purchase.Category),
		purchase.TotalAmount, purchase.Currency, purchase.InstallmentCount,
		purchase.PurchaseDate, string(purchase.Status), purchase.CreatedAt)
	if err != nil {
		return nil, nil, fmt.Errorf("postgresql: insert purchase: %w", err)
	}

	for _, m := range installments {
		if m.ID == "" {
			m.ID = id.NewUUID()
		}
		m.CreditCardPurchaseID = &purchase.ID
		if err := insertMovement(ctx, tx, m); err != nil {
			return nil, nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("postgresql: commit purchase: %w", err)
	}
	return purchase, installments, nil
}

func (r *creditCardPurchaseRepository) GetByID(ctx context.Context, purchaseID string) (*entities.CreditCardPurchase, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+purchaseColumns+` FROM credit_card_purchases WHERE id = $1`, purchaseID)

	var (
		p           entities.CreditCardPurchase
		description sql.NullString
		category    string
		status      string
	)
	err := row.Scan(&p.ID, &p.UserID, &description, &category, &p.TotalAmount, &p.Currency,
		&p.InstallmentCount, &p.PurchaseDate, &status, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgresql: scan purchase: %w", err)
	}

	p.Description = description.String
	p.Category = entities.Category(category)
	p.Status = entities.CreditCardPurchaseStatus(status)
	return &p, nil
}

func (r *creditCardPurchaseRepository) MarkCancelled(ctx context.Context, purchaseID string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE credit_card_purchases SET status = 'cancelled' WHERE id = $1`, purchaseID)
	if err != nil {
		return fmt.Errorf("postgresql: cancel purchase: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}
