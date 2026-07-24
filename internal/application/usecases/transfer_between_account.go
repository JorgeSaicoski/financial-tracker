package usecases

import (
	"context"
	"fmt"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/id"
)

// TransferBetweenAccountsInput describes a move of money between two of
// the user's own accounts. Amount is always positive: the debit leg gets
// -Amount, the credit leg +Amount. A zero Timestamp means "now". v1 is
// same-currency only.
type TransferBetweenAccountsInput struct {
	UserID        string
	FromAccountID string
	ToAccountID   string
	Amount        int64
	Description   string
	Timestamp     time.Time
}

// TransferResult carries both legs of a transfer, linked by TransferID:
// Debit is the negative leg on FromAccountID, Credit the positive leg on
// ToAccountID. Together they net to zero, so the transfer never changes
// net worth.
type TransferResult struct {
	TransferID string
	Debit      *dto.MovementDTO
	Credit     *dto.MovementDTO
}

type TransferBetweenAccountsUseCase interface {
	Execute(ctx context.Context, input TransferBetweenAccountsInput) (TransferResult, error)
}

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

	fromDTO, err := uc.ownedAccount(ctx, input.FromAccountID, input.UserID)
	if err != nil {
		return TransferResult{}, err
	}
	toDTO, err := uc.ownedAccount(ctx, input.ToAccountID, input.UserID)
	if err != nil {
		return TransferResult{}, err
	}

	timestamp := input.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}

	// The single-account rules — same currency (v1 doesn't know a
	// conversion rate; cross-currency needs BACK-11's exchange rates),
	// positive amount, not the same account — live on the Account entity:
	// each account produces the movement its side of the transfer creates.
	from, to := fromDTO.ToEntity(), toDTO.ToEntity()
	debit, err := from.Send(to, input.Amount, input.Description, timestamp)
	if err != nil {
		return TransferResult{}, fmt.Errorf("%w: %v", apperrors.ErrInvalidInput, err)
	}
	credit, err := to.Receive(from, input.Amount, input.Description, timestamp)
	if err != nil {
		return TransferResult{}, fmt.Errorf("%w: %v", apperrors.ErrInvalidInput, err)
	}

	// Linking the pair is cross-entity orchestration — the usecase's job,
	// not either account's.
	transferID := id.NewUUID()
	debit.TransferID, credit.TransferID = &transferID, &transferID

	// Both legs land in one transaction: a transfer with only one leg
	// would silently create or destroy money.
	created, err := uc.movements.CreateBatch(ctx, []*dto.MovementDTO{
		dto.MovementFromEntity(debit),
		dto.MovementFromEntity(credit),
	})
	if err != nil {
		return TransferResult{}, err
	}

	return TransferResult{TransferID: transferID, Debit: created[0], Credit: created[1]}, nil
}

func (uc *transferBetweenAccountsUseCase) ownedAccount(ctx context.Context, accountID, userID string) (*dto.AccountDTO, error) {
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
