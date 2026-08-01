package usecases

import (
	"context"
	"fmt"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

type createMovementUseCase struct {
	repo       repositories.MovementRepository
	accounts   repositories.AccountRepository
	cards      repositories.CardRepository
	methods    repositories.PaymentMethodRepository
	plans      repositories.PlanRepository
	categories repositories.CategoryRepository
	settings   repositories.UserSettingsRepository
}

// NewCreateMovement returns interface type for dependency injection.
func NewCreateMovement(repo repositories.MovementRepository, accounts repositories.AccountRepository, cards repositories.CardRepository, methods repositories.PaymentMethodRepository, plans repositories.PlanRepository, categories repositories.CategoryRepository, settings repositories.UserSettingsRepository) CreateMovementUseCase {
	return &createMovementUseCase{repo: repo, accounts: accounts, cards: cards, methods: methods, plans: plans, categories: categories, settings: settings}
}

func (uc *createMovementUseCase) Execute(ctx context.Context, input CreateMovementInput) (*dto.MovementDTO, error) {
	if input.UserID == "" || input.Currency == "" || input.Amount == 0 {
		return nil, apperrors.ErrInvalidInput
	}
	if err := validateAvoidabilityPercent(input.AvoidabilityOverridePercent); err != nil {
		return nil, err
	}

	paymentMethod, err := resolvePaymentMethod(ctx, uc.methods, input.UserID, input.PaymentMethod)
	if err != nil {
		return nil, err
	}
	categoryID, err := resolveCategoryID(ctx, uc.categories, input.UserID, input.CategoryID)
	if err != nil {
		return nil, err
	}

	var planIDInput string
	if input.PlanID != nil {
		planIDInput = *input.PlanID
	}
	planID, err := resolvePlanForMovement(ctx, uc.plans, input.UserID, planIDInput, input.Currency)
	if err != nil {
		return nil, err
	}

	// An account holds one currency; a movement in a different currency
	// would silently corrupt that account's tracked balance. Ownership is
	// checked here too (BACK-02) — without it, any authenticated user
	// could attach a movement to another user's account by guessing its
	// id, since currency-match alone doesn't prove ownership.
	if input.AccountID != nil {
		account, err := uc.accounts.GetByID(ctx, *input.AccountID)
		if apperrors.Is(err, apperrors.ErrNotFound) {
			return nil, fmt.Errorf("%w: account not found", apperrors.ErrInvalidInput)
		}
		if err != nil {
			return nil, err
		}
		if account.UserID != input.UserID {
			return nil, fmt.Errorf("%w: account not found", apperrors.ErrInvalidInput)
		}
		if account.Currency != input.Currency {
			return nil, fmt.Errorf("%w: movement currency %q does not match account currency %q",
				apperrors.ErrInvalidInput, input.Currency, account.Currency)
		}
	}

	now := time.Now().UTC()
	timestamp := now

	// BACK-08: a charge on a card (payment_method=credit_card, card_id
	// set) is dated on the card's real due day instead of "now", so it
	// lands in the right bucket for next_due_total/open_cycle_total.
	if input.CardID != nil {
		if paymentMethod != entities.PaymentMethodCreditCard {
			return nil, fmt.Errorf("%w: card_id requires payment_method \"credit_card\"", apperrors.ErrInvalidInput)
		}
		card, err := uc.validateCard(ctx, input.UserID, input.Currency, *input.CardID)
		if err != nil {
			return nil, err
		}
		timestamp = entities.NextCardDueDate(card.ClosingDay, card.DueDay, now)
	}

	// BACK-08: a card payment (an ordinary expense settling a card's
	// statement) needs the same ownership/currency validation as a
	// charge, but keeps "now" as its timestamp — it's a real event that
	// happened today, not a future due date. It must also be shaped like
	// the transfer-style payment the doc comment on
	// Movement.CardPaymentForCardID promises: category=transfer, a
	// negative amount, and never combined with CardID — otherwise it
	// double-counts in cashflow and corrupts next_due_total math.
	if input.CardPaymentForCardID != nil {
		if input.CardID != nil {
			return nil, fmt.Errorf("%w: card_id and card_payment_for_card_id are mutually exclusive", apperrors.ErrInvalidInput)
		}
		if categoryID == nil || *categoryID != entities.CategoryTransferID {
			return nil, fmt.Errorf("%w: card_payment_for_card_id requires category \"transfer\"", apperrors.ErrInvalidInput)
		}
		if input.Amount >= 0 {
			return nil, fmt.Errorf("%w: card_payment_for_card_id requires a negative amount", apperrors.ErrInvalidInput)
		}
		if _, err := uc.validateCard(ctx, input.UserID, input.Currency, *input.CardPaymentForCardID); err != nil {
			return nil, err
		}
	}

	syncStatus, err := effectiveSyncStatus(ctx, uc.settings, input.UserID)
	if err != nil {
		return nil, err
	}

	movement := &entities.Movement{
		UserID:                      input.UserID,
		Amount:                      input.Amount,
		Currency:                    input.Currency,
		Description:                 input.Description,
		CategoryID:                  categoryID,
		PaymentMethod:               paymentMethod,
		AvoidabilityOverridePercent: input.AvoidabilityOverridePercent,
		AccountID:                   input.AccountID,
		CardID:                      input.CardID,
		CardPaymentForCardID:        input.CardPaymentForCardID,
		PlanID:                      planID,
		Status:                      entities.MovementStatusActive,
		SyncStatus:                  syncStatus,
		Timestamp:                   timestamp,
		CreatedAt:                   now,
	}

	return uc.repo.Create(ctx, dto.MovementFromEntity(movement))
}

// validateCard checks the card exists, belongs to userID, and shares
// currency with the movement — the ownership/currency guard both CardID
// (a charge) and CardPaymentForCardID (a payment) need.
func (uc *createMovementUseCase) validateCard(ctx context.Context, userID, currency, cardID string) (*dto.CardDTO, error) {
	card, err := uc.cards.GetByID(ctx, userID, cardID)
	if apperrors.Is(err, apperrors.ErrNotFound) {
		return nil, fmt.Errorf("%w: card not found", apperrors.ErrInvalidInput)
	}
	if err != nil {
		return nil, err
	}
	if card.Currency != currency {
		return nil, fmt.Errorf("%w: movement currency %q does not match card currency %q",
			apperrors.ErrInvalidInput, currency, card.Currency)
	}
	return card, nil
}
