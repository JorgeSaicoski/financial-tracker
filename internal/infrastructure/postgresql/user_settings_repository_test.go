package postgresql

import (
	"context"
	"testing"
	"time"
)

func TestUserSettingsGetDefaultsWhenNoRow(t *testing.T) {
	repo := NewUserSettingsRepository(openTestDB(t))
	ctx := context.Background()

	s, err := repo.Get(ctx, "no-such-user")
	if err != nil {
		t.Fatal(err)
	}
	if !s.LedgerSyncEntitled || !s.LedgerSyncEnabled || !s.CloudStorageEntitled {
		t.Errorf("absent row should default to all-true, got %+v", s)
	}
}

func TestUserSettingsUpdateEnabledUpsertsAndPreservesEntitlements(t *testing.T) {
	repo := NewUserSettingsRepository(openTestDB(t))
	ctx := context.Background()
	userID := "00000000-0000-0000-0000-000000000001"

	s, err := repo.UpdateEnabled(ctx, userID, false)
	if err != nil {
		t.Fatal(err)
	}
	if s.LedgerSyncEnabled {
		t.Error("ledger_sync_enabled should be false after disabling")
	}
	if !s.LedgerSyncEntitled || !s.CloudStorageEntitled {
		t.Errorf("entitlements should default true on first touch, got %+v", s)
	}

	s, err = repo.UpdateEnabled(ctx, userID, true)
	if err != nil {
		t.Fatal(err)
	}
	if !s.LedgerSyncEnabled {
		t.Error("ledger_sync_enabled should be true after re-enabling")
	}

	got, err := repo.Get(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.LedgerSyncEnabled {
		t.Error("Get after UpdateEnabled should reflect the new value")
	}
}

func TestUserSettingsListSyncDisabledUserIDs(t *testing.T) {
	repo := NewUserSettingsRepository(openTestDB(t))
	ctx := context.Background()

	if _, err := repo.UpdateEnabled(ctx, "enabled-user", true); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdateEnabled(ctx, "disabled-user", false); err != nil {
		t.Fatal(err)
	}

	disabled, err := repo.ListSyncDisabledUserIDs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(disabled) != 1 || disabled[0] != "disabled-user" {
		t.Fatalf("disabled = %v, want exactly [disabled-user]", disabled)
	}
}

func TestUserSettingsSetCloudStorageEntitledUpsertsAndPreservesOtherFields(t *testing.T) {
	repo := NewUserSettingsRepository(openTestDB(t))
	ctx := context.Background()
	userID := "00000000-0000-0000-0000-000000000002"

	s, err := repo.SetCloudStorageEntitled(ctx, userID, false)
	if err != nil {
		t.Fatal(err)
	}
	if s.CloudStorageEntitled {
		t.Error("cloud_storage_entitled should be false after SetCloudStorageEntitled(false)")
	}
	if !s.LedgerSyncEntitled || !s.LedgerSyncEnabled {
		t.Errorf("other fields should default true on first touch, got %+v", s)
	}

	if _, err := repo.UpdateEnabled(ctx, userID, false); err != nil {
		t.Fatal(err)
	}
	s, err = repo.SetCloudStorageEntitled(ctx, userID, true)
	if err != nil {
		t.Fatal(err)
	}
	if !s.CloudStorageEntitled {
		t.Error("cloud_storage_entitled should be true after SetCloudStorageEntitled(true)")
	}
	if s.LedgerSyncEnabled {
		t.Error("SetCloudStorageEntitled must not touch ledger_sync_enabled")
	}
}

// TestListPendingSyncExcludesUsers mirrors the SQLite package's test of
// the same name: BACK-13's acceptance criterion at the repository-query
// level.
func TestListPendingSyncExcludesUsers(t *testing.T) {
	repo := NewMovementRepository(openTestDB(t))
	ctx := context.Background()
	now := nowTruncated()

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

func TestMarkLocalPendingReclassifiesOnlyThatUsersLocalMovements(t *testing.T) {
	repo := NewMovementRepository(openTestDB(t))
	ctx := context.Background()

	local := testMovement(-100)
	local.SyncStatus = "local"
	local, _ = repo.Create(ctx, local)

	otherUserLocal := testMovement(-200)
	otherUserLocal.UserID = "99999999-9999-9999-9999-999999999999"
	otherUserLocal.SyncStatus = "local"
	otherUserLocal, _ = repo.Create(ctx, otherUserLocal)

	alreadySynced := testMovement(-300)
	alreadySynced, _ = repo.Create(ctx, alreadySynced)
	if err := repo.MarkSynced(ctx, alreadySynced.ID, "ledger-1", nowTruncated()); err != nil {
		t.Fatal(err)
	}

	if err := repo.MarkLocalPending(ctx, local.UserID); err != nil {
		t.Fatal(err)
	}

	got, _ := repo.GetByID(ctx, local.ID)
	if got.SyncStatus != "pending" {
		t.Errorf("local.SyncStatus = %q, want pending", got.SyncStatus)
	}
	got, _ = repo.GetByID(ctx, otherUserLocal.ID)
	if got.SyncStatus != "local" {
		t.Errorf("other user's local movement must stay untouched, got %q", got.SyncStatus)
	}
	got, _ = repo.GetByID(ctx, alreadySynced.ID)
	if got.SyncStatus != "synced" {
		t.Errorf("already-synced movement must stay untouched, got %q", got.SyncStatus)
	}

	if err := repo.MarkLocalPending(ctx, local.UserID); err != nil {
		t.Errorf("second call should be a no-op, got %v", err)
	}
}
