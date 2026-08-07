package usecases

import (
	"context"
	"testing"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
)

// TestEnsureUserGrandfathersExistingUsers guards BACK-19's acceptance
// criterion: a user who already existed before paid cloud storage
// shipped must keep cloud_storage_entitled=true (the settings
// repository's implicit "no row" default) rather than being silently
// switched to the new-signup default of false just because they log in
// again after this shipped.
func TestEnsureUserGrandfathersExistingUsers(t *testing.T) {
	users := newFakeUserRepo()
	settings := newFakeUserSettingsRepo()
	ctx := context.Background()

	// Simulate a user who already existed (e.g. via BACK-02, before this
	// ticket shipped) by seeding the user row directly, bypassing
	// EnsureUser entirely.
	existingID := "already-existed"
	if _, err := users.Upsert(ctx, &dto.UserDTO{ID: existingID, Provider: "authentik"}); err != nil {
		t.Fatal(err)
	}

	uc := NewEnsureUser(users, settings)
	if _, err := uc.Execute(ctx, EnsureUserInput{UserID: existingID, Provider: "authentik"}); err != nil {
		t.Fatal(err)
	}

	s, err := settings.Get(ctx, existingID)
	if err != nil {
		t.Fatal(err)
	}
	if !s.CloudStorageEntitled {
		t.Error("a pre-existing user must keep cloud_storage_entitled=true after logging in again, got false")
	}
}

// TestEnsureUserDefaultsNewSignupsToCloudStorageNotEntitled is the other
// half of the same acceptance criterion: a genuinely new signup (never
// seen before) defaults to cloud_storage_entitled=false.
func TestEnsureUserDefaultsNewSignupsToCloudStorageNotEntitled(t *testing.T) {
	users := newFakeUserRepo()
	settings := newFakeUserSettingsRepo()
	uc := NewEnsureUser(users, settings)
	ctx := context.Background()

	newID := "brand-new-signup"
	if _, err := uc.Execute(ctx, EnsureUserInput{UserID: newID, Provider: "authentik"}); err != nil {
		t.Fatal(err)
	}

	s, err := settings.Get(ctx, newID)
	if err != nil {
		t.Fatal(err)
	}
	if s.CloudStorageEntitled {
		t.Error("a brand-new signup should default to cloud_storage_entitled=false, got true")
	}
	if !s.LedgerSyncEntitled || !s.LedgerSyncEnabled {
		t.Errorf("only cloud_storage_entitled should be affected, got %+v", s)
	}
}

// TestEnsureUserSecondLoginDoesNotResetEntitlement guards against a
// subtler bug: a new signup's *second* login must not re-run the
// "existed" check against a stale view and re-flip an entitlement the
// user has since purchased.
func TestEnsureUserSecondLoginDoesNotResetEntitlement(t *testing.T) {
	users := newFakeUserRepo()
	settings := newFakeUserSettingsRepo()
	uc := NewEnsureUser(users, settings)
	ctx := context.Background()

	userID := "subscribes-then-logs-in-again"
	if _, err := uc.Execute(ctx, EnsureUserInput{UserID: userID}); err != nil {
		t.Fatal(err)
	}
	// Simulate a successful subscription webhook flipping entitlement on.
	if _, err := settings.SetCloudStorageEntitled(ctx, userID, true); err != nil {
		t.Fatal(err)
	}

	if _, err := uc.Execute(ctx, EnsureUserInput{UserID: userID}); err != nil {
		t.Fatal(err)
	}

	s, err := settings.Get(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if !s.CloudStorageEntitled {
		t.Error("a second EnsureUser call must not reset an already-flipped entitlement")
	}
}
