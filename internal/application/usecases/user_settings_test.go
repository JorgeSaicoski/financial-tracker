package usecases

import (
	"context"
	"testing"

	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
)

func TestGetUserSettingsDefaultsWhenUntouched(t *testing.T) {
	uc := NewGetUserSettings(newFakeUserSettingsRepo())

	s, err := uc.Execute(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if !s.LedgerSyncEntitled || !s.LedgerSyncEnabled || !s.CloudStorageEntitled {
		t.Errorf("untouched user should default all-true, got %+v", s)
	}
}

// TestCreateMovementUsesLocalStatusWhenSyncDisabled is BACK-13's core
// acceptance criterion: a movement created while the user's effective
// ledger sync is off never sits in "pending" — it starts "local".
func TestCreateMovementUsesLocalStatusWhenSyncDisabled(t *testing.T) {
	settings := newFakeUserSettingsRepo()
	movements := newFakeMovementRepo()
	createMovement := NewCreateMovement(movements, newFakeAccountRepo(), newFakeCardRepo(), newFakePaymentMethodRepo(), newFakePlanRepo(), settings)
	updateSettings := NewUpdateUserSettings(settings, movements)

	if _, err := updateSettings.Execute(context.Background(), "u1", false); err != nil {
		t.Fatal(err)
	}

	m, err := createMovement.Execute(context.Background(), CreateMovementInput{
		UserID: "u1", Currency: "usd", Amount: -100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.SyncStatus != string(entities.SyncStatusLocal) {
		t.Errorf("SyncStatus = %q, want local", m.SyncStatus)
	}

	// POST /sync equivalent: a movement sitting in "local" must never be
	// picked up by the sync loop's query.
	pending, err := movements.ListPendingSync(context.Background(), m.Timestamp.AddDate(0, 0, 1), 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Errorf("local movement must never appear in ListPendingSync, got %+v", pending)
	}
}

// TestDisableCreateEnableCyclePushesExactlyTheBacklog is BACK-13's
// off/on acceptance criterion: disable -> create movements -> enable ->
// the backlog created in between (and only that backlog) becomes
// syncable; anything already-synced before disabling is untouched.
func TestDisableCreateEnableCyclePushesExactlyTheBacklog(t *testing.T) {
	settings := newFakeUserSettingsRepo()
	movements := newFakeMovementRepo()
	createMovement := NewCreateMovement(movements, newFakeAccountRepo(), newFakeCardRepo(), newFakePaymentMethodRepo(), newFakePlanRepo(), settings)
	updateSettings := NewUpdateUserSettings(settings, movements)
	ctx := context.Background()

	// Something synced before any of this happened — must never be
	// touched by the cycle below.
	alreadySynced, err := createMovement.Execute(ctx, CreateMovementInput{UserID: "u1", Currency: "usd", Amount: -50})
	if err != nil {
		t.Fatal(err)
	}
	if err := movements.MarkSynced(ctx, alreadySynced.ID, "ledger-1", alreadySynced.Timestamp); err != nil {
		t.Fatal(err)
	}

	// Disable.
	if _, err := updateSettings.Execute(ctx, "u1", false); err != nil {
		t.Fatal(err)
	}

	// Create the backlog while disabled.
	backlog1, err := createMovement.Execute(ctx, CreateMovementInput{UserID: "u1", Currency: "usd", Amount: -10})
	if err != nil {
		t.Fatal(err)
	}
	backlog2, err := createMovement.Execute(ctx, CreateMovementInput{UserID: "u1", Currency: "usd", Amount: -20})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{backlog1.SyncStatus, backlog2.SyncStatus} {
		if m != string(entities.SyncStatusLocal) {
			t.Fatalf("backlog movement status = %q, want local while sync is off", m)
		}
	}

	// Re-enable: the backlog should be reclassified to pending.
	if _, err := updateSettings.Execute(ctx, "u1", true); err != nil {
		t.Fatal(err)
	}

	pending, err := movements.ListPendingSync(ctx, backlog2.Timestamp.AddDate(0, 0, 1), 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 {
		t.Fatalf("want exactly the 2-movement backlog pending after re-enabling, got %d: %+v", len(pending), pending)
	}
	for _, m := range pending {
		if m.ID == alreadySynced.ID {
			t.Error("already-synced movement must not be re-queued")
		}
	}
}

// TestEntitlementBlocksEffectiveSyncEvenWhenEnabled: an operator-revoked
// entitlement keeps movements "local" even if the user's own preference
// is still "enabled" — effective capability is entitled AND enabled.
func TestEntitlementBlocksEffectiveSyncEvenWhenEnabled(t *testing.T) {
	settings := newFakeUserSettingsRepo()
	settings.setEntitled("u1", false)
	movements := newFakeMovementRepo()
	createMovement := NewCreateMovement(movements, newFakeAccountRepo(), newFakeCardRepo(), newFakePaymentMethodRepo(), newFakePlanRepo(), settings)

	m, err := createMovement.Execute(context.Background(), CreateMovementInput{
		UserID: "u1", Currency: "usd", Amount: -100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.SyncStatus != string(entities.SyncStatusLocal) {
		t.Errorf("SyncStatus = %q, want local (not entitled, regardless of enabled)", m.SyncStatus)
	}
}
