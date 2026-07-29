package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
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

// testMovement leaves CategoryID nil (uncategorized) since most callers
// don't care about it — category_id is a real foreign key (BACK-14
// follow-up: categories are globally shared, not per-user), so a test
// that does care must create the category first via
// NewCategoryRepository(db).Create and set .CategoryID to its id, the
// same way callers already create an account before setting .AccountID.
func testMovement(amount int64) *dto.MovementDTO {
	now := time.Now().UTC()
	return &dto.MovementDTO{
		UserID:        "00000000-0000-0000-0000-000000000001",
		Amount:        amount,
		Currency:      "usd",
		Description:   "coffee",
		PaymentMethod: "cash",
		Status:        string(entities.MovementStatusActive),
		SyncStatus:    string(entities.SyncStatusPending),
		Timestamp:     now,
		CreatedAt:     now,
	}
}

func TestMovementCreateGetRoundtrip(t *testing.T) {
	db := openTestDB(t)
	repo := NewMovementRepository(db)
	ctx := context.Background()

	food, err := NewCategoryRepository(db).Create(ctx, &dto.CategoryDTO{Name: "food", ContributorIDs: []string{"00000000-0000-0000-0000-000000000001"}})
	if err != nil {
		t.Fatal(err)
	}
	m := testMovement(-450)
	m.CategoryID = &food.ID
	created, err := repo.Create(ctx, m)
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
		got.Category != "food" || got.PaymentMethod != "cash" ||
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

	pending, err := repo.ListPendingSync(ctx, now, time.Minute, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].ID != due.ID {
		t.Fatalf("cooldown pass: want only %s, got %d rows", due.ID, len(pending))
	}

	// Zero cooldown (manual sync) also picks up the fresh failure — but
	// never the future, synced, or voided rows.
	pending, err = repo.ListPendingSync(ctx, now, 0, nil)
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

// TestListPendingSyncExcludesUsers is BACK-13's acceptance criterion at
// the repository-query level: two users, one excluded — the query
// returns only the other one's pending movement, even though both rows
// are otherwise identical and due.
func TestListPendingSyncExcludesUsers(t *testing.T) {
	repo := NewMovementRepository(openTestDB(t))
	ctx := context.Background()
	now := time.Now().UTC()

	enabled := testMovement(-100)
	enabled.UserID = "11111111-1111-1111-1111-111111111111"
	enabled.Timestamp = now.Add(-time.Hour)
	enabled, _ = repo.Create(ctx, enabled)

	disabled := testMovement(-200)
	disabled.UserID = "22222222-2222-2222-2222-222222222222"
	disabled.Timestamp = now.Add(-time.Hour)
	disabled, _ = repo.Create(ctx, disabled)

	pending, err := repo.ListPendingSync(ctx, now, 0, []string{disabled.UserID})
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].ID != enabled.ID {
		t.Fatalf("want only %s (the non-excluded user), got %d rows: %+v", enabled.ID, len(pending), pending)
	}
}

