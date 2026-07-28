package entities

import "time"

// Card is a credit card profile (BACK-08): a closing day (statement cut)
// and a due day (when the installment actually hits the user's money),
// plus optional limit/budget tracking. CreditLimit and MonthlyBudget are
// independent — a user may budget well under their actual limit, so
// "how much can I still spend" is computed against MonthlyBudget, not
// CreditLimit.
type Card struct {
	ID            string
	UserID        string
	Name          string
	LastFour      string // optional; empty when not recorded
	ClosingDay    string // "1"-"28" or "last" — same convention as recurring rules
	DueDay        string // "1"-"28" or "last"
	CreditLimit   *int64 // smallest currency unit; nil when not tracked
	MonthlyBudget *int64 // smallest currency unit; nil when not tracked
	Currency      string
	CreatedAt     time.Time
}

// ValidCardDay reports whether s is an accepted closing_day/due_day value:
// "1" through "28" (never 29-31, so a fixed day never drifts across
// months of different lengths) or the literal "last".
func ValidCardDay(s string) bool {
	if s == "last" {
		return true
	}
	n, ok := parseDay(s)
	return ok && n >= 1 && n <= 28
}

func parseDay(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	return n, true
}

// ResolveCardDay turns a "1"-"28"/"last" day-of-month value into the
// actual day number for the given year/month — "last" resolves to that
// month's real last day (28-31, leap-year-aware for February), an
// explicit "1"-"28" is always valid as-is since every month has at least
// 28 days.
func ResolveCardDay(day string, year int, month time.Month) int {
	if day == "last" {
		// Day 0 of the following month is the last day of this one — a
		// standard time.Date idiom, and correctly leap-year-aware.
		return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
	}
	n, _ := parseDay(day)
	return n
}

// dayRank orders closing_day/due_day values for same-month-vs-next-month
// comparisons: an explicit "1"-"28" ranks by its numeric value, and
// "last" ranks above all of them (30) since it always falls on or after
// any explicit day within its own month.
func dayRank(day string) int {
	if day == "last" {
		return 30
	}
	n, _ := parseDay(day)
	return n
}

// NextCardDueDate returns the due date of the statement that closes on
// or immediately before purchaseDate — the cycle a purchase on
// purchaseDate belongs to. A purchase on or before the closing day lands
// in the cycle closing that same month; after it, the next month's.
//
// Whether the due date falls in the same month as the closing day or the
// following one depends on how the two days compare: a due day ranking
// on/after the closing day (e.g. close on the 5th, due on the 20th) lands
// in the closing month; one ranking before it (e.g. close on the 25th,
// due on the 5th) rolls into the month after closing — the common
// close-then-bill-a-few-days-later card pattern.
func NextCardDueDate(closingDay, dueDay string, purchaseDate time.Time) time.Time {
	year, month := purchaseDate.Year(), purchaseDate.Month()
	closingResolved := ResolveCardDay(closingDay, year, month)

	closingYear, closingMonth := year, month
	if purchaseDate.Day() > closingResolved {
		closingMonth++
	}
	return dueDateForClosing(closingDay, dueDay, closingYear, closingMonth)
}

// dueDateForClosing turns a cycle's closing (year, month) into that
// cycle's due date — the same closing-to-due transformation
// NextCardDueDate and CurrentStatementDueDate both need, factored out so
// each only has to compute which closing month it means.
func dueDateForClosing(closingDay, dueDay string, closingYear int, closingMonth time.Month) time.Time {
	// time.Date normalizes an out-of-range month (13, or 0/negative)
	// into the adjacent year automatically — handles December <-> January
	// rollover in both directions.
	normalized := time.Date(closingYear, closingMonth, 1, 0, 0, 0, 0, time.UTC)
	closingYear, closingMonth = normalized.Year(), normalized.Month()

	dueYear, dueMonth := closingYear, closingMonth
	if dayRank(dueDay) < dayRank(closingDay) {
		dueMonth++
	}
	normalized = time.Date(dueYear, dueMonth, 1, 0, 0, 0, 0, time.UTC)
	dueYear, dueMonth = normalized.Year(), normalized.Month()

	dueDayResolved := ResolveCardDay(dueDay, dueYear, dueMonth)
	return time.Date(dueYear, dueMonth, dueDayResolved, 0, 0, 0, 0, time.UTC)
}

// CurrentStatementDueDate returns the due date of the most recently
// closed statement as of now — "how much he should pay [right now]" per
// BACK-08's next_due_total. A closing day on or before today's day-of-
// month has already closed this month; otherwise the most recent closing
// was last month's.
func CurrentStatementDueDate(closingDay, dueDay string, now time.Time) time.Time {
	year, month := now.Year(), now.Month()
	closingResolved := ResolveCardDay(closingDay, year, month)

	closingYear, closingMonth := year, month
	if now.Day() < closingResolved {
		closingMonth--
	}
	return dueDateForClosing(closingDay, dueDay, closingYear, closingMonth)
}

// AddCardCycle advances a due date by one billing cycle (one month, same
// due-day rule re-resolved for the new month so "last" keeps tracking
// each month's actual last day) — used for an installment purchase's
// second and later payments, which land on consecutive due dates.
func AddCardCycle(dueDay string, previousDueDate time.Time) time.Time {
	next := time.Date(previousDueDate.Year(), previousDueDate.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	resolved := ResolveCardDay(dueDay, next.Year(), next.Month())
	return time.Date(next.Year(), next.Month(), resolved, 0, 0, 0, 0, time.UTC)
}
