package usecases

import (
	"context"
	"testing"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
)

func TestGetCashflowExcludesTransfers(t *testing.T) {
	movements := newFakeMovementRepo()
	accounts := newFakeAccountRepo()
	from := mustCreateAccount(t, accounts, "u1", "usd")
	to := mustCreateAccount(t, accounts, "u1", "usd")

	now := time.Now().UTC()
	income := activeMovement("income", 1000, entities.SyncStatusPending)
	income.Category = entities.CategoryIncome
	income.Timestamp = now
	movements.add(income)

	if _, err := NewTransferBetweenAccounts(movements, accounts).Execute(context.Background(), TransferBetweenAccountsInput{
		UserID: "u1", FromAccountID: from.ID, ToAccountID: to.ID, Amount: 300, Timestamp: now,
	}); err != nil {
		t.Fatal(err)
	}

	uc := NewGetCashflow(movements, accounts)
	result, err := uc.Execute(context.Background(), "u1", now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Totals) != 1 {
		t.Fatalf("totals = %+v, want exactly the income currency (transfer excluded)", result.Totals)
	}
	total := result.Totals[0]
	if total.In != 1000 || total.Out != 0 || total.Net != 1000 {
		t.Errorf("totals = %+v, transfer legs must not inflate in/out", total)
	}
}
