package usecases

import (
	"context"
	"testing"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
)

func boolPtr(b bool) *bool { return &b }

func TestGetUserSettingsDefaultsWhenUntouched(t *testing.T) {
	uc := NewGetUserSettings(newFakeUserSettingsRepo(), newFakeSubscriptionRepo())

	s, err := uc.Execute(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if !s.LedgerSyncEntitled || !s.LedgerSyncEnabled || !s.CloudStorageEntitled {
		t.Errorf("untouched user should default all-true, got %+v", s)
	}
	if s.SubscriptionStatus != "" || s.SubscriptionCurrentPeriodEnd != nil {
		t.Errorf("a user who never subscribed should have no subscription fields, got %+v", s)
	}
}

// TestGetUserSettingsSurfacesSubscriptionFields is BACK-19's "GET
// /settings gains subscription fields" requirement.
func TestGetUserSettingsSurfacesSubscriptionFields(t *testing.T) {
	settings := newFakeUserSettingsRepo()
	subs := newFakeSubscriptionRepo()
	periodEnd := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := subs.Upsert(context.Background(), &dto.SubscriptionDTO{
		UserID: "u1", Provider: "stripe", ProviderSubscriptionID: "sub_1",
		Status: dto.SubscriptionStatusActive, CurrentPeriodEnd: periodEnd,
	}); err != nil {
		t.Fatal(err)
	}

	uc := NewGetUserSettings(settings, subs)
	s, err := uc.Execute(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if s.SubscriptionStatus != dto.SubscriptionStatusActive {
		t.Errorf("SubscriptionStatus = %q, want active", s.SubscriptionStatus)
	}
	if s.SubscriptionCurrentPeriodEnd == nil || !s.SubscriptionCurrentPeriodEnd.Equal(periodEnd) {
		t.Errorf("SubscriptionCurrentPeriodEnd = %v, want %v", s.SubscriptionCurrentPeriodEnd, periodEnd)
	}
}

// TestCreateMovementUsesLocalStatusWhenSyncDisabled is BACK-13's core
// acceptance criterion: a movement created while the user's effective
// ledger sync is off never sits in "pending" — it starts "local".
func TestCreateMovementUsesLocalStatusWhenSyncDisabled(t *testing.T) {
	settings := newFakeUserSettingsRepo()
	movements := newFakeMovementRepo()
createMovement := NewCreateMovement(movements, newFakeAccountRepo(), newFakePaymentMethodRepo(), newFakePlanRepo(), newFakeCategoryRepo(), settings)
	updateSettings := NewUpdateUserSettings(settings, movements, newFakeCategoryRepo())

	if _, err := updateSettings.Execute(context.Background(), "u1", UpdateUserSettingsInput{LedgerSyncEnabled: boolPtr(false)}); err != nil {
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
createMovement := NewCreateMovement(movements, newFakeAccountRepo(), newFakePaymentMethodRepo(), newFakePlanRepo(), newFakeCategoryRepo(), settings)
	updateSettings := NewUpdateUserSettings(settings, movements, newFakeCategoryRepo())
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
	if _, err := updateSettings.Execute(ctx, "u1", UpdateUserSettingsInput{LedgerSyncEnabled: boolPtr(false)}); err != nil {
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
	if _, err := updateSettings.Execute(ctx, "u1", UpdateUserSettingsInput{LedgerSyncEnabled: boolPtr(true)}); err != nil {
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
createMovement := NewCreateMovement(movements, newFakeAccountRepo(), newFakePaymentMethodRepo(), newFakePlanRepo(), newFakeCategoryRepo(), settings)

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
