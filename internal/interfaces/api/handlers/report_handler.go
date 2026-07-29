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
	getPurchasingPower usecases.GetPurchasingPowerUseCase

	log logger.Logger
}

// NewReportHandler returns interface type for dependency injection.
func NewReportHandler(getPurchasingPower usecases.GetPurchasingPowerUseCase, log logger.Logger) ReportHandler {
	return &reportHandler{getPurchasingPower: getPurchasingPower, log: log}
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

	months := 0
	if raw := r.URL.Query().Get("months"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			months = n
		}
	}

	report, err := h.getPurchasingPower.Execute(r.Context(), userID, months)
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
