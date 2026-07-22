package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

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

	all, err := repo.ListByUser(ctx, "00000000-0000-0000-0000-000000000001", nil, 0, 0)
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
	filtered, err := repo.ListByUser(ctx, "00000000-0000-0000-0000-000000000001", &brl, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || filtered[0].Currency != "brl" {
		t.Errorf("currency filter: got %d rows", len(filtered))
	}

	page, err := repo.ListByUser(ctx, "00000000-0000-0000-0000-000000000001", nil, 1, 1)
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
	rows, _ := repo.ListByUser(ctx, original.UserID, nil, 0, 0)
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
