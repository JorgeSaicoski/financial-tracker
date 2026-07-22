package repositories

import (
	"context"

	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
)

// CreditCardPurchaseRepository persists installment-purchase grouping
// records alongside their movements.
type CreditCardPurchaseRepository interface {
	// CreateWithInstallments atomically inserts the purchase and all its
	// installment movements (linking them to the purchase's generated ID)
	// — either everything lands or nothing does.
	CreateWithInstallments(ctx context.Context, purchase *entities.CreditCardPurchase, installments []*entities.Movement) (*entities.CreditCardPurchase, []*entities.Movement, error)
	GetByID(ctx context.Context, id string) (*entities.CreditCardPurchase, error)
	MarkCancelled(ctx context.Context, id string) error
}
