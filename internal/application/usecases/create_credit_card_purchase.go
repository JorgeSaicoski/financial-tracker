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

type createCreditCardPurchaseUseCase struct {
	purchases repositories.CreditCardPurchaseRepository
	cards     repositories.CardRepository
	settings  repositories.UserSettingsRepository
}

// NewCreateCreditCardPurchase returns interface type for dependency injection.
func NewCreateCreditCardPurchase(purchases repositories.CreditCardPurchaseRepository, cards repositories.CardRepository, settings repositories.UserSettingsRepository) CreateCreditCardPurchaseUseCase {
	return &createCreditCardPurchaseUseCase{purchases: purchases, cards: cards, settings: settings}
}

func (uc *createCreditCardPurchaseUseCase) Execute(ctx context.Context, input CreateCreditCardPurchaseInput) (*dto.CreditCardPurchaseDTO, []*dto.MovementDTO, error) {
	if input.UserID == "" || input.Currency == "" || input.TotalAmount == 0 {
		return nil, nil, apperrors.ErrInvalidInput
	}
	if input.Installments < 2 {
		return nil, nil, apperrors.ErrInvalidInput
	}

	category, _, err := normalizeCategoryAndMethod(input.Category, string(entities.PaymentMethodCreditCard))
	if err != nil {
		return nil, nil, err
	}

	// Integer division truncates toward zero, so this works for both
	// signs: the remainder lands on the last installment and the parts
	// always sum exactly to the total.
	base := input.TotalAmount / int64(input.Installments)
	remainder := input.TotalAmount - base*int64(input.Installments)
	if base == 0 {
		// A total too small to split leaves zero-amount installments,
		// which ledger-service rejects — they could never sync.
		return nil, nil, apperrors.ErrInvalidInput
	}

	now := time.Now().UTC()

	// BACK-08: with a card, each installment is dated on the card's real
	// due days instead of a flat monthly offset from the purchase date.
	// Without one, today's flat-offset behavior is unchanged.
	dueDates := make([]time.Time, input.Installments)
	if input.CardID != nil {
		card, err := uc.cards.GetByID(ctx, input.UserID, *input.CardID)
		if apperrors.Is(err, apperrors.ErrNotFound) {
			return nil, nil, fmt.Errorf("%w: card not found", apperrors.ErrInvalidInput)
		}
		if err != nil {
			return nil, nil, err
		}
		if card.Currency != input.Currency {
			return nil, nil, fmt.Errorf("%w: purchase currency %q does not match card currency %q",
				apperrors.ErrInvalidInput, input.Currency, card.Currency)
		}
		dueDates[0] = entities.NextCardDueDate(card.ClosingDay, card.DueDay, now)
		for i := 1; i < len(dueDates); i++ {
			dueDates[i] = entities.AddCardCycle(card.DueDay, dueDates[i-1])
		}
	} else {
		for i := range dueDates {
			dueDates[i] = now.AddDate(0, i, 0)
		}
	}

	syncStatus, err := effectiveSyncStatus(ctx, uc.settings, input.UserID)
	if err != nil {
		return nil, nil, err
	}

	purchase := &entities.CreditCardPurchase{
		UserID:           input.UserID,
		Description:      input.Description,
		Category:         category,
		TotalAmount:      input.TotalAmount,
		Currency:         input.Currency,
		InstallmentCount: input.Installments,
		PurchaseDate:     now,
		Status:           entities.CreditCardPurchaseStatusActive,
		CreatedAt:        now,
		CardID:           input.CardID,
	}

	installments := make([]*entities.Movement, input.Installments)
	for i := range installments {
		amount := base
		if i == input.Installments-1 {
			amount += remainder
		}
		number := i + 1
		installments[i] = &entities.Movement{
			UserID:            input.UserID,
			Amount:            amount,
			Currency:          input.Currency,
			Description:       input.Description,
			Category:          category,
			PaymentMethod:     entities.PaymentMethodCreditCard,
			InstallmentNumber: &number,
			CardID:            input.CardID,
			Status:            entities.MovementStatusActive,
			SyncStatus:        syncStatus,
			// One installment per due date. Future ones are invisible to
			// the sync worker until their date arrives.
			Timestamp: dueDates[i],
			CreatedAt: now,
		}
	}

	return uc.purchases.CreateWithInstallments(ctx,
		dto.CreditCardPurchaseFromEntity(purchase),
		dto.MovementsFromEntities(installments))
}
