// Package recurring generates ordinary movements from recurring rules
// (BACK-07) on a background schedule, the same shape as application/sync's
// background loop for pushing movements to ledger-service.
package recurring

import (
	"strconv"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
)

// dueOccurrences returns every scheduled date for rule strictly after its
// watermark (LastGeneratedAt, or the day before StartsAt if it has never
// generated) and up to and including now — i.e. exactly the dates
// requirements call for: "(last_generated_at, now]". Dates past EndsAt
// (inclusive bound) are never returned.
//
// Capped at 1,200 monthly ticks (100 years) as a safety net against a
// pathological watermark; a real rule never gets remotely close.
func dueOccurrences(rule *dto.RecurringRuleDTO, now time.Time) []time.Time {
	after := rule.StartsAt.AddDate(0, 0, -1)
	if rule.LastGeneratedAt != nil {
		after = *rule.LastGeneratedAt
	}

	year, month, _ := after.Date()
	m := int(month)

	var due []time.Time
	for i := 0; i < 1200; i++ {
		occ := occurrenceInMonth(year, m, rule.DayOfMonth)
		if !occ.After(after) {
			m++
			continue
		}
		if occ.After(now) {
			break
		}
		if rule.EndsAt != nil && occ.After(*rule.EndsAt) {
			break
		}
		due = append(due, occ)
		after = occ
		m++
	}
	return due
}

// occurrenceInMonth is dayOfMonth's date within (year, month) — month may
// be outside 1-12 (e.g. 13, 50, ...); time.Date normalizes that by
// carrying the excess into year, which is exactly the "keep incrementing
// month, let the calendar roll the year over" behavior dueOccurrences
// relies on.
func occurrenceInMonth(year, month int, dayOfMonth string) time.Time {
	if dayOfMonth == "last" {
		// One day before the 1st of the following month.
		return time.Date(year, time.Month(month+1), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
	}
	day, _ := strconv.Atoi(dayOfMonth) // validated at rule-creation time
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}
