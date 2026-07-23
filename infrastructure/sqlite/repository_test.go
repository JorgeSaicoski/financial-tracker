package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func testMovement(amount int64) *entities.Movement {
	now := time.Now().UTC()
	return &entities.Movement{
		UserID:        "00000000-0000-0000-0000-000000000001",
		Amount:        amount,
		Currency:      "usd",
		Description:   "coffee",
		Category:      entities.CategoryFood,
		PaymentMethod: entities.PaymentMethodCash,
		Status:        entities.MovementStatusActive,
		SyncStatus:    entities.SyncStatusPending,
		Timestamp:     now,
		CreatedAt:     now,
	}
}

func TestMovementCreateGetRoundtrip(t *testing.T) {
	repo := NewMovementRepository(openTestDB(t))
	ctx := context.Background()

	created, err := repo.Create(ctx, testMovement(-450))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("no id generated")
	}

	got, err := repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Amount != -450 || got.Description != "coffee" ||
		got.Category != entities.CategoryFood || got.PaymentMethod != entities.PaymentMethodCash ||
		got.Status != entities.MovementStatusActive || got.SyncStatus != entities.SyncStatusPending {
		t.Errorf("roundtrip mismatch: %+v", got)
	}
	if !got.Timestamp.Equal(created.Timestamp) {
		t.Errorf("timestamp %s != %s", got.Timestamp, created.Timestamp)
	}
	if got.CreditCardPurchaseID != nil || got.CancelsMovementID != nil || got.LedgerTransactionID != nil {
		t.Error("nullable fields should be nil")
	}

	if _, err := repo.GetByID(ctx, "missing"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("missing id: want ErrNotFound, got %v", err)
	}
}

