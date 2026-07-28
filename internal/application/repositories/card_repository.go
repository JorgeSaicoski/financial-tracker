package repositories

import (
	"context"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
)

// CardRepository persists credit card profiles (BACK-08), expressed in
// application/dto types — infrastructure adapts its rows to these at the
// boundary.
type CardRepository interface {
	// Create inserts a card, generating its ID, and returns the stored
	// row.
	Create(ctx context.Context, card *dto.CardDTO) (*dto.CardDTO, error)
	GetByID(ctx context.Context, userID, id string) (*dto.CardDTO, error)
	ListByUser(ctx context.Context, userID string) ([]*dto.CardDTO, error)
	// Update overwrites name/last_four/closing_day/due_day/credit_limit/
	// monthly_budget in place — the fields a PATCH can change.
	Update(ctx context.Context, userID, id string, card *dto.CardDTO) error
	Delete(ctx context.Context, userID, id string) error
	// IsReferenced reports whether any movement or credit-card purchase
	// still points at this card (card_id or card_payment_for_card_id) —
	// used to reject deleting a card still in use, same "can't delete
	// what's referenced" rule as accounts.
	IsReferenced(ctx context.Context, id string) (bool, error)
}
