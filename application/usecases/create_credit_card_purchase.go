package usecases

import (
	"context"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
)

type createCreditCardPurchaseUseCase struct {
	purchases repositories.CreditCardPurchaseRepository
}

// NewCreateCreditCardPurchase returns interface type for dependency injection.
func NewCreateCreditCardPurchase(purchases repositories.CreditCardPurchaseRepository) CreateCreditCardPurchaseUseCase {
	return &createCreditCardPurchaseUseCase{purchases: purchases}
}

func (uc *createCreditCardPurchaseUseCase) Execute(ctx context.Context, input CreateCreditCardPurchaseInput) (*entities.CreditCardPurchase, []*entities.Movement, error) {
	if input.UserID == "" || input.Currency == "" || input.TotalAmount == 0 {
		return nil, nil, apperrors.ErrInvalidInput
	}
	if input.Installments < 2 {
		return nil, nil, apperrors.ErrInvalidInput
	}

	category, _, err := normalizeCategoryAndMethod(input.Category, entities.PaymentMethodCreditCard)
	if err != nil {
		return nil, nil, err
	}

	// Integer division truncates toward zero, so this works for both
	// signs: the remainder lands on the last installment and the parts
	// always sum exactly to the total.
	base := input.TotalAmount / int64(input.Installments)
	remainder := input.TotalAmount - base*int64(input.Installments)
	if base == 0 {
		// A total too small to split leaves zero-amount installments,
		// which ledger-service rejects — they could never sync.
		return nil, nil, apperrors.ErrInvalidInput
	}

	now := time.Now().UTC()
	purchase := &entities.CreditCardPurchase{
		UserID:           input.UserID,
		Description:      input.Description,
		Category:         category,
		TotalAmount:      input.TotalAmount,
		Currency:         input.Currency,
		InstallmentCount: input.Installments,
		PurchaseDate:     now,
		Status:           entities.CreditCardPurchaseStatusActive,
		CreatedAt:        now,
	}

	installments := make([]*entities.Movement, input.Installments)
	for i := range installments {
		amount := base
		if i == input.Installments-1 {
			amount += remainder
		}
		number := i + 1
		installments[i] = &entities.Movement{
			UserID:            input.UserID,
			Amount:            amount,
			Currency:          input.Currency,
			Description:       input.Description,
			Category:          category,
			PaymentMethod:     entities.PaymentMethodCreditCard,
			InstallmentNumber: &number,
			Status:            entities.MovementStatusActive,
			SyncStatus:        entities.SyncStatusPending,
			// One installment per month starting now. Future ones are
			// invisible to the sync worker until their date arrives.
			Timestamp: now.AddDate(0, i, 0),
			CreatedAt: now,
		}
	}

	return uc.purchases.CreateWithInstallments(ctx, purchase, installments)
}
