package usecases

import (
	"context"
	"testing"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
)

func newImportFixtures() (*importMovementsUseCase, *fakeMovementRepo, *fakeAccountRepo, *fakeCurrencyRepo) {
	movements := newFakeMovementRepo()
	accounts := newFakeAccountRepo()
	currencies := newFakeCurrencyRepo("usd", "brl")
	uc := &importMovementsUseCase{movements: movements, accounts: accounts, currencies: currencies, methods: newFakePaymentMethodRepo()}
	return uc, movements, accounts, currencies
}

func validImportRow() ImportRowInput {
	return ImportRowInput{
		Date: "2025-11-03", Amount: "-4590", Currency: "usd",
		Description: "Groceries", Category: "food", PaymentMethod: "debit_card",
	}
}

func TestImportRoundTripValidRows(t *testing.T) {
	uc, movements, _, _ := newImportFixtures()

	row1 := validImportRow()
	row2 := ImportRowInput{Date: "2025-11-05", Amount: "250000", Currency: "usd", Description: "Salary", Category: "income", PaymentMethod: "bank_transfer"}

	result, err := uc.Execute(context.Background(), ImportMovementsInput{
		UserID: "u1", Rows: []ImportRowInput{row1, row2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 2 || result.Skipped != 0 || len(result.Errors) != 0 || len(result.Duplicates) != 0 {
		t.Fatalf("result = %+v, want 2 imported, nothing else", result)
	}

	listed, err := movements.ListByUser(context.Background(), "u1", nil, nil, nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 {
		t.Fatalf("listed %d movements, want 2", len(listed))
	}
}

func TestImportValidatesBadDate(t *testing.T) {
	uc, _, _, _ := newImportFixtures()
	row := validImportRow()
	row.Date = "11/03/2025"

	result, err := uc.Execute(context.Background(), ImportMovementsInput{UserID: "u1", Rows: []ImportRowInput{row}})
	if err != nil {
		t.Fatal(err)
	}
	assertSingleFieldError(t, result, "date")
}

func TestImportValidatesZeroAmount(t *testing.T) {
	uc, _, _, _ := newImportFixtures()
	row := validImportRow()
	row.Amount = "0"

	result, err := uc.Execute(context.Background(), ImportMovementsInput{UserID: "u1", Rows: []ImportRowInput{row}})
	if err != nil {
		t.Fatal(err)
	}
	assertSingleFieldError(t, result, "amount")
}

func TestImportValidatesUnknownCategory(t *testing.T) {
	uc, _, _, _ := newImportFixtures()
	row := validImportRow()
	row.Category = "not-a-real-category"

	result, err := uc.Execute(context.Background(), ImportMovementsInput{UserID: "u1", Rows: []ImportRowInput{row}})
	if err != nil {
		t.Fatal(err)
	}
	assertSingleFieldError(t, result, "category")
}

func TestImportValidatesUnknownCurrency(t *testing.T) {
	uc, _, _, _ := newImportFixtures()
	row := validImportRow()
	row.Currency = "xyz"

	result, err := uc.Execute(context.Background(), ImportMovementsInput{UserID: "u1", Rows: []ImportRowInput{row}})
	if err != nil {
		t.Fatal(err)
	}
	assertSingleFieldError(t, result, "currency")
}

func TestImportValidatesUnknownAccountName(t *testing.T) {
	uc, _, _, _ := newImportFixtures()
	row := validImportRow()
	row.Account = "No Such Account"

	result, err := uc.Execute(context.Background(), ImportMovementsInput{UserID: "u1", Rows: []ImportRowInput{row}})
	if err != nil {
		t.Fatal(err)
	}
	assertSingleFieldError(t, result, "account")
}

func TestImportResolvesAccountCaseInsensitivelyAndChecksCurrency(t *testing.T) {
	uc, _, accounts, _ := newImportFixtures()
	account, _ := accounts.Create(context.Background(), &dto.AccountDTO{UserID: "u1", Name: "Main Bank", Currency: "usd"})

	row := validImportRow()
	row.Account = "MAIN bank" // different case than stored

	result, err := uc.Execute(context.Background(), ImportMovementsInput{UserID: "u1", Rows: []ImportRowInput{row}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Errors) != 0 || result.Imported != 1 {
		t.Fatalf("expected the case-insensitive match to resolve, got %+v", result)
	}

	// Mismatched currency must be rejected.
	brlRow := validImportRow()
	brlRow.Currency = "brl"
	brlRow.Account = "Main Bank"
	result, err = uc.Execute(context.Background(), ImportMovementsInput{UserID: "u1", Rows: []ImportRowInput{brlRow}})
	if err != nil {
		t.Fatal(err)
	}
	assertSingleFieldError(t, result, "account")
	_ = account
}

func TestImportStrictModeAbortsOnAnyRowError(t *testing.T) {
	uc, movements, _, _ := newImportFixtures()
	good := validImportRow()
	bad := validImportRow()
	bad.Amount = "not-a-number"

	result, err := uc.Execute(context.Background(), ImportMovementsInput{
		UserID: "u1", Rows: []ImportRowInput{good, bad},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 0 || result.Skipped != 0 || len(result.Errors) != 1 {
		t.Fatalf("strict mode with a bad row should import nothing, got %+v", result)
	}
	listed, _ := movements.ListByUser(context.Background(), "u1", nil, nil, nil, 0, 0)
	if len(listed) != 0 {
		t.Errorf("strict mode must write nothing on any error, got %d movements", len(listed))
	}
}

func TestImportPartialModeInsertsOnlyValidRows(t *testing.T) {
	uc, movements, _, _ := newImportFixtures()
	good := validImportRow()
	bad := validImportRow()
	bad.Amount = "not-a-number"

	result, err := uc.Execute(context.Background(), ImportMovementsInput{
		UserID: "u1", Rows: []ImportRowInput{good, bad}, AllowPartial: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 1 || result.Skipped != 1 || len(result.Errors) != 1 {
		t.Fatalf("partial mode should import the good row and skip the bad one, got %+v", result)
	}
	listed, _ := movements.ListByUser(context.Background(), "u1", nil, nil, nil, 0, 0)
	if len(listed) != 1 {
		t.Errorf("want 1 movement written, got %d", len(listed))
	}
}

func TestImportDryRunWritesNothing(t *testing.T) {
	uc, movements, _, _ := newImportFixtures()
	row := validImportRow()

	result, err := uc.Execute(context.Background(), ImportMovementsInput{
		UserID: "u1", Rows: []ImportRowInput{row}, DryRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 1 {
		t.Errorf("dry_run should still report what WOULD be imported, got %+v", result)
	}
	listed, _ := movements.ListByUser(context.Background(), "u1", nil, nil, nil, 0, 0)
	if len(listed) != 0 {
		t.Errorf("dry_run must write nothing, got %d movements", len(listed))
	}
}

func TestImportFlagsDuplicateAgainstExistingMovement(t *testing.T) {
	uc, movements, _, _ := newImportFixtures()
	row := validImportRow()

	// First import: lands normally.
	if _, err := uc.Execute(context.Background(), ImportMovementsInput{UserID: "u1", Rows: []ImportRowInput{row}}); err != nil {
		t.Fatal(err)
	}
	existing, _ := movements.ListByUser(context.Background(), "u1", nil, nil, nil, 0, 0)
	if len(existing) != 1 {
		t.Fatalf("setup: want 1 existing movement, got %d", len(existing))
	}

	// Re-importing the same row: flagged as a duplicate, but still
	// imported by default (per BACK-03: duplicates are warnings).
	result, err := uc.Execute(context.Background(), ImportMovementsInput{UserID: "u1", Rows: []ImportRowInput{row}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Duplicates) != 1 || result.Duplicates[0].Row != 1 {
		t.Fatalf("want row 1 flagged as duplicate, got %+v", result.Duplicates)
	}
	if result.Duplicates[0].ExistingMovementID == nil || *result.Duplicates[0].ExistingMovementID != existing[0].ID {
		t.Errorf("duplicate should reference the existing movement id, got %+v", result.Duplicates[0])
	}
	if result.Imported != 1 {
		t.Errorf("duplicates still import by default, got imported=%d", result.Imported)
	}
}

func TestImportFlagsInFileDuplicate(t *testing.T) {
	uc, _, _, _ := newImportFixtures()
	row := validImportRow()

	result, err := uc.Execute(context.Background(), ImportMovementsInput{
		UserID: "u1", Rows: []ImportRowInput{row, row}, // identical rows
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Duplicates) != 1 || result.Duplicates[0].Row != 2 {
		t.Fatalf("want row 2 flagged as duplicate of row 1, got %+v", result.Duplicates)
	}
	if result.Duplicates[0].DuplicateOfRow == nil || *result.Duplicates[0].DuplicateOfRow != 1 {
		t.Errorf("want duplicate_of_row=1, got %+v", result.Duplicates[0])
	}
	if result.Imported != 2 {
		t.Errorf("both rows still import by default, got imported=%d", result.Imported)
	}
}

func TestImportSkipDuplicatesExcludesExactlyTheFlaggedRows(t *testing.T) {
	uc, movements, _, _ := newImportFixtures()
	row := validImportRow()
	other := validImportRow()
	other.Description = "Completely different"

	result, err := uc.Execute(context.Background(), ImportMovementsInput{
		UserID: "u1", Rows: []ImportRowInput{row, row, other}, SkipDuplicates: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Duplicates) != 1 || result.Duplicates[0].Row != 2 {
		t.Fatalf("want exactly row 2 flagged, got %+v", result.Duplicates)
	}
	if result.Imported != 2 || result.Skipped != 1 {
		t.Fatalf("want 2 imported (row 1 + row 3), 1 skipped (row 2), got %+v", result)
	}
	listed, _ := movements.ListByUser(context.Background(), "u1", nil, nil, nil, 0, 0)
	if len(listed) != 2 {
		t.Errorf("want 2 movements written, got %d", len(listed))
	}
}

func TestImportRequiresUserID(t *testing.T) {
	uc, _, _, _ := newImportFixtures()
	if _, err := uc.Execute(context.Background(), ImportMovementsInput{Rows: []ImportRowInput{validImportRow()}}); err == nil {
		t.Error("want an error when UserID is empty")
	}
}

func assertSingleFieldError(t *testing.T, result ImportMovementsResult, field string) {
	t.Helper()
	if len(result.Errors) != 1 {
		t.Fatalf("want exactly 1 error, got %+v", result.Errors)
	}
	if result.Errors[0].Field != field {
		t.Errorf("error field = %q, want %q (%+v)", result.Errors[0].Field, field, result.Errors[0])
	}
	if result.Imported != 0 {
		t.Errorf("a row with an error must not be imported (strict default), got imported=%d", result.Imported)
	}
}
