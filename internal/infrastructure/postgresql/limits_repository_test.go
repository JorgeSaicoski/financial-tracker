package postgresql

import (
	"context"
	"errors"
	"testing"

	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

func TestLimitsGetValueReturnsSeededValue(t *testing.T) {
	repo := NewLimitsRepository(openTestDB(t))
	ctx := context.Background()

	v, err := repo.GetValue(ctx, "max_categories_per_user")
	if err != nil {
		t.Fatal(err)
	}
	if v != 10 {
		t.Errorf("max_categories_per_user = %d, want the migration's seeded value of 10", v)
	}
}

func TestLimitsGetValueUnknownName(t *testing.T) {
	repo := NewLimitsRepository(openTestDB(t))
	ctx := context.Background()

	if _, err := repo.GetValue(ctx, "no-such-limit"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}
