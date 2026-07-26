package entities

import (
	"testing"
	"time"
)

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestValidCardDay(t *testing.T) {
	for _, good := range []string{"1", "28", "15", "last"} {
		if !ValidCardDay(good) {
			t.Errorf("%q should be valid", good)
		}
	}
	for _, bad := range []string{"", "0", "29", "31", "last day", "abc"} {
		if ValidCardDay(bad) {
			t.Errorf("%q should be invalid", bad)
		}
	}
}

func TestNextCardDueDateBeforeOnAfterClosingDay(t *testing.T) {
	// closing=15, due=25: due later in the month than closing, so the
	// due date lands in the same month the statement closes.
	const closing, due = "15", "25"

	before := NextCardDueDate(closing, due, date(2026, time.March, 10))
	if !before.Equal(date(2026, time.March, 25)) {
		t.Errorf("before closing: got %v, want 2026-03-25", before)
	}

	onDay := NextCardDueDate(closing, due, date(2026, time.March, 15))
	if !onDay.Equal(date(2026, time.March, 25)) {
		t.Errorf("on closing day: got %v, want 2026-03-25 (on-closing-day purchases belong to the closing cycle)", onDay)
	}

	after := NextCardDueDate(closing, due, date(2026, time.March, 16))
	if !after.Equal(date(2026, time.April, 25)) {
		t.Errorf("after closing: got %v, want 2026-04-25", after)
	}
}

func TestNextCardDueDateDueDayEarlierThanClosingDay(t *testing.T) {
	// closing=25, due=5: due earlier in the month than closing, so the
	// due date rolls into the month after the one that closes.
	const closing, due = "25", "5"

	before := NextCardDueDate(closing, due, date(2026, time.March, 10))
	if !before.Equal(date(2026, time.April, 5)) {
		t.Errorf("before closing: got %v, want 2026-04-05", before)
	}

	after := NextCardDueDate(closing, due, date(2026, time.March, 30))
	if !after.Equal(date(2026, time.May, 5)) {
		t.Errorf("after closing: got %v, want 2026-05-05", after)
	}
}

func TestNextCardDueDateDecemberRollover(t *testing.T) {
	// closing=25, due=5: a December purchase after closing rolls the
	// closing cycle into January, and the due date (earlier-ranked than
	// closing) rolls one month past that, into February — both crossing
	// the year boundary.
	got := NextCardDueDate("25", "5", date(2025, time.December, 30))
	if !got.Equal(date(2026, time.February, 5)) {
		t.Errorf("got %v, want 2026-02-05", got)
	}
}

func TestNextCardDueDateFebruaryLast(t *testing.T) {
	// closing=20, due=last: a purchase after the 20th closes into the
	// next month; when that's February, "last" must resolve to the
	// real leap/non-leap last day, not a hardcoded 28 or 30.
	leapYear := NextCardDueDate("20", "last", date(2024, time.January, 25))
	if !leapYear.Equal(date(2024, time.February, 29)) {
		t.Errorf("2024 (leap): got %v, want 2024-02-29", leapYear)
	}

	nonLeapYear := NextCardDueDate("20", "last", date(2025, time.January, 25))
	if !nonLeapYear.Equal(date(2025, time.February, 28)) {
		t.Errorf("2025 (non-leap): got %v, want 2025-02-28", nonLeapYear)
	}
}

func TestCurrentStatementDueDateSameMonth(t *testing.T) {
	// closing=15, due=25: due date lands in the closing month.
	const closing, due = "15", "25"

	afterClosing := CurrentStatementDueDate(closing, due, date(2026, time.March, 20))
	if !afterClosing.Equal(date(2026, time.March, 25)) {
		t.Errorf("after closing: got %v, want 2026-03-25", afterClosing)
	}

	onClosing := CurrentStatementDueDate(closing, due, date(2026, time.March, 15))
	if !onClosing.Equal(date(2026, time.March, 25)) {
		t.Errorf("on closing day: got %v, want 2026-03-25 (closing day counts as already closed)", onClosing)
	}

	beforeClosing := CurrentStatementDueDate(closing, due, date(2026, time.March, 10))
	if !beforeClosing.Equal(date(2026, time.February, 25)) {
		t.Errorf("before closing: got %v, want 2026-02-25 (last month's statement is the most recently closed one)", beforeClosing)
	}
}

func TestCurrentStatementDueDateRollsIntoNextMonth(t *testing.T) {
	// closing=25, due=5: due date rolls into the month after closing.
	const closing, due = "25", "5"

	beforeClosing := CurrentStatementDueDate(closing, due, date(2026, time.March, 10))
	if !beforeClosing.Equal(date(2026, time.March, 5)) {
		t.Errorf("before closing: got %v, want 2026-03-05 (February's statement, due in March)", beforeClosing)
	}

	afterClosing := CurrentStatementDueDate(closing, due, date(2026, time.March, 30))
	if !afterClosing.Equal(date(2026, time.April, 5)) {
		t.Errorf("after closing: got %v, want 2026-04-05 (March's statement, due in April)", afterClosing)
	}
}

func TestAddCardCycleAdvancesOneMonthReResolvingLast(t *testing.T) {
	first := date(2024, time.January, 31) // due_day "last" for January
	second := AddCardCycle("last", first)
	if !second.Equal(date(2024, time.February, 29)) {
		t.Errorf("got %v, want 2024-02-29 (leap Feb's real last day)", second)
	}
	third := AddCardCycle("last", second)
	if !third.Equal(date(2024, time.March, 31)) {
		t.Errorf("got %v, want 2024-03-31", third)
	}
}
