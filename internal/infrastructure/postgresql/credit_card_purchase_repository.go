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

type creditCardPurchaseRepository struct {
	db *sql.DB
}

// NewCreditCardPurchaseRepository returns the application interface type,
// not the concrete struct, so callers depend only on the contract.
func NewCreditCardPurchaseRepository(db *sql.DB) repositories.CreditCardPurchaseRepository {
	return &creditCardPurchaseRepository{db: db}
}

// purchaseInsertColumns is the column list an INSERT into
// credit_card_purchases targets — category_id (BACK-14 follow-up) comes
// straight from the DTO now, no name resolution happens here.
const purchaseInsertColumns = `id, user_id, description, category_id, total_amount, currency,
	installment_count, purchase_date, status, created_at, card_id`

// purchaseSelectColumns/purchaseFromClause mirror
// movementSelectColumns/movementFromClause: a LEFT JOIN against
// categories resolves category_id back to a name, so
// dto.CreditCardPurchaseDTO.Category keeps behaving exactly as it did
// when category was a plain string column.
const purchaseSelectColumns = `credit_card_purchases.id, credit_card_purchases.user_id, credit_card_purchases.description,
	COALESCE(categories.name, '') AS category, credit_card_purchases.category_id, credit_card_purchases.total_amount, credit_card_purchases.currency,
	credit_card_purchases.installment_count, credit_card_purchases.purchase_date, credit_card_purchases.status,
	credit_card_purchases.created_at, credit_card_purchases.card_id`

const purchaseFromClause = `credit_card_purchases LEFT JOIN categories ON credit_card_purchases.category_id = categories.id`

func (r *creditCardPurchaseRepository) CreateWithInstallments(ctx context.Context, purchase *dto.CreditCardPurchaseDTO, installments []*dto.MovementDTO) (*dto.CreditCardPurchaseDTO, []*dto.MovementDTO, error) {
	if purchase.ID == "" {
		purchase.ID = id.NewUUID()
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("postgresql: begin purchase: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO credit_card_purchases (`+purchaseInsertColumns+`)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		purchase.ID, purchase.UserID, nullString(purchase.Description), strOrNil(purchase.CategoryID),
		purchase.TotalAmount, purchase.Currency, purchase.InstallmentCount,
		purchase.PurchaseDate, purchase.Status, purchase.CreatedAt, strOrNil(purchase.CardID))
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

func (r *creditCardPurchaseRepository) GetByID(ctx context.Context, purchaseID string) (*dto.CreditCardPurchaseDTO, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+purchaseSelectColumns+` FROM `+purchaseFromClause+` WHERE credit_card_purchases.id = $1`, purchaseID)
	p, err := scanPurchase(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	return p, err
}

func (r *creditCardPurchaseRepository) ListByUser(ctx context.Context, userID string) ([]*dto.CreditCardPurchaseDTO, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+purchaseSelectColumns+` FROM `+purchaseFromClause+` WHERE credit_card_purchases.user_id = $1 ORDER BY credit_card_purchases.purchase_date DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("postgresql: query purchases: %w", err)
	}
	defer rows.Close()

	out := make([]*dto.CreditCardPurchaseDTO, 0)
	for rows.Next() {
		p, err := scanPurchase(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// scanPurchase adapts one credit_card_purchases row to the application
// layer's CreditCardPurchaseDTO — shared by GetByID and ListByUser.
func scanPurchase(row scannable) (*dto.CreditCardPurchaseDTO, error) {
	var (
		p           dto.CreditCardPurchaseDTO
		description sql.NullString
		categoryID  sql.NullString
		cardID      sql.NullString
	)
	err := row.Scan(&p.ID, &p.UserID, &description, &p.Category, &categoryID, &p.TotalAmount, &p.Currency,
		&p.InstallmentCount, &p.PurchaseDate, &p.Status, &p.CreatedAt, &cardID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		return nil, fmt.Errorf("postgresql: scan purchase: %w", err)
	}
	p.Description = description.String
	p.CategoryID = stringPtr(categoryID)
	p.CardID = stringPtr(cardID)
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
