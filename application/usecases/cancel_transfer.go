package usecases

import (
	"context"

	"github.com/JorgeSaicoski/financial-tracker/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/application/services"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
)

type cancelTransferUseCase struct {
	movements repositories.MovementRepository
	sync      services.SyncTrigger
}

// NewCancelTransfer returns interface type for dependency injection.
func NewCancelTransfer(movements repositories.MovementRepository, sync services.SyncTrigger) CancelTransferUseCase {
	return &cancelTransferUseCase{movements: movements, sync: sync}
}

func (uc *cancelTransferUseCase) Execute(ctx context.Context, transferID string) (CancelTransferResult, error) {
	if transferID == "" {
		return CancelTransferResult{}, apperrors.ErrInvalidInput
	}

	legs, err := uc.movements.ListByTransferID(ctx, transferID)
	if err != nil {
		return CancelTransferResult{}, err
	}
	if len(legs) == 0 {
		return CancelTransferResult{}, apperrors.ErrNotFound
	}
if len(legs) == 0 {
	return CancelTransferResult{}, apperrors.ErrNotFound
}
if len(legs) != 2 || legs[0].Amount >= 0 || legs[1].Amount <= 0 {
	return CancelTransferResult{}, apperrors.ErrNotFound
}
if legs[0].IsCancelled() && legs[1].IsCancelled() {
	return CancelTransferResult{}, apperrors.ErrConflict
}

var result CancelTransferResult
	anySynced := false

	err = uc.movements.Transact(ctx, func(tx repositories.MovementRepository) error {
		anySynced = false // reset in case the callback is ever re-invoked
		for _, leg := range legs {
			one := CancelMovementResult{Movement: leg}
			if !leg.IsCancelled() {
				var cancelErr error
				one, cancelErr = cancelOne(ctx, tx, leg)
				if cancelErr != nil {
					return cancelErr
				}
				if one.Reversal != nil {
					anySynced = true
				}
			}
			if leg.Amount < 0 {
				result.Debit = one
			} else {
				result.Credit = one
			}
		}
		return nil
	})
	if err != nil {
		return CancelTransferResult{}, err
	}

	if anySynced {
		uc.sync.TriggerAsync()
	}
	return result, nil
}
