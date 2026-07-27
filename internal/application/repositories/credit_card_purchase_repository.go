package repositories

import (
	"context"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
)

// CreditCardPurchaseRepository persists installment-purchase grouping
// records alongside their movements, expressed in application/dto types.
type CreditCardPurchaseRepository interface {
	// CreateWithInstallments atomically inserts the purchase and all its
	// installment movements (linking them to the purchase's generated ID)
	// — either everything lands or nothing does.
	CreateWithInstallments(ctx context.Context, purchase *dto.CreditCardPurchaseDTO, installments []*dto.MovementDTO) (*dto.CreditCardPurchaseDTO, []*dto.MovementDTO, error)
	GetByID(ctx context.Context, id string) (*dto.CreditCardPurchaseDTO, error)
	// ListByUser returns every purchase grouping record for the user,
	// newest purchase_date first. Added for BACK-15's full-account export
	// — nothing else needed a bulk read of a user's purchases before.
	ListByUser(ctx context.Context, userID string) ([]*dto.CreditCardPurchaseDTO, error)
	MarkCancelled(ctx context.Context, id string) error
}
