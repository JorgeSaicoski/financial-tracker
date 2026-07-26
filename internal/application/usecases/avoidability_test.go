package usecases

import (
	"testing"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
)

func intPtrAv(n int) *int { return &n }

func TestEffectiveAvoidabilityOverrideWins(t *testing.T) {
	m := &dto.MovementDTO{Category: "food", AvoidabilityOverridePercent: intPtrAv(100)}
	byName := CategoriesByName([]*dto.CategoryDTO{
		{Name: "food", AvoidabilityPercent: intPtrAv(20)},
	})
	got := EffectiveAvoidability(m, byName)
	if got == nil || *got != 100 {
		t.Fatalf("got %v, want 100 (override)", got)
	}
}

func TestEffectiveAvoidabilityFallsBackToCategory(t *testing.T) {
	m := &dto.MovementDTO{Category: "Food"} // case-insensitive match
	byName := CategoriesByName([]*dto.CategoryDTO{
		{Name: "food", AvoidabilityPercent: intPtrAv(20)},
	})
	got := EffectiveAvoidability(m, byName)
	if got == nil || *got != 20 {
		t.Fatalf("got %v, want 20 (category)", got)
	}
}

func TestEffectiveAvoidabilityNoneWhenNeitherResolves(t *testing.T) {
	cases := []*dto.MovementDTO{
		{Category: ""},                 // uncategorized, no override
		{Category: "unknown-category"}, // category not in the map (e.g. deleted)
		{Category: "transfer"},         // system category, nil avoidability
	}
	byName := CategoriesByName([]*dto.CategoryDTO{
		{Name: "transfer", AvoidabilityPercent: nil},
	})
	for _, m := range cases {
		if got := EffectiveAvoidability(m, byName); got != nil {
			t.Errorf("movement %+v: got %v, want nil", m, *got)
		}
	}
}
