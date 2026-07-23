package usecases

import (
	"context"
	"fmt"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
	"github.com/JorgeSaicoski/financial-tracker/pkg/id"
)

type transferBetweenAccountsUseCase struct {
	movements repositories.MovementRepository
	accounts  repositories.AccountRepository
}

// NewTransferBetweenAccounts returns interface type for dependency injection.
func NewTransferBetweenAccounts(movements repositories.MovementRepository, accounts repositories.AccountRepository) TransferBetweenAccountsUseCase {
	return &transferBetweenAccountsUseCase{movements: movements, accounts: accounts}
}

func (uc *transferBetweenAccountsUseCase) Execute(ctx context.Context, input TransferBetweenAccountsInput) (TransferResult, error) {
	if input.UserID == "" || input.FromAccountID == "" || input.ToAccountID == "" || input.Amount <= 0 {
		return TransferResult{}, apperrors.ErrInvalidInput
	}
	if input.FromAccountID == input.ToAccountID {
		return TransferResult{}, fmt.Errorf("%w: source and destination accounts must differ", apperrors.ErrInvalidInput)
	}

	from, err := uc.ownedAccount(ctx, input.FromAccountID, input.UserID)
	if err != nil {
		return TransferResult{}, err
	}
	to, err := uc.ownedAccount(ctx, input.ToAccountID, input.UserID)
	if err != nil {
		return TransferResult{}, err
	}
	if from.Currency != to.Currency {
		// v1 doesn't know a conversion rate between the two; cross-currency
		// transfers are a later ticket (needs BACK-11's exchange rates).
		return TransferResult{}, fmt.Errorf("%w: cross-currency transfers aren't supported yet (%q vs %q)",
			apperrors.ErrInvalidInput, from.Currency, to.Currency)
	}

	timestamp := input.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	now := time.Now().UTC()
	transferID := id.NewUUID()

	debit := &entities.Movement{
		UserID:        input.UserID,
		Amount:        -input.Amount,
		Currency:      from.Currency,
		Description:   input.Description,
		Category:      entities.CategoryTransfer,
		PaymentMethod: entities.PaymentMethodBankTransfer,
		AccountID:     &from.ID,
		TransferID:    &transferID,
		Status:        entities.MovementStatusActive,
		SyncStatus:    entities.SyncStatusPending,
		Timestamp:     timestamp,
		CreatedAt:     now,
	}
	credit := &entities.Movement{
		UserID:        input.UserID,
		Amount:        input.Amount,
		Currency:      to.Currency,
		Description:   input.Description,
		Category:      entities.CategoryTransfer,
		PaymentMethod: entities.PaymentMethodBankTransfer,
		AccountID:     &to.ID,
		TransferID:    &transferID,
		Status:        entities.MovementStatusActive,
		SyncStatus:    entities.SyncStatusPending,
		Timestamp:     timestamp,
		CreatedAt:     now,
	}

	// Both legs land in one transaction: a transfer with only one leg
	// would silently create or destroy money.
	created, err := uc.movements.CreateBatch(ctx, []*entities.Movement{debit, credit})
	if err != nil {
		return TransferResult{}, err
	}

	return TransferResult{TransferID: transferID, Debit: created[0], Credit: created[1]}, nil
}

func (uc *transferBetweenAccountsUseCase) ownedAccount(ctx context.Context, accountID, userID string) (*entities.Account, error) {
	account, err := uc.accounts.GetByID(ctx, accountID)
	if apperrors.Is(err, apperrors.ErrNotFound) {
		return nil, fmt.Errorf("%w: account not found", apperrors.ErrInvalidInput)
	}
	if err != nil {
		return nil, err
	}
	if account.UserID != userID {
		return nil, fmt.Errorf("%w: account not found", apperrors.ErrInvalidInput)
	}
	return account, nil
}