// TestMarkLocalPendingReclassifiesOnlyThatUsersLocalMovements is BACK-13's
// "re-enable" path: only the target user's "local" rows flip to
// "pending"; another user's local row and this user's already-synced row
// are left untouched.
func TestMarkLocalPendingReclassifiesOnlyThatUsersLocalMovements(t *testing.T) {
	repo := NewMovementRepository(openTestDB(t))
	ctx := context.Background()

	local := testMovement(-100)
	local.SyncStatus = string(entities.SyncStatusLocal)
	local, _ = repo.Create(ctx, local)

	otherUserLocal := testMovement(-200)
	otherUserLocal.UserID = "99999999-9999-9999-9999-999999999999"
	otherUserLocal.SyncStatus = string(entities.SyncStatusLocal)
	otherUserLocal, _ = repo.Create(ctx, otherUserLocal)

	alreadySynced := testMovement(-300)
	alreadySynced, _ = repo.Create(ctx, alreadySynced)
	if err := repo.MarkSynced(ctx, alreadySynced.ID, "ledger-1", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	if err := repo.MarkLocalPending(ctx, local.UserID); err != nil {
		t.Fatal(err)
	}

	got, _ := repo.GetByID(ctx, local.ID)
	if got.SyncStatus != string(entities.SyncStatusPending) {
		t.Errorf("local.SyncStatus = %q, want pending", got.SyncStatus)
	}
	got, _ = repo.GetByID(ctx, otherUserLocal.ID)
	if got.SyncStatus != string(entities.SyncStatusLocal) {
		t.Errorf("other user's local movement must stay untouched, got %q", got.SyncStatus)
	}
	got, _ = repo.GetByID(ctx, alreadySynced.ID)
	if got.SyncStatus != string(entities.SyncStatusSynced) {
		t.Errorf("already-synced movement must stay untouched, got %q", got.SyncStatus)
	}

	// Calling it again with nothing left to reclassify is a normal no-op,
	// not an error.
	if err := repo.MarkLocalPending(ctx, local.UserID); err != nil {
		t.Errorf("second call should be a no-op, got %v", err)
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

func TestMovementUpdateMetadataAndFinancial(t *testing.T) {
	db := openTestDB(t)
	repo := NewMovementRepository(db)
	ctx := context.Background()

	created, err := repo.Create(ctx, testMovement(-450))
	if err != nil {
		t.Fatal(err)
	}
	transport, err := NewCategoryRepository(db).Create(ctx, &dto.CategoryDTO{Name: "transport", ContributorIDs: []string{created.UserID}})
	if err != nil {
		t.Fatal(err)
	}

	account, err := NewAccountRepository(db).Create(ctx, &dto.AccountDTO{
		UserID: created.UserID, Name: "wallet", Type: string(entities.AccountTypeCash),
		Currency: "usd", CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.UpdateMetadata(ctx, created.ID, "renamed", &transport.ID, "pix", &account.ID); err != nil {
		t.Fatalf("update metadata: %v", err)
	}
	got, err := repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != "renamed" || got.Category != "transport" ||
		got.PaymentMethod != "pix" || got.AccountID == nil || *got.AccountID != account.ID {
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

	if err := repo.UpdateMetadata(ctx, "missing", "x", nil, "other", nil); !errors.Is(err, apperrors.ErrNotFound) {
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
	now := time.Now().UTC()

	shopping, err := NewCategoryRepository(db).Create(ctx, &dto.CategoryDTO{Name: "shopping", ContributorIDs: []string{"00000000-0000-0000-0000-000000000001"}})
	if err != nil {
		t.Fatal(err)
	}

	purchase := &dto.CreditCardPurchaseDTO{
		UserID:           "00000000-0000-0000-0000-000000000001",
		Description:      "tv",
		CategoryID:       &shopping.ID,
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

	purchase, _, err = purchases.CreateWithInstallments(ctx, purchase, installments)
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

// testRecurringRule leaves CategoryID nil (uncategorized), same reasoning
// as testMovement — a test that cares about category_id creates one
// first via NewCategoryRepository(db).Create.
func testRecurringRule(dayOfMonth string) *dto.RecurringRuleDTO {
	now := time.Now().UTC()
	return &dto.RecurringRuleDTO{
		UserID:        "00000000-0000-0000-0000-000000000001",
		Amount:        -5000,
		Currency:      "usd",
		Description:   "rent",
		PaymentMethod: string(entities.PaymentMethodBankTransfer),
		DayOfMonth:    dayOfMonth,
		StartsAt:      now,
		Active:        true,
		CreatedAt:     now,
	}
}

func TestRecurringRuleCreateGetRoundtrip(t *testing.T) {
	repo := NewRecurringRuleRepository(openTestDB(t))
	ctx := context.Background()

	created, err := repo.Create(ctx, testRecurringRule("1"))
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
	if got.Amount != -5000 || got.Description != "rent" || got.DayOfMonth != "1" || !got.Active {
		t.Errorf("roundtrip mismatch: %+v", got)
	}
	if got.LastGeneratedAt != nil || got.EndsAt != nil || got.AccountID != nil {
		t.Error("nullable fields should be nil")
	}

	if _, err := repo.GetByID(ctx, "missing"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("missing id: want ErrNotFound, got %v", err)
	}
}

func TestRecurringRuleListByUserAndActive(t *testing.T) {
	repo := NewRecurringRuleRepository(openTestDB(t))
	ctx := context.Background()

	mine, _ := repo.Create(ctx, testRecurringRule("1"))
	inactive := testRecurringRule("15")
	inactive.Active = false
	inactiveRule, _ := repo.Create(ctx, inactive)
	someoneElses := testRecurringRule("1")
	someoneElses.UserID = "00000000-0000-0000-0000-000000000002"
	repo.Create(ctx, someoneElses)

	byUser, err := repo.ListByUser(ctx, "00000000-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	if len(byUser) != 2 {
		t.Fatalf("ListByUser = %d rows, want 2", len(byUser))
	}

	active, err := repo.ListActive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range active {
		if r.ID == inactiveRule.ID {
			t.Error("ListActive returned a deactivated rule")
		}
	}
	found := false
	for _, r := range active {
		if r.ID == mine.ID {
			found = true
		}
	}
	if !found {
		t.Error("ListActive did not return the active rule")
	}
}

func TestRecurringRuleUpdatesAndSetActive(t *testing.T) {
	db := openTestDB(t)
	repo := NewRecurringRuleRepository(db)
	ctx := context.Background()

	rule, _ := repo.Create(ctx, testRecurringRule("1"))
	food, err := NewCategoryRepository(db).Create(ctx, &dto.CategoryDTO{Name: "food", ContributorIDs: []string{"00000000-0000-0000-0000-000000000001"}})
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.UpdateMetadata(ctx, rule.ID, "new desc", &food.ID, "cash", nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateFinancial(ctx, rule.ID, -9999, "brl"); err != nil {
		t.Fatal(err)
	}
	ends := time.Now().UTC().AddDate(1, 0, 0)
	if err := repo.UpdateSchedule(ctx, rule.ID, "last", &ends); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetActive(ctx, rule.ID, false); err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetByID(ctx, rule.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != "new desc" || got.Category != "food" ||
		got.Amount != -9999 || got.Currency != "brl" || got.DayOfMonth != "last" ||
		got.EndsAt == nil || got.Active {
		t.Errorf("updates did not apply: %+v", got)
	}

	if err := repo.SetActive(ctx, "missing", true); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("update missing rule: want ErrNotFound, got %v", err)
	}
}

func TestRecurringRuleGenerateAndAdvance(t *testing.T) {
	db := openTestDB(t)
	repo := NewRecurringRuleRepository(db)
	movements := NewMovementRepository(db)
	ctx := context.Background()

	rule, _ := repo.Create(ctx, testRecurringRule("1"))

	m := testMovement(-5000)
	m.RecurringRuleID = &rule.ID
	watermark := time.Now().UTC()

	generated, err := repo.GenerateAndAdvance(ctx, rule.ID, []*dto.MovementDTO{m}, watermark)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(generated) != 1 || generated[0].ID == "" {
		t.Fatalf("generated = %+v, want one movement with an id", generated)
	}

	stored, err := movements.GetByID(ctx, generated[0].ID)
	if err != nil {
		t.Fatalf("movement not persisted: %v", err)
	}
	if stored.RecurringRuleID == nil || *stored.RecurringRuleID != rule.ID {
		t.Errorf("movement missing recurring_rule_id link: %+v", stored)
	}

	got, err := repo.GetByID(ctx, rule.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastGeneratedAt == nil || !got.LastGeneratedAt.Equal(watermark) {
		t.Errorf("watermark not advanced: %+v", got.LastGeneratedAt)
	}

	if _, err := repo.GenerateAndAdvance(ctx, "missing", nil, watermark); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("generate for missing rule: want ErrNotFound, got %v", err)
	}
}

func TestPurchaseListByUser(t *testing.T) {
	db := openTestDB(t)
	purchases := NewCreditCardPurchaseRepository(db)
	ctx := context.Background()
	now := time.Now().UTC()

	categories := NewCategoryRepository(db)
	shopping, err := categories.Create(ctx, &dto.CategoryDTO{Name: "shopping", ContributorIDs: []string{"00000000-0000-0000-0000-000000000001"}})
	if err != nil {
		t.Fatal(err)
	}

	mine := &dto.CreditCardPurchaseDTO{
		UserID: "00000000-0000-0000-0000-000000000001", CategoryID: &shopping.ID,
		TotalAmount: -900, Currency: "usd", InstallmentCount: 1, PurchaseDate: now, Status: string(entities.CreditCardPurchaseStatusActive), CreatedAt: now,
	}
	someoneElses := &dto.CreditCardPurchaseDTO{
		// Categories are globally shared (BACK-14 follow-up): a
		// different user referencing the same category_id is expected,
		// not an error.
		UserID: "00000000-0000-0000-0000-000000000002", CategoryID: &shopping.ID,
		TotalAmount: -100, Currency: "usd", InstallmentCount: 1, PurchaseDate: now, Status: string(entities.CreditCardPurchaseStatusActive), CreatedAt: now,
	}
	if _, _, err := purchases.CreateWithInstallments(ctx, mine, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := purchases.CreateWithInstallments(ctx, someoneElses, nil); err != nil {
		t.Fatal(err)
	}

	got, err := purchases.ListByUser(ctx, "00000000-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != mine.ID {
		t.Errorf("ListByUser = %+v, want exactly the one purchase belonging to that user", got)
	}
}

func TestLocalArchiveSettingsRepository(t *testing.T) {
	db := openTestDB(t)
	repo := NewLocalArchiveSettingsRepository(db)
	ctx := context.Background()

	enabled, err := repo.IsEnabled(ctx, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if enabled {
		t.Error("a user with no row yet should default to disabled")
	}

	if err := repo.SetEnabled(ctx, "user-1", true); err != nil {
		t.Fatal(err)
	}
	enabled, err = repo.IsEnabled(ctx, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if !enabled {
		t.Error("setting was not persisted")
	}

	// Setting it again (upsert) doesn't error and reflects the new value.
	if err := repo.SetEnabled(ctx, "user-1", false); err != nil {
		t.Fatal(err)
	}
	enabled, err = repo.IsEnabled(ctx, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if enabled {
		t.Error("upsert did not overwrite the previous value")
	}

	other, err := repo.IsEnabled(ctx, "user-2")
	if err != nil {
		t.Fatal(err)
	}
	if other {
		t.Error("setting leaked across users")
	}
}
