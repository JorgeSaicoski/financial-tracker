package usecases

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
)

func newExportFixtures() (*exportMovementsUseCase, *fakeMovementRepo, *fakeAccountRepo) {
	movements := newFakeMovementRepo()
	accounts := newFakeAccountRepo()
	uc := &exportMovementsUseCase{movements: movements, accounts: accounts}
	return uc, movements, accounts
}

func TestExportExcludesCancelledByDefault(t *testing.T) {
	uc, movements, accounts := newExportFixtures()
	ctx := context.Background()

	account, err := accounts.Create(ctx, &dto.AccountDTO{UserID: "u1", Name: "Nubank", Type: "bank", Currency: "usd"})
	if err != nil {
		t.Fatal(err)
	}

	active := movements.add(&dto.MovementDTO{
		UserID: "u1", Amount: -4590, Currency: "usd", Description: "Groceries",
		Category: "food", PaymentMethod: "debit_card", AccountID: &account.ID,
		Status: string(entities.MovementStatusActive), Timestamp: time.Date(2025, 11, 3, 0, 0, 0, 0, time.UTC),
	})
	voided := movements.add(&dto.MovementDTO{
		UserID: "u1", Amount: 100, Currency: "usd", Category: "other", PaymentMethod: "cash",
		Status: string(entities.MovementStatusVoided), Timestamp: time.Now(),
	})
	originalID := "will-be-set"
	original := movements.add(&dto.MovementDTO{
		ID: originalID, UserID: "u1", Amount: 500, Currency: "usd", Category: "other", PaymentMethod: "cash",
		Status: string(entities.MovementStatusActive), Timestamp: time.Now(),
	})
	reversalID := original.ID + "-rev"
	original.ReversedByMovementID = &reversalID
	movements.byID[original.ID] = original
	reversal := movements.add(&dto.MovementDTO{
		ID: reversalID, UserID: "u1", Amount: -500, Currency: "usd", Category: "other", PaymentMethod: "cash",
		Status: string(entities.MovementStatusActive), CancelsMovementID: &original.ID, Timestamp: time.Now(),
	})

	rows, err := uc.Execute(ctx, "u1", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("default export returned %d rows, want 1 (only the untouched movement); got %+v", len(rows), rows)
	}
	if rows[0].Amount != active.Amount || rows[0].Account != "Nubank" {
		t.Errorf("row = %+v, want the active movement with account resolved to its name", rows[0])
	}

	all, err := uc.Execute(ctx, "u1", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 {
		t.Fatalf("include_cancelled export returned %d rows, want 4 (active + voided + original + reversal); got %+v", len(all), all)
	}
	byAmount := map[int64]ExportedRow{}
	for _, r := range all {
		byAmount[r.Amount] = r
	}
	if byAmount[voided.Amount].Status != string(entities.MovementStatusVoided) {
		t.Errorf("voided row status = %q, want %q", byAmount[voided.Amount].Status, entities.MovementStatusVoided)
	}
	if byAmount[reversal.Amount].CancelsMovementID != original.ID {
		t.Errorf("reversal row cancels_movement_id = %q, want %q", byAmount[reversal.Amount].CancelsMovementID, original.ID)
	}
}

// TestExportImportRoundTrip mirrors the ticket's own acceptance
// criterion: export -> wipe -> import must reproduce the same movement
// list and balance. It re-encodes ExportedRow into ImportRowInput the
// same way the HTTP handlers' CSV columns do (export_handler.go writes
// exactly these fields in this order; import_handler.go's CSV parser
// reads them back the same way), without going through actual CSV text —
// the encode/decode symmetry is what this test is protecting.
func TestExportImportRoundTrip(t *testing.T) {
	exportUC, srcMovements, srcAccounts := newExportFixtures()
	ctx := context.Background()

	account, err := srcAccounts.Create(ctx, &dto.AccountDTO{UserID: "u1", Name: "Wallet", Type: "cash", Currency: "usd"})
	if err != nil {
		t.Fatal(err)
	}
	srcMovements.add(&dto.MovementDTO{
		UserID: "u1", Amount: -4590, Currency: "usd", Description: "Groceries",
		Category: "food", PaymentMethod: "debit_card", AccountID: &account.ID,
		Status: string(entities.MovementStatusActive), Timestamp: time.Date(2025, 11, 3, 0, 0, 0, 0, time.UTC),
	})
	srcMovements.add(&dto.MovementDTO{
		UserID: "u1", Amount: 250000, Currency: "usd", Description: "Salary",
		Category: "income", PaymentMethod: "bank_transfer",
		Status: string(entities.MovementStatusActive), Timestamp: time.Date(2025, 11, 5, 0, 0, 0, 0, time.UTC),
	})

	exported, err := exportUC.Execute(ctx, "u1", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(exported) != 2 {
		t.Fatalf("exported %d rows, want 2", len(exported))
	}

	// "Wipe data file → import" — a brand new set of repositories, same
	// account name pre-registered so account-by-name resolution works.
	importUC, dstMovements, dstAccounts, _ := newImportFixtures()
	if _, err := dstAccounts.Create(ctx, &dto.AccountDTO{UserID: "u1", Name: "Wallet", Type: "cash", Currency: "usd"}); err != nil {
		t.Fatal(err)
	}

	rows := make([]ImportRowInput, len(exported))
	for i, e := range exported {
		rows[i] = ImportRowInput{
			Date: e.Date, Amount: strconv.FormatInt(e.Amount, 10), Currency: e.Currency,
			Description: e.Description, Category: e.Category, PaymentMethod: e.PaymentMethod, Account: e.Account,
		}
	}

	result, err := importUC.Execute(ctx, ImportMovementsInput{UserID: "u1", Rows: rows})
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 2 || len(result.Errors) != 0 {
		t.Fatalf("import result = %+v, want 2 imported, no errors", result)
	}

	imported, err := dstMovements.ListByUser(ctx, "u1", nil, nil, nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	var srcBalance, dstBalance int64
	for _, m := range srcMovements.byID {
		if m.Status != string(entities.MovementStatusVoided) {
			srcBalance += m.Amount
		}
	}
	for _, m := range imported {
		if m.Status != string(entities.MovementStatusVoided) {
			dstBalance += m.Amount
		}
	}
	if srcBalance != dstBalance {
		t.Errorf("balance after round-trip = %d, want %d", dstBalance, srcBalance)
	}
	if len(imported) != 2 {
		t.Fatalf("imported %d movements, want 2", len(imported))
	}
}
