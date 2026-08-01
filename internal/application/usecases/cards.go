package usecases

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

// validateCardLimits rejects a negative credit_limit/monthly_budget —
// shared by create and update.
func validateCardLimits(creditLimit, monthlyBudget *int64) error {
	if creditLimit != nil && *creditLimit < 0 {
		return fmt.Errorf("%w: credit_limit must not be negative", apperrors.ErrInvalidInput)
	}
	if monthlyBudget != nil && *monthlyBudget < 0 {
		return fmt.Errorf("%w: monthly_budget must not be negative", apperrors.ErrInvalidInput)
	}
	return nil
}

type createCardUseCase struct {
	cards repositories.CardRepository
}

// NewCreateCard returns interface type for dependency injection.
func NewCreateCard(cards repositories.CardRepository) CreateCardUseCase {
	return &createCardUseCase{cards: cards}
}

func (uc *createCardUseCase) Execute(ctx context.Context, input CreateCardInput) (*dto.CardDTO, error) {
	name := strings.TrimSpace(input.Name)
	if input.UserID == "" || name == "" || input.Currency == "" {
		return nil, fmt.Errorf("%w: name and currency are required", apperrors.ErrInvalidInput)
	}
	if !entities.ValidCardDay(input.ClosingDay) || !entities.ValidCardDay(input.DueDay) {
		return nil, fmt.Errorf(`%w: closing_day/due_day must be "1"-"28" or "last"`, apperrors.ErrInvalidInput)
	}
	if err := validateCardLimits(input.CreditLimit, input.MonthlyBudget); err != nil {
		return nil, err
	}

	return uc.cards.Create(ctx, &dto.CardDTO{
		UserID:        input.UserID,
		Name:          name,
		LastFour:      input.LastFour,
		ClosingDay:    input.ClosingDay,
		DueDay:        input.DueDay,
		CreditLimit:   input.CreditLimit,
		MonthlyBudget: input.MonthlyBudget,
		Currency:      strings.ToLower(strings.TrimSpace(input.Currency)),
		CreatedAt:     time.Now().UTC(),
	})
}

type listCardsUseCase struct {
	cards     repositories.CardRepository
	movements repositories.MovementRepository
}

// NewListCards returns interface type for dependency injection.
func NewListCards(cards repositories.CardRepository, movements repositories.MovementRepository) ListCardsUseCase {
	return &listCardsUseCase{cards: cards, movements: movements}
}

