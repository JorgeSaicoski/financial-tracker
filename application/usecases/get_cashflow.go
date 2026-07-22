package usecases

import (
	"context"
	"sort"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
)

type getCashflowUseCase struct {
	movements repositories.MovementRepository
	accounts  repositories.AccountRepository
}

// NewGetCashflow returns interface type for dependency injection.
func NewGetCashflow(movements repositories.MovementRepository, accounts repositories.AccountRepository) GetCashflowUseCase {
	return &getCashflowUseCase{movements: movements, accounts: accounts}
}

func (uc *getCashflowUseCase) Execute(ctx context.Context, userID string, from, to time.Time) (CashflowResult, error) {
	if userID == "" || from.IsZero() || to.IsZero() || !from.Before(to) {
		return CashflowResult{}, apperrors.ErrInvalidInput
	}

	movements, err := uc.movements.ListByUser(ctx, userID, nil, &from, &to, 0, 0)
	if err != nil {
		return CashflowResult{}, err
	}
	accounts, err := uc.accounts.ListByUser(ctx, userID)
	if err != nil {
		return CashflowResult{}, err
	}
	accountByID := make(map[string]*entities.Account, len(accounts))
	for _, a := range accounts {
		accountByID[a.ID] = a
	}

	totals := make(map[string]*CurrencyFlow)
	byAccount := make(map[string]*AccountFlow)
	for _, m := range movements {
		// Voided movements never happened. A reversal and its original
		// stay active with opposite signs: they show up on both sides of
		// In/Out but cancel in Net, mirroring the ledger's records.
		if m.Status == entities.MovementStatusVoided {
			continue
		}

		total, ok := totals[m.Currency]
		if !ok {
			total = &CurrencyFlow{Currency: m.Currency}
			totals[m.Currency] = total
		}
		addFlow(&total.In, &total.Out, &total.Net, m.Amount)

		accountID := ""
		if m.AccountID != nil {
			accountID = *m.AccountID
		}
		// Unassigned movements can mix currencies, so buckets are keyed
		// by account AND currency (an account itself is single-currency).
		key := accountID + "|" + m.Currency
		flow, ok := byAccount[key]
		if !ok {
			flow = &AccountFlow{AccountID: accountID, Currency: m.Currency}
			if account := accountByID[accountID]; account != nil {
				flow.Name = account.Name
			}
			byAccount[key] = flow
		}
		addFlow(&flow.In, &flow.Out, &flow.Net, m.Amount)
	}

	result := CashflowResult{From: from, To: to}
	for _, t := range totals {
		result.Totals = append(result.Totals, *t)
	}
	sort.Slice(result.Totals, func(i, j int) bool {
		return result.Totals[i].Currency < result.Totals[j].Currency
	})
	for _, f := range byAccount {
		result.ByAccount = append(result.ByAccount, *f)
	}
	// Named accounts alphabetically, unassigned movements last.
	sort.Slice(result.ByAccount, func(i, j int) bool {
		a, b := result.ByAccount[i], result.ByAccount[j]
		if (a.AccountID == "") != (b.AccountID == "") {
			return a.AccountID != ""
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Currency < b.Currency
	})
	return result, nil
}

func addFlow(in, out, net *int64, amount int64) {
	if amount > 0 {
		*in += amount
	} else {
		*out += -amount
	}
	*net += amount
}
