package repositories

import (
	"context"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
)

// PlanRepository is the contract for BACK-10's plans: a user's
// monthly-figure goal, either a pure simulation (stress_test, no
// movements ever) or funded by real movements toward a real target
// (savings). Progress itself isn't stored here — it's computed on read
// from MovementRepository.SumByPlan and, for stress-test plans, from
// cashflow — this contract only persists the plan's own fields.
type PlanRepository interface {
	// Create inserts a plan, generating its ID, and returns the stored
	// row.
	Create(ctx context.Context, p *dto.PlanDTO) (*dto.PlanDTO, error)
	// GetByID returns a plan owned by userID. apperrors.ErrNotFound if it
	// doesn't exist or isn't owned by userID — same "don't distinguish
	// doesn't-exist from isn't-yours" rule as GetMovementUseCase.
	GetByID(ctx context.Context, userID, id string) (*dto.PlanDTO, error)
	// ListByUser returns every plan row for the user.
	ListByUser(ctx context.Context, userID string) ([]*dto.PlanDTO, error)
	// Update overwrites every editable field for a plan owned by userID
	// with exactly the values given (the usecase layer resolves a
	// partial PATCH into the full merged values before calling this,
	// same always-overwrite convention as MovementRepository.UpdateMetadata).
	// apperrors.ErrNotFound if it doesn't exist or isn't owned by userID.
	Update(ctx context.Context, userID, id string, name string, targetAmount *int64, monthlyTargetAmount int64, endDate *time.Time, status string) error
}
