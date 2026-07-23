package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/JorgeSaicoski/financial-tracker/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
)

func mustCreateAccount(t *testing.T, accounts *fakeAccountRepo, userID, currency string) *dto.AccountDTO {
	t.Helper()
	a, err := accounts.Create(context.Background(), &dto.AccountDTO{UserID: userID, Currency: currency})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestTransferBetweenAccountsHappyPath(t *testing.T) {
	movements := newFakeMovementRepo()
	accounts := newFakeAccountRepo()
	from := mustCreateAccount(t, accounts, "u1", "usd")
	to := mustCreateAccount(t, accounts, "u1", "usd")

	uc := NewTransferBetweenAccounts(movements, accounts)
	result, err := uc.Execute(context.Background(), TransferBetweenAccountsInput{
		UserID: "u1", FromAccountID: from.ID, ToAccountID: to.ID, Amount: 500, Description: "moving cash",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TransferID == "" {
		t.Fatal("expected a transfer id")
	}
	if result.Debit.Amount != -500 || result.Credit.Amount != 500 {
		t.Errorf("legs = %d/%d, want -500/500", result.Debit.Amount, result.Credit.Amount)
	}
	if *result.Debit.AccountID != from.ID || *result.Credit.AccountID != to.ID {
		t.Error("legs not tied to the right accounts")
	}
	if *result.Debit.TransferID != result.TransferID || *result.Credit.TransferID != result.TransferID {
		t.Error("legs not linked by transfer id")
	}
	if result.Debit.Category != string(entities.CategoryTransfer) || result.Credit.Category != string(entities.CategoryTransfer) {
		t.Error("legs must be categorized as transfer")
	}

	// Net worth unchanged: both accounts' balances move, but they sum to
	// zero across the pair.
	fromNet, _ := movements.NetByAccount(context.Background(), from.ID, nil, nil)
	toNet, _ := movements.NetByAccount(context.Background(), to.ID, nil, nil)
	if fromNet != -500 || toNet != 500 {
		t.Errorf("account nets = %d/%d, want -500/500", fromNet, toNet)
	}
	if fromNet+toNet != 0 {
		t.Errorf("net worth changed: %d", fromNet+toNet)
	}
}

func TestTransferBetweenAccountsRejectsCurrencyMismatch(t *testing.T) {
	movements := newFakeMovementRepo()
	accounts := newFakeAccountRepo()
	from := mustCreateAccount(t, accounts, "u1", "usd")
	to := mustCreateAccount(t, accounts, "u1", "brl")

	uc := NewTransferBetweenAccounts(movements, accounts)
	_, err := uc.Execute(context.Background(), TransferBetweenAccountsInput{
		UserID: "u1", FromAccountID: from.ID, ToAccountID: to.ID, Amount: 100,
	})
	if !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}
}

func TestTransferBetweenAccountsRejectsSameAccount(t *testing.T) {
	movements := newFakeMovementRepo()
	accounts := newFakeAccountRepo()
	account := mustCreateAccount(t, accounts, "u1", "usd")

	uc := NewTransferBetweenAccounts(movements, accounts)
	_, err := uc.Execute(context.Background(), TransferBetweenAccountsInput{
		UserID: "u1", FromAccountID: account.ID, ToAccountID: account.ID, Amount: 100,
	})
	if !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}
}

func TestTransferBetweenAccountsRejectsUnknownOrForeignAccount(t *testing.T) {
	movements := newFakeMovementRepo()
	accounts := newFakeAccountRepo()
	mine := mustCreateAccount(t, accounts, "u1", "usd")
	someoneElses := mustCreateAccount(t, accounts, "u2", "usd")

	uc := NewTransferBetweenAccounts(movements, accounts)

	if _, err := uc.Execute(context.Background(), TransferBetweenAccountsInput{
		UserID: "u1", FromAccountID: mine.ID, ToAccountID: "does-not-exist", Amount: 100,
	}); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Errorf("unknown destination: want ErrInvalidInput, got %v", err)
	}

	if _, err := uc.Execute(context.Background(), TransferBetweenAccountsInput{
		UserID: "u1", FromAccountID: mine.ID, ToAccountID: someoneElses.ID, Amount: 100,
	}); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Errorf("foreign destination: want ErrInvalidInput, got %v", err)
	}
}

