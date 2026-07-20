package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	appdto "github.com/JorgeSaicoski/financial-tracker/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/application/usecases"
	interfacedto "github.com/JorgeSaicoski/financial-tracker/interfaces/dto"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
	"github.com/JorgeSaicoski/financial-tracker/pkg/logger"
)

// MovementHandler exposes financial-tracker's own /movements API. It never
// talks to ledger-service directly - it only calls usecases, which depend
// on the MovementRepository interface.
type MovementHandler struct {
	createMovement  *usecases.CreateMovement
	getMovement     *usecases.GetMovement
	listMovements   *usecases.ListMovements
	defaultUserID   string
	defaultCurrency string
	log             logger.Logger
}

func NewMovementHandler(
	createMovement *usecases.CreateMovement,
	getMovement *usecases.GetMovement,
	listMovements *usecases.ListMovements,
	defaultUserID string,
	defaultCurrency string,
	log logger.Logger,
) *MovementHandler {
	return &MovementHandler{
		createMovement:  createMovement,
		getMovement:     getMovement,
		listMovements:   listMovements,
		defaultUserID:   defaultUserID,
		defaultCurrency: defaultCurrency,
		log:             log,
	}
}

// CreateMovement handles POST /movements
func (h *MovementHandler) CreateMovement(w http.ResponseWriter, r *http.Request) {
	var req interfacedto.CreateMovementRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	userID := req.UserID
	if userID == "" {
		userID = h.defaultUserID
	}
	currency := req.Currency
	if currency == "" {
		currency = h.defaultCurrency
	}

	movement, err := h.createMovement.Execute(r.Context(), appdto.CreateMovementInput{
		UserID:   userID,
		Amount:   req.Amount,
		Currency: currency,
	})
	if err != nil {
		h.writeUsecaseError(w, "create movement", err)
		return
	}

	h.writeJSON(w, http.StatusCreated, toMovementResponse(movement))
}

// GetMovement handles GET /movements?id=X
func (h *MovementHandler) GetMovement(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	movement, err := h.getMovement.Execute(r.Context(), id)
	if err != nil {
		h.writeUsecaseError(w, "get movement", err)
		return
	}

	h.writeJSON(w, http.StatusOK, toMovementResponse(movement))
}

// ListMovements handles GET /movements?user_id=X&currency=Y&limit=&offset=
func (h *MovementHandler) ListMovements(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		userID = h.defaultUserID
	}

	var currency *string
	if c := r.URL.Query().Get("currency"); c != "" {
		currency = &c
	}

	limit, err := parseNonNegativeIntParam(r, "limit")
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid limit")
		return
	}

	offset, err := parseNonNegativeIntParam(r, "offset")
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid offset")
		return
	}

	result, err := h.listMovements.Execute(r.Context(), appdto.ListMovementsInput{
		UserID:   userID,
		Currency: currency,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		h.writeUsecaseError(w, "list movements", err)
		return
	}

	movements := make([]interfacedto.MovementResponse, 0, len(result.Movements))
	for _, m := range result.Movements {
		movements = append(movements, toMovementResponse(m))
	}

	h.writeJSON(w, http.StatusOK, interfacedto.ListMovementsResponse{
		Movements: movements,
		Balance:   result.Balance,
	})
}

func parseNonNegativeIntParam(r *http.Request, name string) (int, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return 0, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 0 {
		return 0, errInvalidParam
	}
	return v, nil
}

var errInvalidParam = errors.New("invalid parameter")

func toMovementResponse(m appdto.MovementOutput) interfacedto.MovementResponse {
	return interfacedto.MovementResponse{
		ID:        m.ID,
		UserID:    m.UserID,
		Amount:    m.Amount,
		Currency:  m.Currency,
		Timestamp: m.Timestamp,
	}
}

func (h *MovementHandler) writeUsecaseError(w http.ResponseWriter, action string, err error) {
	switch {
	case errors.Is(err, apperrors.ErrInvalidInput):
		h.writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, apperrors.ErrNotFound):
		h.writeError(w, http.StatusNotFound, "movement not found")
	default:
		h.log.Error("%s failed: %v", action, err)
		h.writeError(w, http.StatusBadGateway, "upstream ledger service error")
	}
}

func (h *MovementHandler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.log.Error("failed to encode JSON response: %v", err)
	}
}

func (h *MovementHandler) writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(interfacedto.ErrorResponse{Error: message}); err != nil {
		h.log.Error("failed to encode error response: %v", err)
	}
}
