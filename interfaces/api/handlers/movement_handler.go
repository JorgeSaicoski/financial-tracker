package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	syncapp "github.com/JorgeSaicoski/financial-tracker/application/sync"
	"github.com/JorgeSaicoski/financial-tracker/application/usecases"
	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
	interfacedto "github.com/JorgeSaicoski/financial-tracker/interfaces/dto"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
	"github.com/JorgeSaicoski/financial-tracker/pkg/logger"
)

// SyncRunner is what the handler needs from application/sync: run one
// synchronous pass (POST /sync ignores the retry cooldown — the user
// explicitly asked).
type SyncRunner interface {
	RunPassNow(ctx context.Context) syncapp.Summary
}

// MovementHandler exposes financial-tracker's own API. It never talks to
// the database or ledger-service directly - it only calls application
// code, which depends on the domain repository interfaces.
type MovementHandler interface {
	CreateMovement(w http.ResponseWriter, r *http.Request)
	GetMovement(w http.ResponseWriter, r *http.Request)
	ListMovements(w http.ResponseWriter, r *http.Request)
	CancelMovement(w http.ResponseWriter, r *http.Request)
	CancelCreditCardPurchase(w http.ResponseWriter, r *http.Request)
	Sync(w http.ResponseWriter, r *http.Request)
	ListCategories(w http.ResponseWriter, r *http.Request)
}

type movementHandler struct {
	createMovement usecases.CreateMovementUseCase
	createPurchase usecases.CreateCreditCardPurchaseUseCase
	getMovement    usecases.GetMovementUseCase
	listMovements  usecases.ListMovementsUseCase
	cancelMovement usecases.CancelMovementUseCase
	cancelPurchase usecases.CancelCreditCardPurchaseUseCase
	syncRunner     SyncRunner

	defaultUserID   string
	defaultCurrency string
	log             logger.Logger
}

// NewMovementHandler returns interface type for dependency injection.
func NewMovementHandler(
	createMovement usecases.CreateMovementUseCase,
	createPurchase usecases.CreateCreditCardPurchaseUseCase,
	getMovement usecases.GetMovementUseCase,
	listMovements usecases.ListMovementsUseCase,
	cancelMovement usecases.CancelMovementUseCase,
	cancelPurchase usecases.CancelCreditCardPurchaseUseCase,
	syncRunner SyncRunner,
	defaultUserID string,
	defaultCurrency string,
	log logger.Logger,
) MovementHandler {
	return &movementHandler{
		createMovement:  createMovement,
		createPurchase:  createPurchase,
		getMovement:     getMovement,
		listMovements:   listMovements,
		cancelMovement:  cancelMovement,
		cancelPurchase:  cancelPurchase,
		syncRunner:      syncRunner,
		defaultUserID:   defaultUserID,
		defaultCurrency: defaultCurrency,
		log:             log,
	}
}

// CreateMovement handles POST /movements. When payment_method is
// "credit_card" and installments > 1 it creates an installment purchase
// (one purchase record + N monthly movements) instead of a single row.
func (h *movementHandler) CreateMovement(w http.ResponseWriter, r *http.Request) {
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

	if req.Installments > 1 {
		if entities.PaymentMethod(req.PaymentMethod) != entities.PaymentMethodCreditCard {
			h.writeError(w, http.StatusBadRequest, "installments require payment_method \"credit_card\"")
			return
		}
		purchase, movements, err := h.createPurchase.Execute(r.Context(), usecases.CreateCreditCardPurchaseInput{
			UserID:       userID,
			TotalAmount:  req.Amount,
			Currency:     currency,
			Description:  req.Description,
			Category:     entities.Category(req.Category),
			Installments: req.Installments,
		})
		if err != nil {
			h.writeUsecaseError(w, "create credit card purchase", err)
			return
		}
		h.writeJSON(w, http.StatusCreated, toPurchaseResponse(purchase, movements))
		return
	}

	movement, err := h.createMovement.Execute(r.Context(), usecases.CreateMovementInput{
		UserID:        userID,
		Amount:        req.Amount,
		Currency:      currency,
		Description:   req.Description,
		Category:      entities.Category(req.Category),
		PaymentMethod: entities.PaymentMethod(req.PaymentMethod),
	})
	if err != nil {
		h.writeUsecaseError(w, "create movement", err)
		return
	}

	h.writeJSON(w, http.StatusCreated, toMovementResponse(movement))
}

// GetMovement handles GET /movements?id=X
func (h *movementHandler) GetMovement(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	movement, err := h.getMovement.Execute(r.Context(), id)
	if err != nil {
		h.writeUsecaseError(w, "get movement", err)
		return
	}

	h.writeJSON(w, http.StatusOK, toMovementResponse(movement))
}

