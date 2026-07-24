package usecases

import (
	"context"
	"fmt"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/services"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

// CancelMovementResult reports how the cancel was carried out: a
// never-synced movement is voided in place (Reversal is nil); a synced one
// stays active and gains a compensating Reversal, mirroring
// ledger-service's no-delete rule.
type CancelMovementResult struct {
	Movement *dto.MovementDTO
	Reversal *dto.MovementDTO
}

type CancelMovementUseCase interface {
	Execute(ctx context.Context, id string) (CancelMovementResult, error)
}

type cancelMovementUseCase struct {
	repo repositories.MovementRepository
	sync services.SyncTrigger
}

// NewCancelMovement returns interface type for dependency injection.
func NewCancelMovement(repo repositories.MovementRepository, sync services.SyncTrigger) CancelMovementUseCase {
	return &cancelMovementUseCase{repo: repo, sync: sync}
}

func (uc *cancelMovementUseCase) Execute(ctx context.Context, id string) (CancelMovementResult, error) {
	if id == "" {
		return CancelMovementResult{}, apperrors.ErrInvalidInput
	}

	movement, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return CancelMovementResult{}, err
	}
	if movement.TransferID != nil {
		// Cancelling one leg alone would leave the other stranded,
		// breaking the transfer's zero-net-worth invariant.
		return CancelMovementResult{}, fmt.Errorf(
			"%w: this movement is one leg of a transfer — cancel it via POST /transfers/{id}/cancel",
			apperrors.ErrConflict)
	}

	result, err := cancelOne(ctx, uc.repo, movement)
	if err != nil {
		return CancelMovementResult{}, err
	}
	if result.Reversal != nil {
		uc.sync.TriggerAsync()
	}
	return result, nil
}

// cancelOne applies the cancel semantics to one movement; shared with the
// purchase-level and transfer-level cancels. It receives and returns
// application DTOs (the contract currency), converting to the domain
// entity internally to run its business-rule checks.
func cancelOne(ctx context.Context, repo repositories.MovementRepository, movementDTO *dto.MovementDTO) (CancelMovementResult, error) {
	movement := movementDTO.ToEntity()
	if movement.IsReversal() {
		// Cancelling a reversal would spawn reversal-of-reversal chains;
		// re-create the original movement instead if the cancel was a
		// mistake.
		return CancelMovementResult{}, apperrors.ErrInvalidInput
	}
	if movement.IsCancelled() {
		return CancelMovementResult{}, apperrors.ErrConflict
	}

	if !movement.IsSynced() {
		// Never reached ledger-service: void locally, nothing to reverse.
		// The sync worker only picks up active rows, so it will never
		// push this one.
		if err := repo.Void(ctx, movement.ID); err != nil {
			return CancelMovementResult{}, err
		}
		movementDTO.Status = string(entities.MovementStatusVoided)
		return CancelMovementResult{Movement: movementDTO}, nil
	}

	// Already in ledger-service, which never deletes: compensate with a
	// reversal. Note it deliberately doesn't inherit the purchase link —
	// reversals are not installments.
	now := time.Now().UTC()
	originalID := movement.ID
	reversal := &entities.Movement{
		UserID:            movement.UserID,
		Amount:            -movement.Amount,
		Currency:          movement.Currency,
		Description:       fmt.Sprintf("Reversal of %s", originalID),
		Category:          movement.Category,
		PaymentMethod:     movement.PaymentMethod,
		AccountID:         movement.AccountID, // nets the original out of its account too
		Status:            entities.MovementStatusActive,
		SyncStatus:        entities.SyncStatusPending,
		CancelsMovementID: &originalID,
		Timestamp:         now,
		CreatedAt:         now,
	}

	reversalDTO, err := repo.CreateReversal(ctx, dto.MovementFromEntity(reversal))
	if err != nil {
		return CancelMovementResult{}, err
	}
	movementDTO.ReversedByMovementID = &reversalDTO.ID
	return CancelMovementResult{Movement: movementDTO, Reversal: reversalDTO}, nil
}
