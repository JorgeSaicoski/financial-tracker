package postgresql

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/id"
)

// openTestDB connects to TEST_DATABASE_URL, applies migrations, and wipes
// the tables these tests touch so each test starts from a clean slate —
// mirroring infrastructure/sqlite/repository_test.go's fresh-temp-file
// isolation, but against a real, shared Postgres instance. Tests are
// skipped (not failed) when TEST_DATABASE_URL isn't set, so `go test ./...`
// still passes offline.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres repository tests")
	}

	db, err := Open(url, PoolConfig{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := db.Exec(`TRUNCATE TABLE account_snapshots, movements, credit_card_purchases, accounts CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return db
}

// nowTruncated is truncated to microseconds because Postgres's timestamptz
// — unlike SQLite's fixed-width-nanosecond TEXT storage — only keeps
// microsecond precision, and read-back == created checks would otherwise
// flake on the dropped low-order nanoseconds.
func nowTruncated() time.Time {
	return time.Now().UTC().Truncate(time.Microsecond)
}

func testMovement(amount int64) *dto.MovementDTO {
	now := nowTruncated()
	return &dto.MovementDTO{
		UserID:        "00000000-0000-0000-0000-000000000001",
		Amount:        amount,
		Currency:      "usd",
		Description:   "coffee",
		Category:      string(entities.CategoryFood),
		PaymentMethod: string(entities.PaymentMethodCash),
		Status:        string(entities.MovementStatusActive),
		SyncStatus:    string(entities.SyncStatusPending),
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
		got.Category != string(entities.CategoryFood) || got.PaymentMethod != string(entities.PaymentMethodCash) ||
		got.Status != string(entities.MovementStatusActive) || got.SyncStatus != string(entities.SyncStatusPending) {
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
	now := nowTruncated()

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
	if got.SyncStatus != string(entities.SyncStatusFailed) || got.SyncAttempts != 1 ||
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

	makeReversal := func() *dto.MovementDTO {
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

// TestCreateReversalCleansUpOrphanWhenCallerSwallowsConflict targets the
// specific gap createReversalTx's post-conflict DELETE closes: a top-level
// CreateReversal call always rolls back its own self-contained transaction
// on any error, so a plain conflict alone never leaves an orphan row
// regardless of the DELETE — that only matters when CreateReversal runs
// inside a caller's own Transact and that caller treats "already reversed"
// as a handled, non-fatal case (returning nil instead of propagating the
// conflict), letting the transaction commit anyway.
//
// Reproducing that precisely needs the loser's reversal row to already be
// inserted by the time its link UPDATE loses the race — which a plain
// goroutine race can't guarantee happens on every run (the loser might
// instead observe the original as already reversed before ever inserting,
// same as TestCreateReversalLinksAtomically's sequential second call, which
// exercises a different, uninteresting path here). So this test drives the
// interleaving directly with two raw transactions and createReversalTx
// (the unexported function movementRepositoryTx.CreateReversal itself
// calls) instead of relying on goroutine scheduling: the loser's link
// UPDATE is made to block on the winner's row lock, and only released
// (via the winner's commit) once the loser is guaranteed to be sitting on
// its own already-inserted, not-yet-linked reversal row.
func TestCreateReversalCleansUpOrphanWhenCallerSwallowsConflict(t *testing.T) {
	db := openTestDB(t)
	repo := NewMovementRepository(db)
	ctx := context.Background()

	original, err := repo.Create(ctx, testMovement(-450))
	if err != nil {
		t.Fatal(err)
	}

	txWinner, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	winner := testMovement(450)
	winner.ID = id.NewUUID() // createReversalTx doesn't assign one; callers normally do (see movementRepositoryTx.CreateReversal)
	winner.CancelsMovementID = &original.ID
	if err := createReversalTx(ctx, txWinner, winner); err != nil {
		t.Fatalf("winner reversal failed: %v", err)
	}
	// txWinner is deliberately left open (not committed) here: the loser's
	// SELECT below must still see the original as unreversed, and its
	// UPDATE must still find the row unlocked-by-value-but-lockable, for
	// this to reproduce the real race window.

	txLoser, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	loser := testMovement(450)
	loser.ID = id.NewUUID()
	loser.CancelsMovementID = &original.ID

	loserErrCh := make(chan error, 1)
	go func() {
		// createReversalTx's link UPDATE blocks here on txWinner's row
		// lock until the commit below releases it — by which point
		// loser's reversal row has already been inserted (uncommitted,
		// but present in txLoser), exactly reproducing "insert landed,
		// link lost the race."
		loserErrCh <- createReversalTx(ctx, txLoser, loser)
	}()

	// Generous margin for the goroutine to reach the blocking UPDATE
	// before the lock it's waiting on is released.
	time.Sleep(200 * time.Millisecond)
	if err := txWinner.Commit(); err != nil {
		t.Fatalf("commit winner: %v", err)
	}

	loserErr := <-loserErrCh
	if !errors.Is(loserErr, apperrors.ErrConflict) {
		t.Fatalf("loser: want ErrConflict, got %v", loserErr)
	}

	// Simulate a caller that treats the conflict as handled and commits
	// the transaction anyway, instead of propagating the error and
	// rolling back — the scenario createReversalTx's own cleanup exists
	// for, since nothing else will undo the insert in this case.
	if err := txLoser.Commit(); err != nil {
		t.Fatalf("commit loser: %v", err)
	}

	if _, err := repo.GetByID(ctx, loser.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("loser's reversal row must have been cleaned up even though its transaction committed, got %v", err)
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

	account, err := NewAccountRepository(db).Create(ctx, &dto.AccountDTO{
		UserID: created.UserID, Name: "wallet", Type: string(entities.AccountTypeCash),
		Currency: "usd", CreatedAt: nowTruncated(),
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.UpdateMetadata(ctx, created.ID, "renamed", string(entities.CategoryTransport), string(entities.PaymentMethodPix), &account.ID); err != nil {
		t.Fatalf("update metadata: %v", err)
	}
	got, err := repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != "renamed" || got.Category != string(entities.CategoryTransport) ||
		got.PaymentMethod != string(entities.PaymentMethodPix) || got.AccountID == nil || *got.AccountID != account.ID {
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

	if err := repo.UpdateMetadata(ctx, "missing", "x", string(entities.CategoryOther), string(entities.PaymentMethodOther), nil); !errors.Is(err, apperrors.ErrNotFound) {
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

	created, err := repo.CreateBatch(ctx, []*dto.MovementDTO{debit, credit})
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

	if _, err := repo.CreateBatch(ctx, []*dto.MovementDTO{firstOfSecondBatch, secondOfSecondBatch}); err == nil {
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
	now := nowTruncated()

	purchase := &dto.CreditCardPurchaseDTO{
		UserID:           "00000000-0000-0000-0000-000000000001",
		Description:      "tv",
		Category:         string(entities.CategoryShopping),
		TotalAmount:      -900,
		Currency:         "usd",
		InstallmentCount: 3,
		PurchaseDate:     now,
		Status:           string(entities.CreditCardPurchaseStatusActive),
		CreatedAt:        now,
	}
	var installments []*dto.MovementDTO
	for i := 0; i < 3; i++ {
		m := testMovement(-300)
		m.PaymentMethod = string(entities.PaymentMethodCreditCard)
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
	if got.TotalAmount != -900 || got.InstallmentCount != 3 || got.Status != string(entities.CreditCardPurchaseStatusActive) {
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
	if got.Status != string(entities.CreditCardPurchaseStatusCancelled) {
		t.Error("purchase not cancelled")
	}
	if err := purchases.MarkCancelled(ctx, "missing"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("cancel missing purchase: want ErrNotFound, got %v", err)
	}
}

// TestCurrencyRepositoryListAndAdd doesn't use openTestDB's truncation —
// migrations/postgres/003 seeds usd/brl once and never re-runs, so the
// currencies table (unlike the others) accumulates across the whole test
// binary. The code used here is chosen to be distinct from any other
// test's fixtures and is checked by exact count, not by asserting the
// list's total size.
func TestCurrencyRepositoryListAndAdd(t *testing.T) {
	repo := NewCurrencyRepository(openTestDB(t))
	ctx := context.Background()

	seeded, err := repo.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !containsString(seeded, "usd") || !containsString(seeded, "brl") {
		t.Fatalf("expected seeded usd/brl currencies, got %v", seeded)
	}

	const code = "zzz_test_currency"
	if err := repo.Add(ctx, code); err != nil {
		t.Fatalf("add: %v", err)
	}
	// Adding an existing code is a no-op, not an error.
	if err := repo.Add(ctx, code); err != nil {
		t.Fatalf("add duplicate: %v", err)
	}

	all, err := repo.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, c := range all {
		if c == code {
			count++
		}
	}
	if count != 1 {
		t.Errorf("want exactly one %q in currency list, got %d (list: %v)", code, count, all)
	}
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func TestAccountRepositoryListByUser(t *testing.T) {
	repo := NewAccountRepository(openTestDB(t))
	ctx := context.Background()
	userID := "00000000-0000-0000-0000-000000000001"
	otherUserID := "00000000-0000-0000-0000-000000000002"

	// Inserted out of alphabetical order to verify ListByUser sorts by name.
	for _, name := range []string{"savings", "wallet", "brokerage"} {
		_, err := repo.Create(ctx, &dto.AccountDTO{
			UserID: userID, Name: name, Type: string(entities.AccountTypeBank),
			Currency: "usd", CreatedAt: nowTruncated(),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	// A different user's account must never show up in userID's list.
	if _, err := repo.Create(ctx, &dto.AccountDTO{
		UserID: otherUserID, Name: "other-account", Type: string(entities.AccountTypeCash),
		Currency: "usd", CreatedAt: nowTruncated(),
	}); err != nil {
		t.Fatal(err)
	}

	got, err := repo.ListByUser(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("listed %d accounts, want 3", len(got))
	}
	if got[0].Name != "brokerage" || got[1].Name != "savings" || got[2].Name != "wallet" {
		t.Errorf("ListByUser not alphabetical: %s, %s, %s", got[0].Name, got[1].Name, got[2].Name)
	}
	for _, a := range got {
		if a.UserID != userID {
			t.Errorf("ListByUser leaked another user's account: %+v", a)
		}
	}
}

func TestAccountSnapshotsRoundtrip(t *testing.T) {
	db := openTestDB(t)
	repo := NewAccountRepository(db)
	ctx := context.Background()
	now := nowTruncated()

	account, err := repo.Create(ctx, &dto.AccountDTO{
		UserID: "00000000-0000-0000-0000-000000000001", Name: "wallet",
		Type: string(entities.AccountTypeCash), Currency: "usd", CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}

	for i, balance := range []int64{1000, 1500, 2000} {
		snap := &dto.AccountSnapshotDTO{
			AccountID: account.ID, Balance: balance,
			Timestamp: now.Add(time.Duration(i) * time.Hour), CreatedAt: now,
		}
		created, err := repo.AddSnapshot(ctx, snap)
		if err != nil {
			t.Fatalf("add snapshot %d: %v", i, err)
		}
		if created.ID == "" {
			t.Fatal("no snapshot id generated")
		}
	}

	latest, err := repo.LatestSnapshots(ctx, account.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(latest) != 2 {
		t.Fatalf("want 2 latest snapshots, got %d", len(latest))
	}
	// Newest first.
	if latest[0].Balance != 2000 || latest[1].Balance != 1500 {
		t.Errorf("wrong order/values: %d, %d", latest[0].Balance, latest[1].Balance)
	}

	all, err := repo.LatestSnapshots(ctx, account.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("want all 3 snapshots when n exceeds count, got %d", len(all))
	}
}
