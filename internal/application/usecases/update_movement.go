package usecases

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/services"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

type updateMovementUseCase struct {
	repo       repositories.MovementRepository
	accounts   repositories.AccountRepository
	categories repositories.CategoryRepository
	sync       services.SyncTrigger
}

// NewUpdateMovement returns interface type for dependency injection.
func NewUpdateMovement(repo repositories.MovementRepository, accounts repositories.AccountRepository, categories repositories.CategoryRepository, sync services.SyncTrigger) UpdateMovementUseCase {
	return &updateMovementUseCase{repo: repo, accounts: accounts, categories: categories, sync: sync}
}

func (uc *updateMovementUseCase) Execute(ctx context.Context, userID, id string, input UpdateMovementInput) (UpdateMovementResult, error) {
	if userID == "" || id == "" {
		return UpdateMovementResult{}, apperrors.ErrInvalidInput
	}

	movementDTO, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return UpdateMovementResult{}, err
	}
	if movementDTO.UserID != userID {
		return UpdateMovementResult{}, apperrors.ErrNotFound
	}
	movement := movementDTO.ToEntity()
	if movement.IsCancelled() {
		return UpdateMovementResult{}, fmt.Errorf("%w: movement is already cancelled", apperrors.ErrConflict)
	}
	if movement.IsReversal() {
		// A reversal is itself a compensating entry; editing it would
		// desync it from the movement it exists to cancel.
		return UpdateMovementResult{}, fmt.Errorf("%w: can't edit a reversal movement", apperrors.ErrConflict)
	}

	if err := validateAvoidabilityPercent(input.AvoidabilityOverridePercent); err != nil {
		return UpdateMovementResult{}, err
	}

	editsFinancial := input.Amount != nil || input.Currency != nil || input.Timestamp != nil
	editsMetadata := input.Description != nil || input.CategoryID != nil || input.PaymentMethod != nil ||
		input.AccountID != nil || input.AvoidabilityOverridePercent != nil

	if editsFinancial && movement.CreditCardPurchaseID != nil {
		return UpdateMovementResult{}, fmt.Errorf(
			"%w: can't edit one installment's amount, currency or timestamp — cancel the purchase instead",
			apperrors.ErrConflict)
	}
	if editsFinancial && movement.TransferID != nil {
		// Editing one leg's amount/currency/timestamp alone would break
		// the transfer's zero-net-worth invariant.
		return UpdateMovementResult{}, fmt.Errorf(
			"%w: can't edit one transfer leg's amount, currency or timestamp — cancel the transfer instead",
			apperrors.ErrConflict)
	}

	description := orDefault(input.Description, movement.Description)
	paymentMethodInput := orDefault(input.PaymentMethod, string(movement.PaymentMethod))
	avoidabilityOverride := movement.AvoidabilityOverridePercent
	if input.AvoidabilityOverridePercent != nil {
		avoidabilityOverride = input.AvoidabilityOverridePercent
	}
	amount := orDefault(input.Amount, movement.Amount)
	currency := movement.Currency
	if input.Currency != nil {
		currency = strings.ToLower(strings.TrimSpace(*input.Currency))
		if currency == "" {
			return UpdateMovementResult{}, fmt.Errorf("%w: currency is required", apperrors.ErrInvalidInput)
		}
	}
	timestamp := orDefault(input.Timestamp, movement.Timestamp)

	accountID := movement.AccountID
	if input.AccountID != nil {
		if *input.AccountID == "" {
			accountID = nil
		} else {
			accountID = input.AccountID
		}
	}
	categoryIDInput := movement.CategoryID
	if input.CategoryID != nil {
		if *input.CategoryID == "" {
			categoryIDInput = nil
		} else {
			categoryIDInput = input.CategoryID
		}
	}

	paymentMethod, err := normalizePaymentMethod(paymentMethodInput)
	if err != nil {
		return UpdateMovementResult{}, err
	}
	categoryID, err := resolveCategoryID(ctx, uc.categories, userID, categoryIDInput)
	if err != nil {
		return UpdateMovementResult{}, err
	}
	if amount == 0 {
		return UpdateMovementResult{}, apperrors.ErrInvalidInput
	}
	if accountID != nil {
		account, err := uc.accounts.GetByID(ctx, *accountID)
		if apperrors.Is(err, apperrors.ErrNotFound) {
			return UpdateMovementResult{}, fmt.Errorf("%w: account not found", apperrors.ErrInvalidInput)
		}
		if err != nil {
			return UpdateMovementResult{}, err
		}
		if account.UserID != movement.UserID {
			return UpdateMovementResult{}, fmt.Errorf("%w: account not found", apperrors.ErrInvalidInput)
		}
		if account.Currency != currency {
			return UpdateMovementResult{}, fmt.Errorf("%w: movement currency %q does not match account currency %q",
				apperrors.ErrInvalidInput, currency, account.Currency)
		}
	}

	if !editsFinancial {
		if editsMetadata {
			if err := uc.repo.UpdateMetadata(ctx, movement.ID, description, categoryID, string(paymentMethod), accountID); err != nil {
				return UpdateMovementResult{}, err
			}
			if input.AvoidabilityOverridePercent != nil {
				if err := uc.repo.UpdateAvoidabilityOverride(ctx, movement.ID, avoidabilityOverride); err != nil {
					return UpdateMovementResult{}, err
				}
			}
			movementDTO.Description, movementDTO.CategoryID, movementDTO.PaymentMethod, movementDTO.AccountID =
				description, categoryID, string(paymentMethod), accountID
			movementDTO.AvoidabilityOverridePercent = avoidabilityOverride
		}
		return UpdateMovementResult{Movement: movementDTO}, nil
	}

	if !movement.IsSynced() {
		// Never reached ledger-service: every field can still be edited
		// in place.
		originalAmount, originalCurrency, originalTimestamp := movement.Amount, movement.Currency, movement.Timestamp
		if err := uc.repo.UpdateFinancial(ctx, movement.ID, amount, currency, timestamp); err != nil {
			return UpdateMovementResult{}, err
		}
		if editsMetadata {
			if err := uc.repo.UpdateMetadata(ctx, movement.ID, description, categoryID, string(paymentMethod), accountID); err != nil {
				if rollbackErr := uc.repo.UpdateFinancial(ctx, movement.ID, originalAmount, originalCurrency, originalTimestamp); rollbackErr != nil {
					return UpdateMovementResult{}, fmt.Errorf(
						"metadata update failed after financial update and rollback also failed: metadata: %w; rollback: %v",
						err, rollbackErr)
				}
				return UpdateMovementResult{}, err
			}
			if input.AvoidabilityOverridePercent != nil {
				if err := uc.repo.UpdateAvoidabilityOverride(ctx, movement.ID, avoidabilityOverride); err != nil {
					return UpdateMovementResult{}, err
				}
			}
			movementDTO.Description, movementDTO.CategoryID, movementDTO.PaymentMethod, movementDTO.AccountID =
				description, categoryID, string(paymentMethod), accountID
			movementDTO.AvoidabilityOverridePercent = avoidabilityOverride
		}
		movementDTO.Amount, movementDTO.Currency, movementDTO.Timestamp = amount, currency, timestamp
		return UpdateMovementResult{Movement: movementDTO}, nil
	}

	// Already in ledger-service, which never deletes: compensate the
	// original with a reversal (same mechanics as a plain cancel) and
	// create a fresh movement carrying the corrected financial values
	// plus whatever metadata was requested. The original stays exactly as
	// it was, just marked reversed, so it remains an accurate record of
	// what actually synced.
	var (
		replacement *dto.MovementDTO
		result      CancelMovementResult
	)
	err = uc.repo.Transact(ctx, func(tx repositories.MovementRepository) error {
		var err error
		result, err = cancelOne(ctx, tx, movementDTO)
		if err != nil {
			return err
		}

		now := time.Now().UTC()
		replacementEntity := &entities.Movement{
			UserID:                      movement.UserID,
			Amount:                      amount,
			Currency:                    currency,
			Description:                 description,
			CategoryID:                  categoryID,
			PaymentMethod:               paymentMethod,
			AvoidabilityOverridePercent: avoidabilityOverride,
			AccountID:                   accountID,
			Status:                      entities.MovementStatusActive,
			SyncStatus:                  entities.SyncStatusPending,
			Timestamp:                   timestamp,
			CreatedAt:                   now,
		}
		replacement, err = tx.Create(ctx, dto.MovementFromEntity(replacementEntity))
		return err
	})
	if err != nil {
		return UpdateMovementResult{}, err
	}
	uc.sync.TriggerAsync()
	return UpdateMovementResult{Movement: result.Movement, Reversal: result.Reversal, Replacement: replacement}, nil
}

// orDefault returns the patch value when present, else the current one —
// the merge rule for every PATCH field in this use case.
func orDefault[T any](patch *T, current T) T {
	if patch != nil {
		return *patch
	}
	return current
}
