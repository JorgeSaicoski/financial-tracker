package usecases

import (
	"context"
	"fmt"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
	"github.com/JorgeSaicoski/financial-tracker/domain/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
)

// SyncTrigger lets cancel usecases kick a best-effort background sync so a
// freshly created reversal reaches ledger-service promptly, without the
// cancel ever blocking on ledger-service being up. Implemented by
// application/sync.Service.
type SyncTrigger interface {
	TriggerAsync()
}

// CancelMovementResult reports how the cancel was carried out: a
// never-synced movement is voided in place (Reversal is nil); a synced one
// stays active and gains a compensating Reversal, mirroring
// ledger-service's no-delete rule.
type CancelMovementResult struct {
	Movement *entities.Movement
	Reversal *entities.Movement
}

type CancelMovementUseCase interface {
	Execute(ctx context.Context, id string) (CancelMovementResult, error)
}

type cancelMovementUseCase struct {
	repo repositories.MovementRepository
	sync SyncTrigger
}

// NewCancelMovement returns interface type for dependency injection.
func NewCancelMovement(repo repositories.MovementRepository, sync SyncTrigger) CancelMovementUseCase {
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
// purchase-level cancel.
func cancelOne(ctx context.Context, repo repositories.MovementRepository, movement *entities.Movement) (CancelMovementResult, error) {
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
		movement.Status = entities.MovementStatusVoided
		return CancelMovementResult{Movement: movement}, nil
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

	reversal, err := repo.CreateReversal(ctx, reversal)
	if err != nil {
		return CancelMovementResult{}, err
	}
	movement.ReversedByMovementID = &reversal.ID
	return CancelMovementResult{Movement: movement, Reversal: reversal}, nil
}