func TestMovementListByUser(t *testing.T) {
	repo := NewMovementRepository(openTestDB(t))
	ctx := context.Background()

	for i, amount := range []int64{-100, -200, 300} {
		m := testMovement(amount)
		m.Timestamp = m.Timestamp.Add(time.Duration(i) * time.Minute)
		if i == 2 {
			m.Currency = "brl"
		}
		if _, err := repo.Create(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	all, err := repo.ListByUser(ctx, "00000000-0000-0000-0000-000000000001", nil, nil, nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("listed %d movements, want 3", len(all))
	}
	// Newest first.
	if all[0].Amount != 300 || all[2].Amount != -100 {
		t.Errorf("wrong order: %d, %d, %d", all[0].Amount, all[1].Amount, all[2].Amount)
	}

	brl := "brl"
	filtered, err := repo.ListByUser(ctx, "00000000-0000-0000-0000-000000000001", &brl, nil, nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || filtered[0].Currency != "brl" {
		t.Errorf("currency filter: got %d rows", len(filtered))
	}

	page, err := repo.ListByUser(ctx, "00000000-0000-0000-0000-000000000001", nil, nil, nil, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 1 || page[0].Amount != -200 {
		t.Errorf("pagination: got %+v", page)
	}
}

func TestListPendingSyncFilters(t *testing.T) {
	repo := NewMovementRepository(openTestDB(t))
	ctx := context.Background()
	now := time.Now().UTC()

	due := testMovement(-100)
	due.Timestamp = now.Add(-time.Hour)
	due, _ = repo.Create(ctx, due)

	future := testMovement(-200) // installment not yet due
	future.Timestamp = now.AddDate(0, 1, 0)
	future, _ = repo.Create(ctx, future)

	synced := testMovement(-300)
	synced, _ = repo.Create(ctx, synced)
	if err := repo.MarkSynced(ctx, synced.ID, "ledger-1", now); err != nil {
		t.Fatal(err)
	}

	voided := testMovement(-400)
	voided, _ = repo.Create(ctx, voided)
	if err := repo.Void(ctx, voided.ID); err != nil {
		t.Fatal(err)
	}

	failedRecently := testMovement(-500)
	failedRecently.Timestamp = now.Add(-time.Hour)
	failedRecently, _ = repo.Create(ctx, failedRecently)
	if err := repo.MarkSyncFailed(ctx, failedRecently.ID, "boom", now); err != nil {
		t.Fatal(err)
	}

	pending, err := repo.ListPendingSync(ctx, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].ID != due.ID {
		t.Fatalf("cooldown pass: want only %s, got %d rows", due.ID, len(pending))
	}

	// Zero cooldown (manual sync) also picks up the fresh failure — but
	// never the future, synced, or voided rows.
	pending, err = repo.ListPendingSync(ctx, now, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 {
		t.Fatalf("manual pass: want 2 rows, got %d", len(pending))
	}

	got, _ := repo.GetByID(ctx, failedRecently.ID)
	if got.SyncStatus != entities.SyncStatusFailed || got.SyncAttempts != 1 ||
		got.LastSyncError == nil || *got.LastSyncError != "boom" {
		t.Errorf("failure not recorded: %+v", got)
	}
	got, _ = repo.GetByID(ctx, synced.ID)
	if got.LedgerTransactionID == nil || *got.LedgerTransactionID != "ledger-1" || got.SyncedAt == nil {
		t.Errorf("sync success not recorded: %+v", got)
	}
}

func TestCreateReversalLinksAtomically(t *testing.T) {
	repo := NewMovementRepository(openTestDB(t))
	ctx := context.Background()

	original, err := repo.Create(ctx, testMovement(-450))
	if err != nil {
		t.Fatal(err)
	}

	makeReversal := func() *entities.Movement {
		r := testMovement(450)
		r.CancelsMovementID = &original.ID
		return r
	}

	reversal, err := repo.CreateReversal(ctx, makeReversal())
	if err != nil {
		t.Fatalf("create reversal: %v", err)
	}

	got, _ := repo.GetByID(ctx, original.ID)
	if got.ReversedByMovementID == nil || *got.ReversedByMovementID != reversal.ID {
		t.Error("original not linked to reversal")
	}
	gotRev, _ := repo.GetByID(ctx, reversal.ID)
	if gotRev.CancelsMovementID == nil || *gotRev.CancelsMovementID != original.ID {
		t.Error("reversal not linked to original")
	}

	// A second reversal of the same movement must conflict, and the
	// losing insert must not survive.
	if _, err := repo.CreateReversal(ctx, makeReversal()); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("second reversal: want ErrConflict, got %v", err)
	}
	rows, _ := repo.ListByUser(ctx, original.UserID, nil, nil, nil, 0, 0)
	if len(rows) != 2 {
		t.Errorf("expected 2 rows after failed second reversal, got %d", len(rows))
	}

	missing := "does-not-exist"
	bad := testMovement(1)
	bad.CancelsMovementID = &missing
	if _, err := repo.CreateReversal(ctx, bad); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("reversal of missing movement: want ErrNotFound, got %v", err)
	}
}

func TestMovementUpdateMetadataAndFinancial(t *testing.T) {
	db := openTestDB(t)
	repo := NewMovementRepository(db)
	ctx := context.Background()

	created, err := repo.Create(ctx, testMovement(-450))
	if err != nil {
		t.Fatal(err)
	}

	account, err := NewAccountRepository(db).Create(ctx, &entities.Account{
		UserID: created.UserID, Name: "wallet", Type: entities.AccountTypeCash,
		Currency: "usd", CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.UpdateMetadata(ctx, created.ID, "renamed", entities.CategoryTransport, entities.PaymentMethodPix, &account.ID); err != nil {
		t.Fatalf("update metadata: %v", err)
	}
	got, err := repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != "renamed" || got.Category != entities.CategoryTransport ||
		got.PaymentMethod != entities.PaymentMethodPix || got.AccountID == nil || *got.AccountID != account.ID {
		t.Errorf("metadata not persisted: %+v", got)
	}
	if got.Amount != -450 {
		t.Errorf("financial fields must be untouched by UpdateMetadata: %+v", got)
	}

	newTimestamp := created.Timestamp.Add(24 * time.Hour)
	if err := repo.UpdateFinancial(ctx, created.ID, -900, "brl", newTimestamp); err != nil {
		t.Fatalf("update financial: %v", err)
	}
	got, err = repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Amount != -900 || got.Currency != "brl" || !got.Timestamp.Equal(newTimestamp) {
		t.Errorf("financial fields not persisted: %+v", got)
	}
	if got.Description != "renamed" {
		t.Errorf("metadata must be untouched by UpdateFinancial: %+v", got)
	}

	if err := repo.UpdateMetadata(ctx, "missing", "x", entities.CategoryOther, entities.PaymentMethodOther, nil); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("update metadata on missing id: want ErrNotFound, got %v", err)
	}
	if err := repo.UpdateFinancial(ctx, "missing", -1, "usd", time.Now()); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("update financial on missing id: want ErrNotFound, got %v", err)
	}
}

func TestMovementCreateBatchAtomicity(t *testing.T) {
	repo := NewMovementRepository(openTestDB(t))
	ctx := context.Background()

	transferID := "transfer-1"
	debit := testMovement(-500)
	debit.TransferID = &transferID
	credit := testMovement(500)
	credit.TransferID = &transferID

	created, err := repo.CreateBatch(ctx, []*entities.Movement{debit, credit})
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}

	legs, err := repo.ListByTransferID(ctx, transferID)
	if err != nil {
		t.Fatal(err)
	}
	if len(legs) != 2 || legs[0].Amount != -500 || legs[1].Amount != 500 {
		t.Fatalf("legs = %+v, want debit then credit", legs)
	}
	if legs[0].ID != created[0].ID || legs[1].ID != created[1].ID {
		t.Error("returned movements don't match what was persisted")
	}

	// Second leg fails (duplicate ID collides with the first, already
	// committed row) — the first leg of this new batch must not survive
	// either, or the transfer would create money out of nowhere.
	dupID := debit.ID
	firstOfSecondBatch := testMovement(-100)
	secondOfSecondBatch := testMovement(100)
	secondOfSecondBatch.ID = dupID

	if _, err := repo.CreateBatch(ctx, []*entities.Movement{firstOfSecondBatch, secondOfSecondBatch}); err == nil {
		t.Fatal("expected the batch to fail on the colliding second leg")
	}
	if _, err := repo.GetByID(ctx, firstOfSecondBatch.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("first leg of the failed batch must have rolled back, got %v", err)
	}
}

// TestTransactRollsBackReversalWhenLaterWriteFails is the repository-level
// proof behind update_movement's post-sync path (and cancel_transfer's
// per-leg loop): both wrap a CreateReversal plus a later write in one
// Transact specifically so that if the later write fails, the reversal —
// which on its own commits immediately — gets undone too. Without this
// guarantee, a failure after the reversal would leave a movement
// compensated with nothing to show for it: money silently disappearing.
func TestTransactRollsBackReversalWhenLaterWriteFails(t *testing.T) {
	repo := NewMovementRepository(openTestDB(t))
	ctx := context.Background()

	original, err := repo.Create(ctx, testMovement(10000))
	if err != nil {
		t.Fatal(err)
	}

	// The later write collides on ID with a row that will only exist
	// once this same Transact has already inserted it — the simplest way
	// to force a real, deterministic failure on the second write.
	dupID := "collide-1"

	err = repo.Transact(ctx, func(tx repositories.MovementRepository) error {
		reversal := testMovement(-10000)
		reversal.ID = dupID
		reversal.CancelsMovementID = &original.ID
		if _, err := tx.CreateReversal(ctx, reversal); err != nil {
			return err
		}

		// This second insert collides with the reversal's own ID and
		// must fail, taking the whole transaction down with it.
		colliding := testMovement(-999)
		colliding.ID = dupID
		_, err := tx.Create(ctx, colliding)
		return err
	})
	if err == nil {
		t.Fatal("expected the transaction to fail on the colliding second write")
	}

	got, err := repo.GetByID(ctx, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ReversedByMovementID != nil {
		t.Errorf("original must NOT be left reversed when the transaction rolled back: %+v", got)
	}
	if _, err := repo.GetByID(ctx, dupID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("the reversal must have rolled back too, got %v", err)
	}
}

func TestPurchaseCreateWithInstallments(t *testing.T) {
	db := openTestDB(t)
	purchases := NewCreditCardPurchaseRepository(db)
	movements := NewMovementRepository(db)
	ctx := context.Background()
	now := time.Now().UTC()

	purchase := &entities.CreditCardPurchase{
		UserID:           "00000000-0000-0000-0000-000000000001",
		Description:      "tv",
		Category:         entities.CategoryShopping,
		TotalAmount:      -900,
		Currency:         "usd",
		InstallmentCount: 3,
		PurchaseDate:     now,
		Status:           entities.CreditCardPurchaseStatusActive,
		CreatedAt:        now,
	}
	var installments []*entities.Movement
	for i := 0; i < 3; i++ {
		m := testMovement(-300)
		m.PaymentMethod = entities.PaymentMethodCreditCard
		n := i + 1
		m.InstallmentNumber = &n
		m.Timestamp = now.AddDate(0, i, 0)
		installments = append(installments, m)
	}

	purchase, _, err := purchases.CreateWithInstallments(ctx, purchase, installments)
	if err != nil {
		t.Fatalf("create purchase: %v", err)
	}

	got, err := purchases.GetByID(ctx, purchase.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.TotalAmount != -900 || got.InstallmentCount != 3 || got.Status != entities.CreditCardPurchaseStatusActive {
		t.Errorf("purchase roundtrip mismatch: %+v", got)
	}

	linked, err := movements.ListByCreditCardPurchase(ctx, purchase.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(linked) != 3 {
		t.Fatalf("linked installments = %d, want 3", len(linked))
	}
	for i, m := range linked {
		if *m.InstallmentNumber != i+1 {
			t.Errorf("installment order broken at %d", i)
		}
	}

	if err := purchases.MarkCancelled(ctx, purchase.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = purchases.GetByID(ctx, purchase.ID)
	if got.Status != entities.CreditCardPurchaseStatusCancelled {
		t.Error("purchase not cancelled")
	}
	if err := purchases.MarkCancelled(ctx, "missing"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("cancel missing purchase: want ErrNotFound, got %v", err)
	}
}
