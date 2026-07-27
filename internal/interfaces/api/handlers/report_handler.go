package handlers

import (
	"net/http"
	"strconv"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/usecases"
	"github.com/JorgeSaicoski/financial-tracker/internal/interfaces/api/reqctx"
	interfacedto "github.com/JorgeSaicoski/financial-tracker/internal/interfaces/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/logger"
)

type reportHandler struct {
	getPurchasingPower   usecases.GetPurchasingPowerUseCase
	getAvoidabilityScore usecases.GetAvoidabilityScoreUseCase

	log logger.Logger
}

// NewReportHandler returns interface type for dependency injection.
func NewReportHandler(getPurchasingPower usecases.GetPurchasingPowerUseCase, getAvoidabilityScore usecases.GetAvoidabilityScoreUseCase, log logger.Logger) ReportHandler {
	return &reportHandler{getPurchasingPower: getPurchasingPower, getAvoidabilityScore: getAvoidabilityScore, log: log}
}

// monthsQueryParam parses the ?months= query param shared by both report
// endpoints below — invalid or missing values fall back to 0 (each
// usecase applies its own default/clamp), since this is a reporting
// convenience param, not one where a malformed value should block the
// response.
func monthsQueryParam(r *http.Request) int {
	months := 0
	if raw := r.URL.Query().Get("months"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			months = n
		}
	}
	return months
}

// PurchasingPower handles GET /reports/purchasing-power?months=N (BACK-12).
// months defaults to 6, clamped to 24, invalid values fall back to the
// default rather than erroring — this is a reporting convenience
// endpoint, not one where a malformed query param should block the
// response.
func (h *reportHandler) PurchasingPower(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		writeError(h.log, w, http.StatusUnauthorized, "unauthorized")
		return
	}

	report, err := h.getPurchasingPower.Execute(r.Context(), userID, monthsQueryParam(r))
	if err != nil {
		writeUsecaseError(h.log, w, "get purchasing power report", err)
		return
	}

	resp := interfacedto.PurchasingPowerResponse{Months: make([]interfacedto.PurchasingPowerMonthResponse, 0, len(report))}
	for _, month := range report {
		resp.Months = append(resp.Months, toPurchasingPowerMonthResponse(month))
	}
	writeJSON(h.log, w, http.StatusOK, resp)
}

// AvoidabilityScore handles GET /reports/avoidability-score?months=N
// (BACK-18). Same months default/clamp/error-tolerance as PurchasingPower.
func (h *reportHandler) AvoidabilityScore(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		writeError(h.log, w, http.StatusUnauthorized, "unauthorized")
		return
	}

	report, err := h.getAvoidabilityScore.Execute(r.Context(), userID, monthsQueryParam(r))
	if err != nil {
		writeUsecaseError(h.log, w, "get avoidability score report", err)
		return
	}

	resp := interfacedto.AvoidabilityScoreResponse{Months: make([]interfacedto.AvoidabilityScoreMonthResponse, 0, len(report))}
	for _, month := range report {
		resp.Months = append(resp.Months, toAvoidabilityScoreMonthResponse(month))
	}
	writeJSON(h.log, w, http.StatusOK, resp)
}

func toPurchasingPowerMonthResponse(m usecases.PurchasingPowerMonth) interfacedto.PurchasingPowerMonthResponse {
	resp := interfacedto.PurchasingPowerMonthResponse{
		Month:                  m.Month,
		Currencies:             make([]interfacedto.PurchasingPowerCurrencyResponse, 0, len(m.Currencies)),
		ProfitUSDAtCurrentRate: m.ProfitUSDAtCurrentRate,
	}
	for _, c := range m.Currencies {
		currency := interfacedto.PurchasingPowerCurrencyResponse{
			Currency:            c.Currency,
			SpendingByCategory:  make([]interfacedto.PurchasingPowerCategorySpendingResponse, 0, len(c.SpendingByCategory)),
			Income:              c.Income,
			TotalExpenses:       c.TotalExpenses,
			PotentialSavings:    c.PotentialSavings,
			Profit:              c.Profit,
			IncomeUSD:           c.IncomeUSD,
			TotalExpensesUSD:    c.TotalExpensesUSD,
			PotentialSavingsUSD: c.PotentialSavingsUSD,
			ProfitUSD:           c.ProfitUSD,
			USDIncomplete:       c.USDIncomplete,
		}
		for _, s := range c.SpendingByCategory {
			currency.SpendingByCategory = append(currency.SpendingByCategory, interfacedto.PurchasingPowerCategorySpendingResponse{
				Category: s.Category, AvoidabilityPercent: s.AvoidabilityPercent, Amount: s.Amount,
			})
		}
		resp.Currencies = append(resp.Currencies, currency)
	}
	return resp
}

func toAvoidabilityScoreMonthResponse(m usecases.AvoidabilityScoreMonth) interfacedto.AvoidabilityScoreMonthResponse {
	resp := interfacedto.AvoidabilityScoreMonthResponse{
		Month:      m.Month,
		Currencies: make([]interfacedto.AvoidabilityScoreCurrencyResponse, 0, len(m.Currencies)),
	}
	for _, c := range m.Currencies {
		currency := interfacedto.AvoidabilityScoreCurrencyResponse{
			Currency:     c.Currency,
			Categories:   make([]interfacedto.CategoryAvoidabilityScoreResponse, 0, len(c.Categories)),
			OverallScore: c.OverallScore,
		}
		for _, cat := range c.Categories {
			currency.Categories = append(currency.Categories, interfacedto.CategoryAvoidabilityScoreResponse{
				Category:            cat.Category,
				AvoidabilityPercent: cat.AvoidabilityPercent,
				Baseline:            cat.Baseline,
				Actual:              cat.Actual,
				ReductionPercent:    cat.ReductionPercent,
				WeightedScore:       cat.WeightedScore,
				InsufficientHistory: cat.InsufficientHistory,
			})
		}
		resp.Currencies = append(resp.Currencies, currency)
	}
	return resp
}
