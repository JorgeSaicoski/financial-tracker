package usecases

import (
	"context"
	"testing"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
)

func TestExportArchiveGathersOnlyThatUsersData(t *testing.T) {
	ctx := context.Background()
	accounts := newFakeAccountRepo()
	movements := newFakeMovementRepo()
	purchases := newFakePurchaseRepo(movements)

	if _, err := accounts.Create(ctx, &dto.AccountDTO{UserID: "user-1", Name: "Checking", Type: "bank", Currency: "usd"}); err != nil {
		t.Fatal(err)
	}
	if _, err := accounts.Create(ctx, &dto.AccountDTO{UserID: "user-2", Name: "Other user's account", Type: "bank", Currency: "usd"}); err != nil {
		t.Fatal(err)
	}
	m1 := activeMovement("", -500, entities.SyncStatusPending)
	m1.UserID = "user-1"
	if _, err := movements.Create(ctx, m1); err != nil {
		t.Fatal(err)
	}
	m2 := activeMovement("", -700, entities.SyncStatusPending)
	m2.UserID = "user-2"
	if _, err := movements.Create(ctx, m2); err != nil {
		t.Fatal(err)
	}
	if _, _, err := purchases.CreateWithInstallments(ctx, &dto.CreditCardPurchaseDTO{UserID: "user-1", InstallmentCount: 1}, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := purchases.CreateWithInstallments(ctx, &dto.CreditCardPurchaseDTO{UserID: "user-2", InstallmentCount: 1}, nil); err != nil {
		t.Fatal(err)
	}

	uc := NewExportArchive(accounts, movements, purchases)
	bundle, err := uc.Execute(ctx, "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(bundle.Accounts) != 1 || bundle.Accounts[0].UserID != "user-1" {
		t.Errorf("accounts = %+v, want exactly user-1's one account", bundle.Accounts)
	}
	if len(bundle.Movements) != 1 || bundle.Movements[0].UserID != "user-1" {
		t.Errorf("movements = %+v, want exactly user-1's one movement", bundle.Movements)
	}
	if len(bundle.CreditCardPurchases) != 1 || bundle.CreditCardPurchases[0].UserID != "user-1" {
		t.Errorf("purchases = %+v, want exactly user-1's one purchase", bundle.CreditCardPurchases)
	}
}
