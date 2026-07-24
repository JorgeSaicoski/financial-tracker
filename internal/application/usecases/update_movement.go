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

// UpdateMovementInput carries a PATCH /movements/{id} partial body — a nil
// field means "leave unchanged". Description/Category/PaymentMethod/
// AccountID are metadata: local-only, always editable regardless of sync
// status. Amount/Currency/Timestamp are financial: editable in place only
// before the movement syncs; once synced, editing them produces a
// reversal + a replacement instead (see UpdateMovementResult).
type UpdateMovementInput struct {
	Description   *string
	Category      *string
	PaymentMethod *string
	AccountID     *string // a pointer to "" clears the account
	Amount        *int64
	Currency      *string
	Timestamp     *time.Time
}

// UpdateMovementResult reports how the edit was carried out. A
// metadata-only edit, or a financial edit on a not-yet-synced movement,
// updates Movement in place (Reversal/Replacement nil). A financial edit
// on an already-synced movement leaves Movement untouched other than the
// reversal link and returns the compensating Reversal plus the
// Replacement movement carrying the corrected values — mirroring
// CancelMovementResult's shape for the same reason (ledger-service never
// deletes).
type UpdateMovementResult struct {
	Movement    *dto.MovementDTO
	Reversal    *dto.MovementDTO
	Replacement *dto.MovementDTO
}

type UpdateMovementUseCase interface {
	Execute(ctx context.Context, id string, input UpdateMovementInput) (UpdateMovementResult, error)
}

type updateMovementUseCase struct {
	repo     repositories.MovementRepository
	accounts repositories.AccountRepository
	sync     services.SyncTrigger
}

// NewUpdateMovement returns interface type for dependency injection.
func NewUpdateMovement(repo repositories.MovementRepository, accounts repositories.AccountRepository, sync services.SyncTrigger) UpdateMovementUseCase {
	return &updateMovementUseCase{repo: repo, accounts: accounts, sync: sync}
}

func (uc *updateMovementUseCase) Execute(ctx context.Context, id string, input UpdateMovementInput) (UpdateMovementResult, error) {
	if id == "" {
		return UpdateMovementResult{}, apperrors.ErrInvalidInput
	}

	movementDTO, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return UpdateMovementResult{}, err
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

	editsFinancial := input.Amount != nil || input.Currency != nil || input.Timestamp != nil
	editsMetadata := input.Description != nil || input.Category != nil || input.PaymentMethod != nil || input.AccountID != nil

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
	categoryInput := orDefault(input.Category, string(movement.Category))
	paymentMethodInput := orDefault(input.PaymentMethod, string(movement.PaymentMethod))
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

	category, paymentMethod, err := normalizeCategoryAndMethod(categoryInput, paymentMethodInput)
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
			if err := uc.repo.UpdateMetadata(ctx, movement.ID, description, string(category), string(paymentMethod), accountID); err != nil {
				return UpdateMovementResult{}, err
			}
			movementDTO.Description, movementDTO.Category, movementDTO.PaymentMethod, movementDTO.AccountID =
				description, string(category), string(paymentMethod), accountID
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
			if err := uc.repo.UpdateMetadata(ctx, movement.ID, description, string(category), string(paymentMethod), accountID); err != nil {
				if rollbackErr := uc.repo.UpdateFinancial(ctx, movement.ID, originalAmount, originalCurrency, originalTimestamp); rollbackErr != nil {
					return UpdateMovementResult{}, fmt.Errorf(
						"metadata update failed after financial update and rollback also failed: metadata: %w; rollback: %v",
						err, rollbackErr)
				}
				return UpdateMovementResult{}, err
			}
			movementDTO.Description, movementDTO.Category, movementDTO.PaymentMethod, movementDTO.AccountID =
				description, string(category), string(paymentMethod), accountID
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
			UserID:        movement.UserID,
			Amount:        amount,
			Currency:      currency,
			Description:   description,
			Category:      category,
			PaymentMethod: paymentMethod,
			AccountID:     accountID,
			Status:        entities.MovementStatusActive,
			SyncStatus:    entities.SyncStatusPending,
			Timestamp:     timestamp,
			CreatedAt:     now,
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
