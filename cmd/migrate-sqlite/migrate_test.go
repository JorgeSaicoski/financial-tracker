package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
	"github.com/JorgeSaicoski/financial-tracker/infrastructure/postgresql"
	"github.com/JorgeSaicoski/financial-tracker/infrastructure/sqlite"
)

// openSourceDB is a fresh SQLite database seeded through the normal
// repositories, standing in for a real deployment's local database.
func openSourceDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "source.db"))
	if err != nil {
		t.Fatalf("open source: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := sqlite.Migrate(db); err != nil {
		t.Fatalf("migrate source: %v", err)
	}
	return db
}

// openTargetDB connects to TEST_DATABASE_URL and wipes every table this
// tool writes to, so the migration test starts from a clean target —
// mirroring infrastructure/postgresql/repository_test.go's own guard.
func openTargetDB(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping migrate-sqlite integration test")
	}

	db, err := postgresql.Open(url, postgresql.PoolConfig{})
	if err != nil {
		t.Fatalf("open target: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := postgresql.Migrate(db); err != nil {
		t.Fatalf("migrate target: %v", err)
	}
	if _, err := db.Exec(`TRUNCATE TABLE account_snapshots, movements, credit_card_purchases, accounts CASCADE`); err != nil {
		t.Fatalf("truncate target: %v", err)
	}
	return db
}

const testUserID = "00000000-0000-0000-0000-000000000001"

func nowTruncated() time.Time {
	return time.Now().UTC().Truncate(time.Microsecond)
}

