package recurring

import (
	"testing"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
)

func mustDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func datesOf(occurrences []time.Time) []string {
	out := make([]string, len(occurrences))
	for i, t := range occurrences {
		out[i] = t.Format("2006-01-02")
	}
	return out
}

func assertDates(t *testing.T, got []time.Time, want ...string) {
	t.Helper()
	gotStr := datesOf(got)
	if len(gotStr) != len(want) {
		t.Fatalf("dueOccurrences = %v, want %v", gotStr, want)
	}
	for i := range want {
		if gotStr[i] != want[i] {
			t.Fatalf("dueOccurrences = %v, want %v", gotStr, want)
		}
	}
}

func TestDueOccurrencesGeneratesOnDueDate(t *testing.T) {
	rule := &dto.RecurringRuleDTO{
		DayOfMonth: "15",
		StartsAt:   mustDate("2026-01-01"),
	}
	// Exactly the due date, nothing before it.
	assertDates(t, dueOccurrences(rule, mustDate("2026-01-15")), "2026-01-15")
}

func TestDueOccurrencesNotYetDue(t *testing.T) {
	rule := &dto.RecurringRuleDTO{
		DayOfMonth: "15",
		StartsAt:   mustDate("2026-01-01"),
	}
	if got := dueOccurrences(rule, mustDate("2026-01-14")); len(got) != 0 {
		t.Errorf("dueOccurrences = %v, want none (not due yet)", datesOf(got))
	}
}

func TestDueOccurrencesCatchesUpMultipleMissedPeriodsExactlyOnce(t *testing.T) {
	rule := &dto.RecurringRuleDTO{
		DayOfMonth: "1",
		StartsAt:   mustDate("2026-01-01"),
	}
	// Never generated, now is April: Jan, Feb, Mar, Apr are all due.
	assertDates(t, dueOccurrences(rule, mustDate("2026-04-01")),
		"2026-01-01", "2026-02-01", "2026-03-01", "2026-04-01")

	// Simulate having already generated through February — only March and
	// April remain, each appearing exactly once, not re-generated.
	feb := mustDate("2026-02-01")
	rule.LastGeneratedAt = &feb
	assertDates(t, dueOccurrences(rule, mustDate("2026-04-01")), "2026-03-01", "2026-04-01")
}

func TestDueOccurrencesLastDayOfMonthHandlesFebruary(t *testing.T) {
	rule := &dto.RecurringRuleDTO{
		DayOfMonth: "last",
		StartsAt:   mustDate("2026-01-01"),
	}
	assertDates(t, dueOccurrences(rule, mustDate("2026-02-28")), "2026-01-31", "2026-02-28")

	// 2028 is a leap year: last day of February is the 29th.
	rule2 := &dto.RecurringRuleDTO{
		DayOfMonth: "last",
		StartsAt:   mustDate("2028-02-01"),
	}
	assertDates(t, dueOccurrences(rule2, mustDate("2028-02-29")), "2028-02-29")
}

func TestDueOccurrencesRespectsEndsAt(t *testing.T) {
	ends := mustDate("2026-02-01")
	rule := &dto.RecurringRuleDTO{
		DayOfMonth: "1",
		StartsAt:   mustDate("2026-01-01"),
		EndsAt:     &ends,
	}
	// April is due per the schedule, but the rule ended after February.
	assertDates(t, dueOccurrences(rule, mustDate("2026-04-01")), "2026-01-01", "2026-02-01")
}

func TestDueOccurrencesStartsAtMidMonthSkipsToNextMonth(t *testing.T) {
	// day_of_month=1, but the rule doesn't start until Jan 15 — the first
	// occurrence should be Feb 1, not the already-passed Jan 1.
	rule := &dto.RecurringRuleDTO{
		DayOfMonth: "1",
		StartsAt:   mustDate("2026-01-15"),
	}
	assertDates(t, dueOccurrences(rule, mustDate("2026-02-01")), "2026-02-01")
}
