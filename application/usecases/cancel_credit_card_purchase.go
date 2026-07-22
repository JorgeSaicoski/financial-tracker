package usecases

import (
	"context"

	"github.com/JorgeSaicoski/financial-tracker/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/application/services"
	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
)

type cancelCreditCardPurchaseUseCase struct {
	purchases repositories.CreditCardPurchaseRepository
	movements repositories.MovementRepository
	sync      services.SyncTrigger
}

// NewCancelCreditCardPurchase returns interface type for dependency injection.
func NewCancelCreditCardPurchase(
	purchases repositories.CreditCardPurchaseRepository,
	movements repositories.MovementRepository,
	sync services.SyncTrigger,
) CancelCreditCardPurchaseUseCase {
	return &cancelCreditCardPurchaseUseCase{purchases: purchases, movements: movements, sync: sync}
}

func (uc *cancelCreditCardPurchaseUseCase) Execute(ctx context.Context, id string) (CancelCreditCardPurchaseResult, error) {
	if id == "" {
		return CancelCreditCardPurchaseResult{}, apperrors.ErrInvalidInput
	}

	purchase, err := uc.purchases.GetByID(ctx, id)
	if err != nil {
		return CancelCreditCardPurchaseResult{}, err
	}
	if purchase.Status == entities.CreditCardPurchaseStatusCancelled {
		return CancelCreditCardPurchaseResult{}, apperrors.ErrConflict
	}

	installments, err := uc.movements.ListByCreditCardPurchase(ctx, id)
	if err != nil {
		return CancelCreditCardPurchaseResult{}, err
	}

	result := CancelCreditCardPurchaseResult{Purchase: purchase}
	for _, installment := range installments {
		if installment.IsCancelled() {
			// Individually cancelled earlier — nothing more to do for it.
			continue
		}
		one, err := cancelOne(ctx, uc.movements, installment)
		if err != nil {
			return CancelCreditCardPurchaseResult{}, err
		}
		if one.Reversal != nil {
			result.Reversals = append(result.Reversals, one.Reversal)
		} else {
			result.Voided = append(result.Voided, one.Movement)
		}
	}

	if err := uc.purchases.MarkCancelled(ctx, id); err != nil {
		return CancelCreditCardPurchaseResult{}, err
	}
	purchase.Status = entities.CreditCardPurchaseStatusCancelled

	if len(result.Reversals) > 0 {
		uc.sync.TriggerAsync()
	}
	return result, nil
}