func TestTransferBetweenAccountsRejectsNonPositiveAmount(t *testing.T) {
	movements := newFakeMovementRepo()
	accounts := newFakeAccountRepo()
	from := mustCreateAccount(t, accounts, "u1", "usd")
	to := mustCreateAccount(t, accounts, "u1", "usd")

	uc := NewTransferBetweenAccounts(movements, accounts)
	for _, amount := range []int64{0, -100} {
		if _, err := uc.Execute(context.Background(), TransferBetweenAccountsInput{
			UserID: "u1", FromAccountID: from.ID, ToAccountID: to.ID, Amount: amount,
		}); !errors.Is(err, apperrors.ErrInvalidInput) {
			t.Errorf("amount %d: want ErrInvalidInput, got %v", amount, err)
		}
	}
}

func TestCancelTransferCancelsBothLegsPerSyncStatus(t *testing.T) {
	cases := []struct {
		name             string
		debitSynced      bool
		creditSynced     bool
		wantDebitVoided  bool
		wantCreditVoided bool
		wantSyncTrigger  bool
	}{
		{"both pending", false, false, true, true, false},
		{"both synced", true, true, false, false, true},
		{"debit synced only", true, false, false, true, true},
		{"credit synced only", false, true, true, false, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			movements := newFakeMovementRepo()
			accounts := newFakeAccountRepo()
			from := mustCreateAccount(t, accounts, "u1", "usd")
			to := mustCreateAccount(t, accounts, "u1", "usd")

			transferUC := NewTransferBetweenAccounts(movements, accounts)
			transfer, err := transferUC.Execute(context.Background(), TransferBetweenAccountsInput{
				UserID: "u1", FromAccountID: from.ID, ToAccountID: to.ID, Amount: 500,
			})
			if err != nil {
				t.Fatal(err)
			}
			if tc.debitSynced {
				if err := movements.MarkSynced(context.Background(), transfer.Debit.ID, "ledger-d", transfer.Debit.Timestamp); err != nil {
					t.Fatal(err)
				}
			}
			if tc.creditSynced {
				if err := movements.MarkSynced(context.Background(), transfer.Credit.ID, "ledger-c", transfer.Credit.Timestamp); err != nil {
					t.Fatal(err)
				}
			}

			trigger := &fakeSyncTrigger{}
			result, err := NewCancelTransfer(movements, trigger).Execute(context.Background(), transfer.TransferID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tc.wantDebitVoided {
				if result.Debit.Movement.Status != string(entities.MovementStatusVoided) || result.Debit.Reversal != nil {
					t.Errorf("debit should be voided: %+v", result.Debit)
				}
			} else {
				if result.Debit.Reversal == nil {
					t.Errorf("debit should be reversed: %+v", result.Debit)
				}
			}
			if tc.wantCreditVoided {
				if result.Credit.Movement.Status != string(entities.MovementStatusVoided) || result.Credit.Reversal != nil {
					t.Errorf("credit should be voided: %+v", result.Credit)
				}
			} else {
				if result.Credit.Reversal == nil {
					t.Errorf("credit should be reversed: %+v", result.Credit)
				}
			}
			if (trigger.calls > 0) != tc.wantSyncTrigger {
				t.Errorf("sync trigger calls = %d, want triggered=%v", trigger.calls, tc.wantSyncTrigger)
			}

			// Whichever way each leg was cancelled, the pair still nets
			// to zero: cancelling a transfer must never change net worth.
			fromNet, _ := movements.NetByAccount(context.Background(), from.ID, nil, nil)
			toNet, _ := movements.NetByAccount(context.Background(), to.ID, nil, nil)
			if fromNet+toNet != 0 {
				t.Errorf("net worth changed after cancel: %d + %d != 0", fromNet, toNet)
			}
		})
	}
}

