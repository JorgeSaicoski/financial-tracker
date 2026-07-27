package usecases

import (
	"context"
	"errors"
	"testing"

	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

func TestLocalArchiveSettingDefaultsFalse(t *testing.T) {
	get := NewGetLocalArchiveSetting(newFakeLocalArchiveSettingsRepo())

	enabled, err := get.Execute(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if enabled {
		t.Error("a user who never toggled it should default to disabled")
	}
}

func TestLocalArchiveSettingSetAndGetRoundtrip(t *testing.T) {
	repo := newFakeLocalArchiveSettingsRepo()
	set := NewSetLocalArchiveSetting(repo)
	get := NewGetLocalArchiveSetting(repo)
	ctx := context.Background()

	if _, err := set.Execute(ctx, "user-1", true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	enabled, err := get.Execute(ctx, "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !enabled {
		t.Error("setting was not persisted")
	}

	// A different user's setting is untouched.
	other, err := get.Execute(ctx, "user-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if other {
		t.Error("setting leaked across users")
	}
}

func TestLocalArchiveSettingRejectsEmptyUserID(t *testing.T) {
	repo := newFakeLocalArchiveSettingsRepo()
	get := NewGetLocalArchiveSetting(repo)
	set := NewSetLocalArchiveSetting(repo)

	if _, err := get.Execute(context.Background(), ""); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Errorf("get: want ErrInvalidInput, got %v", err)
	}
	if _, err := set.Execute(context.Background(), "", true); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Errorf("set: want ErrInvalidInput, got %v", err)
	}
}
