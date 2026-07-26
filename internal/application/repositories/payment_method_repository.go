package repositories

import (
	"context"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
)

// PaymentMethodRepository is the per-user, extendable registry of
// payment-method names (BACK-17) — same shape as CurrencyRepository, but
// scoped per user, replacing the old fixed enum
// (domain/entities/payment_method.go). "credit_card" and "bank_transfer"
// are system entries: code branches on them directly (installment
// validation, transfer legs), so they can't be renamed or deleted —
// enforced by the usecase layer, not here (this contract has no notion
// of "system", same reasoning MovementRepository has no notion of
// "reversal cannot be edited").
type PaymentMethodRepository interface {
	// EnsureByName returns the user's existing payment method matching
	// name (case-insensitive), or creates one if none exists yet —
	// idempotent, same "adding an existing one is a no-op" shape as
	// CurrencyRepository.Add, but returning the row since callers
	// (implicit registration on movement creation) need its id.
	EnsureByName(ctx context.Context, userID, name string) (*dto.PaymentMethodDTO, error)
	// GetByID returns a payment method owned by userID. apperrors.ErrNotFound
	// if it doesn't exist or isn't owned by userID — same "don't
	// distinguish doesn't-exist from isn't-yours" rule as GetMovementUseCase.
	GetByID(ctx context.Context, userID, id string) (*dto.PaymentMethodDTO, error)
	// ListByUser returns every payment-method row for the user, name
	// ascending.
	ListByUser(ctx context.Context, userID string) ([]*dto.PaymentMethodDTO, error)
	// Create inserts a brand-new payment-method row, generating its ID.
	// Callers (the usecase layer) reject duplicate names and reserved
	// system names first — this method does not check either.
	Create(ctx context.Context, m *dto.PaymentMethodDTO) (*dto.PaymentMethodDTO, error)
	// Update renames a payment method owned by userID. apperrors.ErrNotFound
	// if it doesn't exist or isn't owned by userID.
	Update(ctx context.Context, userID, id, name string) error
	// Delete removes a payment method owned by userID. apperrors.ErrNotFound
	// if it doesn't exist or isn't owned by userID. Deleting one still
	// referenced by movements is allowed — it's a label, not an FK.
	Delete(ctx context.Context, userID, id string) error
}
