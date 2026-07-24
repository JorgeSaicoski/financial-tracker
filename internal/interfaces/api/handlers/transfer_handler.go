package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/usecases"
	interfacedto "github.com/JorgeSaicoski/financial-tracker/internal/interfaces/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/logger"
)

// TransferHandler exposes account-to-account transfers: a linked
// debit/credit pair of movements that nets to zero, so it never changes
// the user's overall net worth.
type TransferHandler interface {
	CreateTransfer(w http.ResponseWriter, r *http.Request)
	CancelTransfer(w http.ResponseWriter, r *http.Request)
}

type transferHandler struct {
	createTransfer usecases.TransferBetweenAccountsUseCase
	cancelTransfer usecases.CancelTransferUseCase
	defaultUserID  string
	log            logger.Logger
}

// NewTransferHandler returns interface type for dependency injection.
func NewTransferHandler(
	createTransfer usecases.TransferBetweenAccountsUseCase,
	cancelTransfer usecases.CancelTransferUseCase,
	defaultUserID string,
	log logger.Logger,
) TransferHandler {
	return &transferHandler{
		createTransfer: createTransfer,
		cancelTransfer: cancelTransfer,
		defaultUserID:  defaultUserID,
		log:            log,
	}
}

// CreateTransfer handles POST /transfers.
func (h *transferHandler) CreateTransfer(w http.ResponseWriter, r *http.Request) {
	var req interfacedto.CreateTransferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(h.log, w, http.StatusBadRequest, "invalid request body")
		return
	}

	userID := req.UserID
	if userID == "" {
		userID = h.defaultUserID
	}
	var timestamp time.Time
	if req.Timestamp != nil {
		timestamp = *req.Timestamp
	}

	result, err := h.createTransfer.Execute(r.Context(), usecases.TransferBetweenAccountsInput{
		UserID:        userID,
		FromAccountID: req.FromAccountID,
		ToAccountID:   req.ToAccountID,
		Amount:        req.Amount,
		Description:   req.Description,
		Timestamp:     timestamp,
	})
	if err != nil {
		writeUsecaseError(h.log, w, "create transfer", err)
		return
	}

	writeJSON(h.log, w, http.StatusCreated, interfacedto.TransferResponse{
		TransferID: result.TransferID,
		Debit:      toMovementResponse(result.Debit),
		Credit:     toMovementResponse(result.Credit),
	})
}

// CancelTransfer handles POST /transfers/{id}/cancel, where {id} is the
// transfer_id shared by both legs.
func (h *transferHandler) CancelTransfer(w http.ResponseWriter, r *http.Request) {
	result, err := h.cancelTransfer.Execute(r.Context(), r.PathValue("id"))
	if err != nil {
		writeUsecaseError(h.log, w, "cancel transfer", err)
		return
	}

	writeJSON(h.log, w, http.StatusOK, interfacedto.CancelTransferResponse{
		Debit:  toCancelMovementResponse(result.Debit),
		Credit: toCancelMovementResponse(result.Credit),
	})
}
