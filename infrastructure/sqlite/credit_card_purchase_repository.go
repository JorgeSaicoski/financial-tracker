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

type creditCardPurchaseRepository struct {
	db *sql.DB
}

// NewCreditCardPurchaseRepository returns the application interface type,
// not the concrete struct, so callers depend only on the contract.
func NewCreditCardPurchaseRepository(db *sql.DB) repositories.CreditCardPurchaseRepository {
	return &creditCardPurchaseRepository{db: db}
}

const purchaseColumns = `id, user_id, description, category, total_amount, currency,
	installment_count, purchase_date, status, created_at`

func (r *creditCardPurchaseRepository) CreateWithInstallments(ctx context.Context, purchase *dto.CreditCardPurchaseDTO, installments []*dto.MovementDTO) (*dto.CreditCardPurchaseDTO, []*dto.MovementDTO, error) {
	if purchase.ID == "" {
		purchase.ID = id.NewUUID()
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("sqlite: begin purchase: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO credit_card_purchases (`+purchaseColumns+`)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		purchase.ID, purchase.UserID, nullString(purchase.Description), purchase.Category,
		purchase.TotalAmount, purchase.Currency, purchase.InstallmentCount,
		formatTime(purchase.PurchaseDate), purchase.Status, formatTime(purchase.CreatedAt))
	if err != nil {
		return nil, nil, fmt.Errorf("sqlite: insert purchase: %w", err)
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
		return nil, nil, fmt.Errorf("sqlite: commit purchase: %w", err)
	}
	return purchase, installments, nil
}

func (r *creditCardPurchaseRepository) GetByID(ctx context.Context, purchaseID string) (*dto.CreditCardPurchaseDTO, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+purchaseColumns+` FROM credit_card_purchases WHERE id = ?`, purchaseID)

	var (
		p           dto.CreditCardPurchaseDTO
		description sql.NullString
		date, born  string
	)
	err := row.Scan(&p.ID, &p.UserID, &description, &p.Category, &p.TotalAmount, &p.Currency,
		&p.InstallmentCount, &date, &p.Status, &born)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: scan purchase: %w", err)
	}

	p.Description = description.String
	if p.PurchaseDate, err = parseTime(date); err != nil {
		return nil, fmt.Errorf("sqlite: parse purchase_date: %w", err)
	}
	if p.CreatedAt, err = parseTime(born); err != nil {
		return nil, fmt.Errorf("sqlite: parse created_at: %w", err)
	}
	return &p, nil
}

func (r *creditCardPurchaseRepository) MarkCancelled(ctx context.Context, purchaseID string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE credit_card_purchases SET status = 'cancelled' WHERE id = ?`, purchaseID)
	if err != nil {
		return fmt.Errorf("sqlite: cancel purchase: %w", err)
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
