package sqlite

import (
	"context"
	"testing"
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

	// First touch: creates the row lazily.
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

	// Re-enable: only ledger_sync_enabled changes.
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
	// A user with no row at all (all-true default) must not show up as
	// disabled.
	// (no row created for "untouched-user")

	disabled, err := repo.ListSyncDisabledUserIDs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(disabled) != 1 || disabled[0] != "disabled-user" {
		t.Fatalf("disabled = %v, want exactly [disabled-user]", disabled)
	}
}
