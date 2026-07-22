package usecases

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/JorgeSaicoski/financial-tracker/application/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
)

// currencyCodePattern keeps codes short lowercase identifiers (usd, brl,
// btc, usdt, ...) so they behave everywhere a currency string is used.
var currencyCodePattern = regexp.MustCompile(`^[a-z0-9]{2,10}$`)

type listCurrenciesUseCase struct {
	repo repositories.CurrencyRepository
}

// NewListCurrencies returns interface type for dependency injection.
func NewListCurrencies(repo repositories.CurrencyRepository) ListCurrenciesUseCase {
	return &listCurrenciesUseCase{repo: repo}
}

func (uc *listCurrenciesUseCase) Execute(ctx context.Context) ([]string, error) {
	return uc.repo.List(ctx)
}

type addCurrencyUseCase struct {
	repo repositories.CurrencyRepository
}

// NewAddCurrency returns interface type for dependency injection.
func NewAddCurrency(repo repositories.CurrencyRepository) AddCurrencyUseCase {
	return &addCurrencyUseCase{repo: repo}
}

func (uc *addCurrencyUseCase) Execute(ctx context.Context, code string) (string, error) {
	code = strings.ToLower(strings.TrimSpace(code))
	if !currencyCodePattern.MatchString(code) {
		return "", fmt.Errorf("%w: currency code must be 2-10 lowercase letters or digits", apperrors.ErrInvalidInput)
	}
	if err := uc.repo.Add(ctx, code); err != nil {
		return "", err
	}
	return code, nil
}
