package dto

import "time"

// PurchasingPowerCategorySpendingResponse is one category's slice of a
// month's native-currency spending, annotated with that category's
// avoidability_percent (BACK-14; absent for an unrecognized/uncategorized
// bucket).
type PurchasingPowerCategorySpendingResponse struct {
	Category            string `json:"category"`
	AvoidabilityPercent *int   `json:"avoidability_percent,omitempty"`
	Amount              int64  `json:"amount"`
}

// PurchasingPowerCurrencyResponse is one month's figures in one native
// currency (BACK-12) plus that same group's USD view, converted at each
// movement's own historical exchange rate — never summed across
// currencies natively. usd_incomplete is true when at least one
// movement's currency had no rate for its date; the usd_* figures then
// exclude that movement rather than guessing (native figures are always
// complete).
type PurchasingPowerCurrencyResponse struct {
	Currency           string                                    `json:"currency"`
	SpendingByCategory []PurchasingPowerCategorySpendingResponse `json:"spending_by_category"`
	Income             int64                                     `json:"income"`
	TotalExpenses      int64                                     `json:"total_expenses"`
	PotentialSavings   int64                                     `json:"potential_savings"`
	Profit             int64                                     `json:"profit"`

	IncomeUSD           int64 `json:"income_usd"`
	TotalExpensesUSD    int64 `json:"total_expenses_usd"`
	PotentialSavingsUSD int64 `json:"potential_savings_usd"`
	ProfitUSD           int64 `json:"profit_usd"`
	USDIncomplete       bool  `json:"usd_incomplete,omitempty"`
}

// PurchasingPowerMonthResponse is one calendar month's report. Currencies
// holds one entry per native currency active that month.
// ProfitUSDAtCurrentRate is a cross-currency sum (unlike everything in
// Currencies) since converting every currency at today's rate already
// normalizes them onto one comparable unit — see "what it's worth now"
// vs. "what it was worth then" in the README's "Purchasing power" section.
type PurchasingPowerMonthResponse struct {
	Month                  time.Time                         `json:"month"`
	Currencies             []PurchasingPowerCurrencyResponse `json:"currencies"`
	ProfitUSDAtCurrentRate int64                             `json:"profit_usd_at_current_rate"`
}

type PurchasingPowerResponse struct {
	Months []PurchasingPowerMonthResponse `json:"months"`
}

// CategoryAvoidabilityScoreResponse is one category's follow-through
// figure for one scored month, in one currency (BACK-18). Baseline,
// reduction_percent and weighted_score are all 0 when
// insufficient_history is true — the client should check that flag first,
// not infer "no history" from a zero baseline.
type CategoryAvoidabilityScoreResponse struct {
	Category            string  `json:"category"`
	AvoidabilityPercent *int    `json:"avoidability_percent,omitempty"`
	Baseline            int64   `json:"baseline"`
	Actual              int64   `json:"actual"`
	ReductionPercent    float64 `json:"reduction_percent"`
	WeightedScore       float64 `json:"weighted_score"`
	InsufficientHistory bool    `json:"insufficient_history,omitempty"`
}

// AvoidabilityScoreCurrencyResponse is one scored month's figures in one
// native currency (never summed across currencies). OverallScore sums
// WeightedScore across every category with a valid baseline in this
// currency.
type AvoidabilityScoreCurrencyResponse struct {
	Currency     string                              `json:"currency"`
	Categories   []CategoryAvoidabilityScoreResponse `json:"categories"`
	OverallScore float64                             `json:"overall_score"`
}

type AvoidabilityScoreMonthResponse struct {
	Month      time.Time                           `json:"month"`
	Currencies []AvoidabilityScoreCurrencyResponse `json:"currencies"`
}

type AvoidabilityScoreResponse struct {
	Months []AvoidabilityScoreMonthResponse `json:"months"`
}
