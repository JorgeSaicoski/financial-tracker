package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/usecases"
	"github.com/JorgeSaicoski/financial-tracker/internal/interfaces/api/reqctx"
	interfacedto "github.com/JorgeSaicoski/financial-tracker/internal/interfaces/dto"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/logger"
)

type paymentMethodHandler struct {
	createMethod usecases.CreatePaymentMethodUseCase
	updateMethod usecases.UpdatePaymentMethodUseCase
	deleteMethod usecases.DeletePaymentMethodUseCase

	log logger.Logger
}

// NewPaymentMethodHandler returns interface type for dependency injection.
func NewPaymentMethodHandler(
	createMethod usecases.CreatePaymentMethodUseCase,
	updateMethod usecases.UpdatePaymentMethodUseCase,
	deleteMethod usecases.DeletePaymentMethodUseCase,
	log logger.Logger,
) PaymentMethodHandler {
	return &paymentMethodHandler{
		createMethod: createMethod,
		updateMethod: updateMethod,
		deleteMethod: deleteMethod,
		log:          log,
	}
}

// CreatePaymentMethod handles POST /payment-methods.
func (h *paymentMethodHandler) CreatePaymentMethod(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		writeError(h.log, w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req interfacedto.CreatePaymentMethodRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(h.log, w, http.StatusBadRequest, "invalid request body")
		return
	}

	method, err := h.createMethod.Execute(r.Context(), usecases.CreatePaymentMethodInput{
		UserID: userID,
		Name:   req.Name,
	})
	if err != nil {
		h.writeUsecaseError(w, "create payment method", err)
		return
	}
	writeJSON(h.log, w, http.StatusCreated, toPaymentMethodResponse(method))
}

// UpdatePaymentMethod handles PATCH /payment-methods/{id}.
func (h *paymentMethodHandler) UpdatePaymentMethod(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		writeError(h.log, w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req interfacedto.UpdatePaymentMethodRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(h.log, w, http.StatusBadRequest, "invalid request body")
		return
	}

	method, err := h.updateMethod.Execute(r.Context(), userID, r.PathValue("id"), usecases.UpdatePaymentMethodInput{
		Name: req.Name,
	})
	if err != nil {
		h.writeUsecaseError(w, "update payment method", err)
		return
	}
	writeJSON(h.log, w, http.StatusOK, toPaymentMethodResponse(method))
}

// DeletePaymentMethod handles DELETE /payment-methods/{id}. Deleting one
// still referenced by movements is allowed — it's a label, not an FK.
func (h *paymentMethodHandler) DeletePaymentMethod(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		writeError(h.log, w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := h.deleteMethod.Execute(r.Context(), userID, r.PathValue("id")); err != nil {
		h.writeUsecaseError(w, "delete payment method", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *paymentMethodHandler) writeUsecaseError(w http.ResponseWriter, action string, err error) {
	switch {
	case apperrors.Is(err, apperrors.ErrInvalidInput):
		writeError(h.log, w, http.StatusBadRequest, err.Error())
	case apperrors.Is(err, apperrors.ErrNotFound):
		writeError(h.log, w, http.StatusNotFound, "payment method not found")
	case apperrors.Is(err, apperrors.ErrConflict):
		writeError(h.log, w, http.StatusConflict, err.Error())
	default:
		h.log.Error("%s failed: %v", action, err)
		writeError(h.log, w, http.StatusInternalServerError, "internal error")
	}
}
