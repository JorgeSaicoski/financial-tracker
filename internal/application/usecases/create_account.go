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

// CreateAccountInput carries the caller-supplied fields for a new
// account. Type defaults to "other" when empty and is validated against
// the domain's fixed list in the usecase; Currency must already be
// registered (POST /currencies first for a new one).
type CreateAccountInput struct {
	UserID   string
	Name     string
	Type     string
	Currency string
}

type CreateAccountUseCase interface {
	Execute(ctx context.Context, input CreateAccountInput) (*dto.AccountDTO, error)
}

type createAccountUseCase struct {
	accounts   repositories.AccountRepository
	currencies repositories.CurrencyRepository
}

// NewCreateAccount returns interface type for dependency injection.
func NewCreateAccount(accounts repositories.AccountRepository, currencies repositories.CurrencyRepository) CreateAccountUseCase {
	return &createAccountUseCase{accounts: accounts, currencies: currencies}
}

func (uc *createAccountUseCase) Execute(ctx context.Context, input CreateAccountInput) (*dto.AccountDTO, error) {
	name := strings.TrimSpace(input.Name)
	if input.UserID == "" || name == "" {
		return nil, fmt.Errorf("%w: account name is required", apperrors.ErrInvalidInput)
	}

	accountType := entities.AccountType(input.Type)
	if accountType == "" {
		accountType = entities.AccountTypeOther
	}
	if !accountType.IsValid() {
		return nil, fmt.Errorf("%w: unknown account type %q", apperrors.ErrInvalidInput, input.Type)
	}

	currency := strings.ToLower(strings.TrimSpace(input.Currency))
	registered, err := uc.currencies.List(ctx)
	if err != nil {
		return nil, err
	}
	if !contains(registered, currency) {
		return nil, fmt.Errorf("%w: unknown currency %q (add it via POST /currencies first)", apperrors.ErrInvalidInput, input.Currency)
	}

	existing, err := uc.accounts.ListByUser(ctx, input.UserID)
	if err != nil {
		return nil, err
	}
	for _, a := range existing {
		if strings.EqualFold(a.Name, name) {
			return nil, fmt.Errorf("%w: account %q already exists", apperrors.ErrConflict, name)
		}
	}

	return uc.accounts.Create(ctx, dto.AccountFromEntity(&entities.Account{
		UserID:    input.UserID,
		Name:      name,
		Type:      accountType,
		Currency:  currency,
		CreatedAt: time.Now().UTC(),
	}))
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
