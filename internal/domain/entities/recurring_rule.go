package entities

import "time"

// RecurringRule is a definition that generates ordinary movements on a
// monthly schedule (rent, salary, subscriptions) so they don't need
// manual re-entry every month. The rule itself is never a movement;
// generated movements carry RecurringRuleID (see Movement) so the UI can
// show provenance and the generator can be idempotent.
type RecurringRule struct {
	ID            string
	UserID        string
	Amount        int64
	Currency      string
	Description   string
	PaymentMethod string

	// CategoryID references the shared categories registry (BACK-14
	// follow-up) — nil means genuinely uncategorized. Copied directly
	// onto each movement the generator creates (see
	// internal/application/recurring/service.go); no name resolution
	// happens at generation time anymore, so this must already be a
	// valid id by the time the rule is created.
	CategoryID *string

	// AccountID mirrors Movement's own field: which account the
	// generated movements move money in/out of, nil when unassigned.
	AccountID *string

	// DayOfMonth is "1".."28" or "last" — never 29-31, so a fixed day
	// never silently drifts across months of different lengths. "last"
	// always means that month's actual last day (28-31 depending on
	// the month).
	DayOfMonth string

	// StartsAt is when the rule begins generating; EndsAt, if set, is
	// the last date it's allowed to generate for (inclusive).
	StartsAt time.Time
	EndsAt   *time.Time

	// Active false stops future generation; it never touches movements
	// already generated.
	Active bool

	// LastGeneratedAt is the watermark the generator advances past on
	// every pass — nil means the rule has never generated anything yet.
	LastGeneratedAt *time.Time

	CreatedAt time.Time
}

// ValidDayOfMonth reports whether s is an acceptable DayOfMonth value:
// "last", or an integer from 1 to 28.
func ValidDayOfMonth(s string) bool {
	if s == "last" {
		return true
	}
	if len(s) == 0 || len(s) > 2 {
		return false
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
		n = n*10 + int(c-'0')
	}
	return n >= 1 && n <= 28
}
