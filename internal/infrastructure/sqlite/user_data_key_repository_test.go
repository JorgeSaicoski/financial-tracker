package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

func TestUserDataKeyGetReturnsNotFoundWhenAbsent(t *testing.T) {
	repo := NewUserDataKeyRepository(openTestDB(t))
	ctx := context.Background()

	if _, err := repo.Get(ctx, "no-such-user"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestUserDataKeyCreateAndGetRoundtrip(t *testing.T) {
	repo := NewUserDataKeyRepository(openTestDB(t))
	ctx := context.Background()

	created, err := repo.Create(ctx, &dto.UserDataKeyDTO{
		UserID: "user-1", WrappedKey: "wrapped-bytes-b64", CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.WrappedKey != "wrapped-bytes-b64" {
		t.Errorf("Create returned WrappedKey = %q", created.WrappedKey)
	}

	got, err := repo.Get(ctx, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.WrappedKey != "wrapped-bytes-b64" {
		t.Errorf("Get WrappedKey = %q, want %q", got.WrappedKey, "wrapped-bytes-b64")
	}
}

func TestUserDataKeyCreateIsRaceSafe(t *testing.T) {
	repo := NewUserDataKeyRepository(openTestDB(t))
	ctx := context.Background()

	first, err := repo.Create(ctx, &dto.UserDataKeyDTO{
		UserID: "user-1", WrappedKey: "first-key", CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	second, err := repo.Create(ctx, &dto.UserDataKeyDTO{
		UserID: "user-1", WrappedKey: "second-key", CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.WrappedKey != first.WrappedKey {
		t.Errorf("second Create must return the existing row (%q), got %q", first.WrappedKey, second.WrappedKey)
	}
}
