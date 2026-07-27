package usecases

import (
	"testing"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
)

func intPtrAv(n int) *int       { return &n }
func strPtrAv(s string) *string { return &s }

func TestEffectiveAvoidabilityOverrideWins(t *testing.T) {
	m := &dto.MovementDTO{CategoryID: strPtrAv("food"), AvoidabilityOverridePercent: intPtrAv(100)}
	byID := CategoriesByID([]*dto.CategoryDTO{
		{ID: "food", AvoidabilityPercent: intPtrAv(20)},
	})
	got := EffectiveAvoidability(m, byID)
	if got == nil || *got != 100 {
		t.Fatalf("got %v, want 100 (override)", got)
	}
}

func TestEffectiveAvoidabilityFallsBackToCategory(t *testing.T) {
	m := &dto.MovementDTO{CategoryID: strPtrAv("food")}
	byID := CategoriesByID([]*dto.CategoryDTO{
		{ID: "food", AvoidabilityPercent: intPtrAv(20)},
	})
	got := EffectiveAvoidability(m, byID)
	if got == nil || *got != 20 {
		t.Fatalf("got %v, want 20 (category)", got)
	}
}

func TestEffectiveAvoidabilityNoneWhenNeitherResolves(t *testing.T) {
	cases := []*dto.MovementDTO{
		{CategoryID: nil}, // uncategorized, no override
		{CategoryID: strPtrAv("unknown-category-id")}, // category not in the map (e.g. hidden)
		{CategoryID: strPtrAv("transfer")},            // system category, nil avoidability
	}
	byID := CategoriesByID([]*dto.CategoryDTO{
		{ID: "transfer", AvoidabilityPercent: nil},
	})
	for _, m := range cases {
		if got := EffectiveAvoidability(m, byID); got != nil {
			t.Errorf("movement %+v: got %v, want nil", m, *got)
		}
	}
}