func (uc *listCardsUseCase) Execute(ctx context.Context, userID string) ([]CardView, error) {
	if userID == "" {
		return nil, apperrors.ErrInvalidInput
	}
	cards, err := uc.cards.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	views := make([]CardView, 0, len(cards))
	for _, c := range cards {
		view, err := buildCardView(ctx, uc.movements, c)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

type getCardUseCase struct {
	cards     repositories.CardRepository
	movements repositories.MovementRepository
}

// NewGetCard returns interface type for dependency injection.
func NewGetCard(cards repositories.CardRepository, movements repositories.MovementRepository) GetCardUseCase {
	return &getCardUseCase{cards: cards, movements: movements}
}

func (uc *getCardUseCase) Execute(ctx context.Context, userID, id string) (CardView, error) {
	if userID == "" || id == "" {
		return CardView{}, apperrors.ErrInvalidInput
	}
	card, err := uc.cards.GetByID(ctx, userID, id)
	if err != nil {
		return CardView{}, err
	}
	return buildCardView(ctx, uc.movements, card)
}

// buildCardView derives the amount-due/limit/budget picture for one
// card; shared by ListCards and GetCard so both return the same shape.
//
// A known v1 simplification: payments aren't scoped to a specific
// statement (there's no separate statement/invoice entity), so every
// payment ever recorded against the card nets against whatever the
// current NextDueTotal is, rather than being tied to the exact
// statement it was intended for. This matches the ticket's own
// tolerance for simplicity elsewhere (e.g. no overpayment enforcement)
// — revisit only if it turns out to matter in practice.
func buildCardView(ctx context.Context, movements repositories.MovementRepository, card *dto.CardDTO) (CardView, error) {
	view := CardView{Card: card}

	now := time.Now().UTC()
	nextDueDate := entities.CurrentStatementDueDate(card.ClosingDay, card.DueDay, now)
	view.NextDueDate = nextDueDate

	charges, err := movements.ListByCard(ctx, card.ID)
	if err != nil {
		return CardView{}, err
	}
	for _, m := range charges {
		if m.Status == string(entities.MovementStatusVoided) {
			continue
		}
		switch {
		case m.Timestamp.Equal(nextDueDate):
			view.NextDueTotal += -m.Amount // charges are negative; totals are owed-positive
		case m.Timestamp.After(nextDueDate):
			view.OpenCycleTotal += -m.Amount
			// Timestamps before nextDueDate belong to older, already-past
			// statements this v1 doesn't track separately — see the doc
			// comment above.
		}
	}

	payments, err := movements.ListCardPayments(ctx, card.ID)
	if err != nil {
		return CardView{}, err
	}
	for _, m := range payments {
		if m.Status == string(entities.MovementStatusVoided) {
			continue
		}
		// A payment is a negative expense movement on the paying
		// account; its magnitude is what it reduces NextDueTotal by.
		view.NextDueTotal += m.Amount
	}

	if card.CreditLimit != nil {
		available := *card.CreditLimit - view.NextDueTotal - view.OpenCycleTotal
		view.AvailableCredit = &available
	}
	if card.MonthlyBudget != nil {
		remaining := *card.MonthlyBudget - view.OpenCycleTotal
		view.BudgetRemaining = &remaining
		view.OverBudget = remaining < 0
	}

	return view, nil
}

type updateCardUseCase struct {
	cards repositories.CardRepository
}

// NewUpdateCard returns interface type for dependency injection.
func NewUpdateCard(cards repositories.CardRepository) UpdateCardUseCase {
	return &updateCardUseCase{cards: cards}
}

func (uc *updateCardUseCase) Execute(ctx context.Context, userID, id string, input UpdateCardInput) (*dto.CardDTO, error) {
	if userID == "" || id == "" {
		return nil, apperrors.ErrInvalidInput
	}
	existing, err := uc.cards.GetByID(ctx, userID, id)
	if err != nil {
		return nil, err
	}

	name := existing.Name
	if input.Name != nil {
		name = strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, fmt.Errorf("%w: name is required", apperrors.ErrInvalidInput)
		}
	}
	lastFour := existing.LastFour
	if input.LastFour != nil {
		lastFour = *input.LastFour
	}
	closingDay := existing.ClosingDay
	if input.ClosingDay != nil {
		if !entities.ValidCardDay(*input.ClosingDay) {
			return nil, fmt.Errorf(`%w: closing_day must be "1"-"28" or "last"`, apperrors.ErrInvalidInput)
		}
		closingDay = *input.ClosingDay
	}
	dueDay := existing.DueDay
	if input.DueDay != nil {
		if !entities.ValidCardDay(*input.DueDay) {
			return nil, fmt.Errorf(`%w: due_day must be "1"-"28" or "last"`, apperrors.ErrInvalidInput)
		}
		dueDay = *input.DueDay
	}
	creditLimit := existing.CreditLimit
	if input.CreditLimit != nil {
		creditLimit = input.CreditLimit
	}
	monthlyBudget := existing.MonthlyBudget
	if input.MonthlyBudget != nil {
		monthlyBudget = input.MonthlyBudget
	}
	if err := validateCardLimits(creditLimit, monthlyBudget); err != nil {
		return nil, err
	}

	updated := &dto.CardDTO{
		Name:          name,
		LastFour:      lastFour,
		ClosingDay:    closingDay,
		DueDay:        dueDay,
		CreditLimit:   creditLimit,
		MonthlyBudget: monthlyBudget,
	}
	if err := uc.cards.Update(ctx, userID, id, updated); err != nil {
		return nil, err
	}
	return uc.cards.GetByID(ctx, userID, id)
}

type deleteCardUseCase struct {
	cards repositories.CardRepository
}

// NewDeleteCard returns interface type for dependency injection.
func NewDeleteCard(cards repositories.CardRepository) DeleteCardUseCase {
	return &deleteCardUseCase{cards: cards}
}

func (uc *deleteCardUseCase) Execute(ctx context.Context, userID, id string) error {
	if userID == "" || id == "" {
		return apperrors.ErrInvalidInput
	}
	// Ownership check first — GetByID returns ErrNotFound for another
	// user's card, same as everywhere else, before IsReferenced would
	// otherwise leak whether the id exists at all.
	if _, err := uc.cards.GetByID(ctx, userID, id); err != nil {
		return err
	}
	referenced, err := uc.cards.IsReferenced(ctx, id)
	if err != nil {
		return err
	}
	if referenced {
		return fmt.Errorf("%w: card is still referenced by a movement or purchase", apperrors.ErrConflict)
	}
	return uc.cards.Delete(ctx, userID, id)
}