// TestCancelTransferRollsBackFirstLegWhenSecondLegFails is the regression
// test for the same bug class as update_movement's: cancelling a transfer
// loops over both legs and used to commit each leg's cancel independently.
// If the second leg's cancel ever failed, the first was already gone,
// leaving a "half-cancelled" transfer with one leg voided/reversed and the
// other still active — breaking the zero-net-worth invariant. The fix
// wraps both legs in one Transact; this forces the second (credit) leg to
// fail and asserts the first (debit) leg rolls back with it.
func TestCancelTransferRollsBackFirstLegWhenSecondLegFails(t *testing.T) {
	movements := newFakeMovementRepo()
	accounts := newFakeAccountRepo()
	from := mustCreateAccount(t, accounts, "u1", "usd")
	to := mustCreateAccount(t, accounts, "u1", "usd")

	transferUC := NewTransferBetweenAccounts(movements, accounts)
	transfer, err := transferUC.Execute(context.Background(), TransferBetweenAccountsInput{
		UserID: "u1", FromAccountID: from.ID, ToAccountID: to.ID, Amount: 500,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Both legs synced, so cancelling either goes through the
	// reversal path rather than a plain void.
	if err := movements.MarkSynced(context.Background(), transfer.Debit.ID, "ledger-d", transfer.Debit.Timestamp); err != nil {
		t.Fatal(err)
	}
	if err := movements.MarkSynced(context.Background(), transfer.Credit.ID, "ledger-c", transfer.Credit.Timestamp); err != nil {
		t.Fatal(err)
	}

	// ListByTransferID orders debit (negative) before credit (positive),
	// so the credit leg is the "second write" here.
	movements.createReversalErrForID = transfer.Credit.ID

	trigger := &fakeSyncTrigger{}
	_, err = NewCancelTransfer(movements, trigger).Execute(context.Background(), transfer.TransferID)
	if err == nil {
		t.Fatal("expected the cancel to fail when the second leg's reversal fails")
	}
	if trigger.calls != 0 {
		t.Error("a failed cancel must not trigger a sync")
	}

	debit, _ := movements.GetByID(context.Background(), transfer.Debit.ID)
	if debit.ReversedByMovementID != nil {
		t.Errorf("debit leg must NOT be left reversed when the credit leg's cancel failed: %+v", debit)
	}
	credit, _ := movements.GetByID(context.Background(), transfer.Credit.ID)
	if credit.ReversedByMovementID != nil {
		t.Errorf("credit leg must not be reversed either — the whole cancel failed: %+v", credit)
	}

	fromNet, _ := movements.NetByAccount(context.Background(), from.ID, nil, nil)
	toNet, _ := movements.NetByAccount(context.Background(), to.ID, nil, nil)
	if fromNet+toNet != 0 {
		t.Errorf("net worth must be unaffected by a fully-rolled-back cancel: %d + %d != 0", fromNet, toNet)
	}
}

func TestCancelTransferMissingID(t *testing.T) {
	movements := newFakeMovementRepo()
	uc := NewCancelTransfer(movements, &fakeSyncTrigger{})
	if _, err := uc.Execute(context.Background(), "nope"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestCancelMovementRejectsDirectSingleLegCancel(t *testing.T) {
	movements := newFakeMovementRepo()
	accounts := newFakeAccountRepo()
	from := mustCreateAccount(t, accounts, "u1", "usd")
	to := mustCreateAccount(t, accounts, "u1", "usd")

	transfer, err := NewTransferBetweenAccounts(movements, accounts).Execute(context.Background(), TransferBetweenAccountsInput{
		UserID: "u1", FromAccountID: from.ID, ToAccountID: to.ID, Amount: 500,
	})
	if err != nil {
		t.Fatal(err)
	}

	uc := NewCancelMovement(movements, &fakeSyncTrigger{})
	if _, err := uc.Execute(context.Background(), transfer.Debit.ID); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}
}