// seedSource populates the source SQLite database with one instance of
// every link type BACK-06 must preserve: a synced movement, a pending
// movement, a reversal pair, an installment set, an account with a
// snapshot, and a transfer pair.
func seedSource(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()

	movementRepo := sqlite.NewMovementRepository(db)
	purchaseRepo := sqlite.NewCreditCardPurchaseRepository(db)
	accountRepo := sqlite.NewAccountRepository(db)

	account, err := accountRepo.Create(ctx, &dto.AccountDTO{
		UserID:    testUserID,
		Name:      "checking",
		Type:      string(entities.AccountTypeBank),
		Currency:  "usd",
		CreatedAt: nowTruncated(),
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if _, err := accountRepo.AddSnapshot(ctx, &dto.AccountSnapshotDTO{
		AccountID: account.ID,
		Balance:   100000,
		Timestamp: nowTruncated(),
		CreatedAt: nowTruncated(),
	}); err != nil {
		t.Fatalf("add snapshot: %v", err)
	}

	now := nowTruncated()
	syncedAt := now
	synced, err := movementRepo.Create(ctx, &dto.MovementDTO{
		UserID:        testUserID,
		Amount:        -500,
		Currency:      "usd",
		Description:   "synced coffee",
		Category:      string(entities.CategoryFood),
		PaymentMethod: string(entities.PaymentMethodCash),
		AccountID:     &account.ID,
		Status:        string(entities.MovementStatusActive),
		SyncStatus:    string(entities.SyncStatusPending),
		Timestamp:     now,
		CreatedAt:     now,
	})
	if err != nil {
		t.Fatalf("create synced movement: %v", err)
	}
	if err := movementRepo.MarkSynced(ctx, synced.ID, "ledger-tx-1", syncedAt); err != nil {
		t.Fatalf("mark synced: %v", err)
	}

	if _, err := movementRepo.Create(ctx, &dto.MovementDTO{
		UserID:        testUserID,
		Amount:        -1200,
		Currency:      "usd",
		Description:   "pending lunch",
		Category:      string(entities.CategoryFood),
		PaymentMethod: string(entities.PaymentMethodDebitCard),
		Status:        string(entities.MovementStatusActive),
		SyncStatus:    string(entities.SyncStatusPending),
		Timestamp:     now,
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("create pending movement: %v", err)
	}

	// Reversal pair: a synced movement that gets cancelled.
	toReverse, err := movementRepo.Create(ctx, &dto.MovementDTO{
		UserID:        testUserID,
		Amount:        -3000,
		Currency:      "usd",
		Description:   "will be reversed",
		Category:      string(entities.CategoryShopping),
		PaymentMethod: string(entities.PaymentMethodCreditCard),
		Status:        string(entities.MovementStatusActive),
		SyncStatus:    string(entities.SyncStatusPending),
		Timestamp:     now,
		CreatedAt:     now,
	})
	if err != nil {
		t.Fatalf("create movement to reverse: %v", err)
	}
	if err := movementRepo.MarkSynced(ctx, toReverse.ID, "ledger-tx-2", syncedAt); err != nil {
		t.Fatalf("mark synced (to reverse): %v", err)
	}
	if _, err := movementRepo.CreateReversal(ctx, &dto.MovementDTO{
		UserID:            testUserID,
		Amount:            3000,
		Currency:          "usd",
		Description:       "reversal",
		Category:          string(entities.CategoryShopping),
		PaymentMethod:     string(entities.PaymentMethodCreditCard),
		Status:            string(entities.MovementStatusActive),
		CancelsMovementID: &toReverse.ID,
		SyncStatus:        string(entities.SyncStatusPending),
		Timestamp:         now,
		CreatedAt:         now,
	}); err != nil {
		t.Fatalf("create reversal: %v", err)
	}

	// Installment set.
	if _, _, err := purchaseRepo.CreateWithInstallments(ctx,
		&dto.CreditCardPurchaseDTO{
			UserID:           testUserID,
			Description:      "new laptop",
			Category:         string(entities.CategoryShopping),
			TotalAmount:      -300000,
			Currency:         "usd",
			InstallmentCount: 3,
			PurchaseDate:     now,
			Status:           string(entities.CreditCardPurchaseStatusActive),
			CreatedAt:        now,
		},
		[]*dto.MovementDTO{
			{UserID: testUserID, Amount: -100000, Currency: "usd", Description: "new laptop 1/3", Category: string(entities.CategoryShopping), PaymentMethod: string(entities.PaymentMethodCreditCard), Status: string(entities.MovementStatusActive), SyncStatus: string(entities.SyncStatusPending), Timestamp: now, CreatedAt: now, InstallmentNumber: intPtr(1)},
			{UserID: testUserID, Amount: -100000, Currency: "usd", Description: "new laptop 2/3", Category: string(entities.CategoryShopping), PaymentMethod: string(entities.PaymentMethodCreditCard), Status: string(entities.MovementStatusActive), SyncStatus: string(entities.SyncStatusPending), Timestamp: now.AddDate(0, 1, 0), CreatedAt: now, InstallmentNumber: intPtr(2)},
			{UserID: testUserID, Amount: -100000, Currency: "usd", Description: "new laptop 3/3", Category: string(entities.CategoryShopping), PaymentMethod: string(entities.PaymentMethodCreditCard), Status: string(entities.MovementStatusActive), SyncStatus: string(entities.SyncStatusPending), Timestamp: now.AddDate(0, 2, 0), CreatedAt: now, InstallmentNumber: intPtr(3)},
		},
	); err != nil {
		t.Fatalf("create installment purchase: %v", err)
	}

	// Transfer pair.
	transferID := "transfer-1"
	if _, err := movementRepo.CreateBatch(ctx, []*dto.MovementDTO{
		{UserID: testUserID, Amount: -5000, Currency: "usd", Description: "transfer out", Category: string(entities.CategoryTransfer), PaymentMethod: string(entities.PaymentMethodOther), AccountID: &account.ID, TransferID: &transferID, Status: string(entities.MovementStatusActive), SyncStatus: string(entities.SyncStatusPending), Timestamp: now, CreatedAt: now},
		{UserID: testUserID, Amount: 5000, Currency: "usd", Description: "transfer in", Category: string(entities.CategoryTransfer), PaymentMethod: string(entities.PaymentMethodOther), TransferID: &transferID, Status: string(entities.MovementStatusActive), SyncStatus: string(entities.SyncStatusPending), Timestamp: now, CreatedAt: now},
	}); err != nil {
		t.Fatalf("create transfer pair: %v", err)
	}
}

func intPtr(n int) *int { return &n }

func TestMigrateSeedsMatchAfterCopy(t *testing.T) {
	src := openSourceDB(t)
	seedSource(t, src)
	dst := openTargetDB(t)
	ctx := context.Background()

	counts, err := run(ctx, src, dst, false)
	if err != nil {
		t.Fatalf("run: %v (counts: %+v)", err, counts)
	}
	for _, c := range counts {
		if c.source != c.target {
			t.Errorf("table %s: source=%d target=%d", c.table, c.source, c.target)
		}
	}

	// Compare through the target's own repositories, the way the API
	// would read this data back — not just raw row counts.
	movementRepo := postgresql.NewMovementRepository(dst)
	accountRepo := postgresql.NewAccountRepository(dst)

	srcMovements, err := sqlite.NewMovementRepository(src).ListByUser(ctx, testUserID, nil, nil, nil, 100, 0)
	if err != nil {
		t.Fatalf("list source movements: %v", err)
	}
	dstMovements, err := movementRepo.ListByUser(ctx, testUserID, nil, nil, nil, 100, 0)
	if err != nil {
		t.Fatalf("list target movements: %v", err)
	}
	if len(srcMovements) != len(dstMovements) {
		t.Fatalf("movement count mismatch: source=%d target=%d", len(srcMovements), len(dstMovements))
	}

	byID := make(map[string]*dto.MovementDTO, len(dstMovements))
	for _, m := range dstMovements {
		byID[m.ID] = m
	}
	for _, want := range srcMovements {
		got, ok := byID[want.ID]
		if !ok {
			t.Fatalf("movement %s missing from target", want.ID)
		}
		if got.Amount != want.Amount || got.Currency != want.Currency || got.Description != want.Description ||
			got.SyncStatus != want.SyncStatus || strDeref(got.LedgerTransactionID) != strDeref(want.LedgerTransactionID) ||
			strDeref(got.CancelsMovementID) != strDeref(want.CancelsMovementID) ||
			strDeref(got.ReversedByMovementID) != strDeref(want.ReversedByMovementID) ||
			strDeref(got.TransferID) != strDeref(want.TransferID) ||
			strDeref(got.AccountID) != strDeref(want.AccountID) ||
			strDeref(got.CreditCardPurchaseID) != strDeref(want.CreditCardPurchaseID) {
			t.Errorf("movement %s mismatch:\n  source=%+v\n  target=%+v", want.ID, want, got)
		}
		if !got.Timestamp.Equal(want.Timestamp) {
			t.Errorf("movement %s timestamp mismatch: source=%s target=%s", want.ID, want.Timestamp, got.Timestamp)
		}
	}

	// Reversal link is queryable from either side once both rows exist.
	var reversed *dto.MovementDTO
	for _, m := range srcMovements {
		if m.CancelsMovementID != nil {
			reversed = m
		}
	}
	if reversed == nil {
		t.Fatal("test setup: no reversal found in source")
	}
	original, err := movementRepo.GetByID(ctx, *reversed.CancelsMovementID)
	if err != nil {
		t.Fatalf("get reversed original: %v", err)
	}
	if original.ReversedByMovementID == nil || *original.ReversedByMovementID != reversed.ID {
		t.Errorf("original movement's ReversedByMovementID not linked back to %s: %+v", reversed.ID, original)
	}

	// Balance-relevant query matches too.
	accounts, err := accountRepo.ListByUser(ctx, testUserID)
	if err != nil {
		t.Fatalf("list target accounts: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(accounts))
	}
	net, err := movementRepo.NetByAccount(ctx, accounts[0].ID, nil, nil)
	if err != nil {
		t.Fatalf("net by account: %v", err)
	}
	srcNet, err := sqlite.NewMovementRepository(src).NetByAccount(ctx, accounts[0].ID, nil, nil)
	if err != nil {
		t.Fatalf("source net by account: %v", err)
	}
	if net != srcNet {
		t.Errorf("account net mismatch: source=%d target=%d", srcNet, net)
	}

	snapshots, err := accountRepo.LatestSnapshots(ctx, accounts[0].ID, 10)
	if err != nil {
		t.Fatalf("latest snapshots: %v", err)
	}
	if len(snapshots) != 1 || snapshots[0].Balance != 100000 {
		t.Errorf("snapshot mismatch: %+v", snapshots)
	}
}

func TestMigrateRefusesNonEmptyTargetWithoutForce(t *testing.T) {
	src := openSourceDB(t)
	seedSource(t, src)
	dst := openTargetDB(t)
	ctx := context.Background()

	if _, err := run(ctx, src, dst, false); err != nil {
		t.Fatalf("first run: %v", err)
	}

	if _, err := run(ctx, src, dst, false); err == nil {
		t.Fatal("second run without --force should have failed")
	}
}

func strDeref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
