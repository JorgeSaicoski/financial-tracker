package entities

import "testing"

func TestPotentialSavingNilAvoidabilityIsZero(t *testing.T) {
	if got := PotentialSaving(1000, nil); got != 0 {
		t.Errorf("PotentialSaving(1000, nil) = %d, want 0", got)
	}
}

func TestPotentialSavingAppliesPercent(t *testing.T) {
	avoidability := 25
	if got := PotentialSaving(1000, &avoidability); got != 250 {
		t.Errorf("PotentialSaving(1000, 25%%) = %d, want 250", got)
	}
}