// ListMovements handles GET /movements?user_id=X&currency=Y&limit=&offset=
func (h *movementHandler) ListMovements(w http.ResponseWriter, r *http.Request) {
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

	result, err := h.listMovements.Execute(r.Context(), userID, currency, limit, offset)
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

// CancelMovement handles POST /movements/{id}/cancel
func (h *movementHandler) CancelMovement(w http.ResponseWriter, r *http.Request) {
	result, err := h.cancelMovement.Execute(r.Context(), r.PathValue("id"))
	if err != nil {
		h.writeUsecaseError(w, "cancel movement", err)
		return
	}

	resp := interfacedto.CancelMovementResponse{Movement: toMovementResponse(result.Movement)}
	if result.Reversal != nil {
		reversal := toMovementResponse(result.Reversal)
		resp.Reversal = &reversal
	}
	h.writeJSON(w, http.StatusOK, resp)
}

// CancelCreditCardPurchase handles POST /credit-card-purchases/{id}/cancel
func (h *movementHandler) CancelCreditCardPurchase(w http.ResponseWriter, r *http.Request) {
	result, err := h.cancelPurchase.Execute(r.Context(), r.PathValue("id"))
	if err != nil {
		h.writeUsecaseError(w, "cancel credit card purchase", err)
		return
	}

	resp := interfacedto.CancelCreditCardPurchaseResponse{
		Purchase:  toPurchaseResponse(result.Purchase, nil),
		Voided:    make([]interfacedto.MovementResponse, 0, len(result.Voided)),
		Reversals: make([]interfacedto.MovementResponse, 0, len(result.Reversals)),
	}
	for _, m := range result.Voided {
		resp.Voided = append(resp.Voided, toMovementResponse(m))
	}
	for _, m := range result.Reversals {
		resp.Reversals = append(resp.Reversals, toMovementResponse(m))
	}
	h.writeJSON(w, http.StatusOK, resp)
}

// Sync handles POST /sync: one synchronous catch-up pass against
// ledger-service, for the UI's "sync now" button.
func (h *movementHandler) Sync(w http.ResponseWriter, r *http.Request) {
	summary := h.syncRunner.RunPassNow(r.Context())
	h.writeJSON(w, http.StatusOK, interfacedto.SyncSummaryResponse{
		Synced: summary.Synced,
		Failed: summary.Failed,
	})
}

// ListCategories handles GET /categories so the frontend never hardcodes
// the fixed category/payment-method lists.
func (h *movementHandler) ListCategories(w http.ResponseWriter, _ *http.Request) {
	categories := make([]string, 0)
	for _, c := range entities.Categories() {
		categories = append(categories, string(c))
	}
	methods := make([]string, 0)
	for _, m := range entities.PaymentMethods() {
		methods = append(methods, string(m))
	}
	h.writeJSON(w, http.StatusOK, interfacedto.CategoriesResponse{
		Categories:     categories,
		PaymentMethods: methods,
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

func toMovementResponse(m *entities.Movement) interfacedto.MovementResponse {
	resp := interfacedto.MovementResponse{
		ID:            m.ID,
		UserID:        m.UserID,
		Amount:        m.Amount,
		Currency:      m.Currency,
		Description:   m.Description,
		Category:      string(m.Category),
		PaymentMethod: string(m.PaymentMethod),
		Status:        string(m.Status),
		SyncStatus:    string(m.SyncStatus),
		Timestamp:     m.Timestamp,
	}
	if m.LedgerTransactionID != nil {
		resp.LedgerTransactionID = *m.LedgerTransactionID
	}
	if m.CreditCardPurchaseID != nil {
		resp.CreditCardPurchaseID = *m.CreditCardPurchaseID
	}
	if m.InstallmentNumber != nil {
		resp.InstallmentNumber = *m.InstallmentNumber
	}
	if m.CancelsMovementID != nil {
		resp.CancelsMovementID = *m.CancelsMovementID
	}
	if m.ReversedByMovementID != nil {
		resp.ReversedByMovementID = *m.ReversedByMovementID
	}
	return resp
}

func toPurchaseResponse(p *entities.CreditCardPurchase, movements []*entities.Movement) interfacedto.CreditCardPurchaseResponse {
	resp := interfacedto.CreditCardPurchaseResponse{
		ID:               p.ID,
		UserID:           p.UserID,
		Description:      p.Description,
		Category:         string(p.Category),
		TotalAmount:      p.TotalAmount,
		Currency:         p.Currency,
		InstallmentCount: p.InstallmentCount,
		PurchaseDate:     p.PurchaseDate,
		Status:           string(p.Status),
	}
	for _, m := range movements {
		resp.Movements = append(resp.Movements, toMovementResponse(m))
	}
	return resp
}

func (h *movementHandler) writeUsecaseError(w http.ResponseWriter, action string, err error) {
	switch {
	case errors.Is(err, apperrors.ErrInvalidInput):
		h.writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, apperrors.ErrNotFound):
		h.writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, apperrors.ErrConflict):
		h.writeError(w, http.StatusConflict, "already cancelled")
	case errors.Is(err, apperrors.ErrUpstream):
		h.log.Error("%s failed: %v", action, err)
		h.writeError(w, http.StatusBadGateway, "upstream ledger service error")
	default:
		h.log.Error("%s failed: %v", action, err)
		h.writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func (h *movementHandler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.log.Error("failed to encode JSON response: %v", err)
	}
}

func (h *movementHandler) writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(interfacedto.ErrorResponse{Error: message}); err != nil {
		h.log.Error("failed to encode error response: %v", err)
	}
}
