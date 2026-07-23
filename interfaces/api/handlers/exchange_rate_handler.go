package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/application/usecases"
	interfacedto "github.com/JorgeSaicoski/financial-tracker/interfaces/dto"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
	"github.com/JorgeSaicoski/financial-tracker/pkg/logger"
)

// ExchangeRateHandler exposes user-managed, historical exchange rates
// against USD (BACK-11) — reference data the user maintains themselves,
// no external rate API involved.
type ExchangeRateHandler interface {
	SetExchangeRate(w http.ResponseWriter, r *http.Request)
	ListExchangeRates(w http.ResponseWriter, r *http.Request)
	DeleteExchangeRate(w http.ResponseWriter, r *http.Request)
}

type exchangeRateHandler struct {
	setRate    usecases.SetExchangeRateUseCase
	listRates  usecases.ListExchangeRatesUseCase
	deleteRate usecases.DeleteExchangeRateUseCase

	defaultUserID string
	log           logger.Logger
}

// NewExchangeRateHandler returns interface type for dependency injection.
func NewExchangeRateHandler(
	setRate usecases.SetExchangeRateUseCase,
	listRates usecases.ListExchangeRatesUseCase,
	deleteRate usecases.DeleteExchangeRateUseCase,
	defaultUserID string,
	log logger.Logger,
) ExchangeRateHandler {
	return &exchangeRateHandler{
		setRate:       setRate,
		listRates:     listRates,
		deleteRate:    deleteRate,
		defaultUserID: defaultUserID,
		log:           log,
	}
}

// SetExchangeRate handles POST /exchange-rates: registers today's (or a
// backdated) rate; posting the same currency+date again corrects it.
func (h *exchangeRateHandler) SetExchangeRate(w http.ResponseWriter, r *http.Request) {
	var req interfacedto.SetExchangeRateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(h.log, w, http.StatusBadRequest, "invalid request body")
		return
	}

	var effectiveFrom time.Time
	if req.EffectiveFrom != nil {
		effectiveFrom = *req.EffectiveFrom
	}

	rate, err := h.setRate.Execute(r.Context(), usecases.SetExchangeRateInput{
		UserID:        h.defaultUserID,
		Currency:      req.Currency,
		UnitsPerUSD:   req.UnitsPerUSD,
		EffectiveFrom: effectiveFrom,
	})
	if err != nil {
		h.writeUsecaseError(w, "set exchange rate", err)
		return
	}
	writeJSON(h.log, w, http.StatusCreated, toExchangeRateResponse(rate))
}

// ListExchangeRates handles GET /exchange-rates: every currency the user
// has rates for, each with its currently-effective rate and full history.
func (h *exchangeRateHandler) ListExchangeRates(w http.ResponseWriter, r *http.Request) {
	groups, err := h.listRates.Execute(r.Context(), h.defaultUserID)
	if err != nil {
		h.writeUsecaseError(w, "list exchange rates", err)
		return
	}

	resp := interfacedto.ExchangeRatesResponse{Rates: make([]interfacedto.ExchangeRateGroupResponse, 0, len(groups))}
	for _, g := range groups {
		group := interfacedto.ExchangeRateGroupResponse{
			Currency: g.Currency,
			History:  make([]interfacedto.ExchangeRateResponse, 0, len(g.History)),
		}
		if g.Current != nil {
			current := toExchangeRateResponse(g.Current)
			group.Current = &current
		}
		for _, rate := range g.History {
			group.History = append(group.History, toExchangeRateResponse(rate))
		}
		resp.Rates = append(resp.Rates, group)
	}
	writeJSON(h.log, w, http.StatusOK, resp)
}

// DeleteExchangeRate handles DELETE /exchange-rates/{id}: fixing a typo
// in history is legitimate, this is user-owned reference data.
func (h *exchangeRateHandler) DeleteExchangeRate(w http.ResponseWriter, r *http.Request) {
	if err := h.deleteRate.Execute(r.Context(), h.defaultUserID, r.PathValue("id")); err != nil {
		h.writeUsecaseError(w, "delete exchange rate", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func toExchangeRateResponse(r *dto.ExchangeRateDTO) interfacedto.ExchangeRateResponse {
	return interfacedto.ExchangeRateResponse{
		ID:            r.ID,
		Currency:      r.Currency,
		UnitsPerUSD:   r.UnitsPerUSD,
		EffectiveFrom: r.EffectiveFrom,
		CreatedAt:     r.CreatedAt,
	}
}

func (h *exchangeRateHandler) writeUsecaseError(w http.ResponseWriter, action string, err error) {
	switch {
	case apperrors.Is(err, apperrors.ErrInvalidInput):
		writeError(h.log, w, http.StatusBadRequest, err.Error())
	case apperrors.Is(err, apperrors.ErrNotFound):
		writeError(h.log, w, http.StatusNotFound, "exchange rate not found")
	case apperrors.Is(err, apperrors.ErrConflict):
		writeError(h.log, w, http.StatusConflict, err.Error())
	default:
		h.log.Error("%s failed: %v", action, err)
		writeError(h.log, w, http.StatusInternalServerError, "internal error")
	}
}
