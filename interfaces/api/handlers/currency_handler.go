package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/JorgeSaicoski/financial-tracker/application/usecases"
	interfacedto "github.com/JorgeSaicoski/financial-tracker/interfaces/dto"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
	"github.com/JorgeSaicoski/financial-tracker/pkg/logger"
)

// CurrencyHandler exposes the user-extendable currency registry backing
// the frontend's currency dropdown.
type CurrencyHandler interface {
	ListCurrencies(w http.ResponseWriter, r *http.Request)
	AddCurrency(w http.ResponseWriter, r *http.Request)
}

type currencyHandler struct {
	listCurrencies usecases.ListCurrenciesUseCase
	addCurrency    usecases.AddCurrencyUseCase
	log            logger.Logger
}

// NewCurrencyHandler returns interface type for dependency injection.
func NewCurrencyHandler(
	listCurrencies usecases.ListCurrenciesUseCase,
	addCurrency usecases.AddCurrencyUseCase,
	log logger.Logger,
) CurrencyHandler {
	return &currencyHandler{listCurrencies: listCurrencies, addCurrency: addCurrency, log: log}
}

// ListCurrencies handles GET /currencies.
func (h *currencyHandler) ListCurrencies(w http.ResponseWriter, r *http.Request) {
	currencies, err := h.listCurrencies.Execute(r.Context())
	if err != nil {
		h.log.Error("list currencies failed: %v", err)
		writeError(h.log, w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(h.log, w, http.StatusOK, interfacedto.CurrenciesResponse{Currencies: currencies})
}

// AddCurrency handles POST /currencies. Adding an existing code is a
// no-op success, so the frontend can add without checking first.
func (h *currencyHandler) AddCurrency(w http.ResponseWriter, r *http.Request) {
	var req interfacedto.AddCurrencyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(h.log, w, http.StatusBadRequest, "invalid request body")
		return
	}

	if _, err := h.addCurrency.Execute(r.Context(), req.Code); err != nil {
		if apperrors.Is(err, apperrors.ErrInvalidInput) {
			writeError(h.log, w, http.StatusBadRequest, err.Error())
			return
		}
		h.log.Error("add currency failed: %v", err)
		writeError(h.log, w, http.StatusInternalServerError, "internal error")
		return
	}

	currencies, err := h.listCurrencies.Execute(r.Context())
	if err != nil {
		h.log.Error("list currencies after add failed: %v", err)
		writeError(h.log, w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(h.log, w, http.StatusCreated, interfacedto.CurrenciesResponse{Currencies: currencies})
}
