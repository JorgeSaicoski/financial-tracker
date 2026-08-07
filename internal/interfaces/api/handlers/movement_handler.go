package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/services"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/usecases"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
	"github.com/JorgeSaicoski/financial-tracker/internal/interfaces/api/reqctx"
	interfacedto "github.com/JorgeSaicoski/financial-tracker/internal/interfaces/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/logger"
)

type movementHandler struct {
	createMovement     usecases.CreateMovementUseCase
	createPurchase     usecases.CreateCreditCardPurchaseUseCase
	getMovement        usecases.GetMovementUseCase
	listMovements      usecases.ListMovementsUseCase
	updateMovement     usecases.UpdateMovementUseCase
	cancelMovement     usecases.CancelMovementUseCase
	cancelPurchase     usecases.CancelCreditCardPurchaseUseCase
	getCashflow        usecases.GetCashflowUseCase
	listPaymentMethods usecases.ListPaymentMethodsUseCase
	listCategories     usecases.ListCategoriesUseCase
	syncRunner         services.SyncRunner

	defaultCurrency string
	log             logger.Logger
}

// NewMovementHandler returns interface type for dependency injection.
func NewMovementHandler(
	createMovement usecases.CreateMovementUseCase,
	createPurchase usecases.CreateCreditCardPurchaseUseCase,
	getMovement usecases.GetMovementUseCase,
	listMovements usecases.ListMovementsUseCase,
	updateMovement usecases.UpdateMovementUseCase,
	cancelMovement usecases.CancelMovementUseCase,
	cancelPurchase usecases.CancelCreditCardPurchaseUseCase,
	getCashflow usecases.GetCashflowUseCase,
	listPaymentMethods usecases.ListPaymentMethodsUseCase,
	listCategories usecases.ListCategoriesUseCase,
	syncRunner services.SyncRunner,
	defaultCurrency string,
	log logger.Logger,
) MovementHandler {
	return &movementHandler{
		createMovement:     createMovement,
		createPurchase:     createPurchase,
		getMovement:        getMovement,
		listMovements:      listMovements,
		updateMovement:     updateMovement,
		cancelMovement:     cancelMovement,
		cancelPurchase:     cancelPurchase,
		getCashflow:        getCashflow,
		listPaymentMethods: listPaymentMethods,
		listCategories:     listCategories,
		syncRunner:         syncRunner,
		defaultCurrency:    defaultCurrency,
		log:                log,
	}
}

// CreateMovement handles POST /movements. When payment_method is
// "credit_card" and installments > 1 it creates an installment purchase
// (one purchase record + N monthly movements) instead of a single row.
func (h *movementHandler) CreateMovement(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		h.writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req interfacedto.CreateMovementRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	currency := req.Currency
	if currency == "" {
		currency = h.defaultCurrency
	}

	var accountID *string
	if req.AccountID != "" {
		accountID = &req.AccountID
	}
	var planID *string
	if req.PlanID != "" {
		planID = &req.PlanID
	}

	if req.Installments > 1 {
		if req.PaymentMethod != entities.PaymentMethodCreditCard {
			h.writeError(w, http.StatusBadRequest, "installments require payment_method \"credit_card\"")
			return
		}
		if accountID != nil {
			// Installments describe future credit-card bills, not money
			// already sitting in an account; supporting that needs a
			// card-account concept this MVP doesn't have yet.
			h.writeError(w, http.StatusBadRequest, "installment purchases can't be assigned to an account yet")
			return
		}
		purchase, movements, err := h.createPurchase.Execute(r.Context(), usecases.CreateCreditCardPurchaseInput{
			UserID:       userID,
			TotalAmount:  req.Amount,
			Currency:     currency,
			Description:  req.Description,
			CategoryID:   req.CategoryID,
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
		UserID:                      userID,
		Amount:                      req.Amount,
		Currency:                    currency,
		Description:                 req.Description,
		CategoryID:                  req.CategoryID,
		PaymentMethod:               req.PaymentMethod,
		AccountID:                   accountID,
		PlanID:                      planID,
		AvoidabilityOverridePercent: req.AvoidabilityOverridePercent,
	})
	if err != nil {
		h.writeUsecaseError(w, "create movement", err)
		return
	}

	h.writeJSON(w, http.StatusCreated, toMovementResponse(movement))
}

// GetMovement handles GET /movements?id=X. Scoped to the authenticated
// user: a movement id that exists but belongs to someone else is
// indistinguishable from one that doesn't exist — both 404 (BACK-02).
func (h *movementHandler) GetMovement(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		h.writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := r.URL.Query().Get("id")

	movement, err := h.getMovement.Execute(r.Context(), userID, id)
	if err != nil {
		h.writeUsecaseError(w, "get movement", err)
		return
	}

	h.writeJSON(w, http.StatusOK, toMovementResponse(movement))
}

// ListMovements handles GET /movements?currency=Y&from=&to=&limit=&offset=,
// always scoped to the authenticated user (BACK-02 — user_id is no longer
// an accepted query param). from/to accept RFC 3339 or YYYY-MM-DD (to is
// inclusive when date-only).
func (h *movementHandler) ListMovements(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		h.writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var currency *string
	if c := r.URL.Query().Get("currency"); c != "" {
		currency = &c
	}

	from, err := parseTimeParam(r, "from", false)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid from (want YYYY-MM-DD or RFC 3339)")
		return
	}
	to, err := parseTimeParam(r, "to", true)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid to (want YYYY-MM-DD or RFC 3339)")
		return
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

	result, err := h.listMovements.Execute(r.Context(), userID, currency, from, to, limit, offset)
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

// UpdateMovement handles PATCH /movements/{id}. Editing an already-synced
// movement's amount/currency/timestamp produces a reversal + a
// replacement instead of an in-place edit (ledger-service never deletes);
// the response's reversal/replacement fields tell the UI which happened.
func (h *movementHandler) UpdateMovement(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		h.writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req interfacedto.UpdateMovementRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	input := usecases.UpdateMovementInput{
		Description:                 req.Description,
		CategoryID:                  req.CategoryID,
		PaymentMethod:               req.PaymentMethod,
		AccountID:                   req.AccountID,
		PlanID:                      req.PlanID,
		Amount:                      req.Amount,
		Currency:                    req.Currency,
		Timestamp:                   req.Timestamp,
		AvoidabilityOverridePercent: req.AvoidabilityOverridePercent,
	}

	result, err := h.updateMovement.Execute(r.Context(), userID, r.PathValue("id"), input)
	if err != nil {
		h.writeUsecaseError(w, "update movement", err)
		return
	}

	resp := interfacedto.UpdateMovementResponse{Movement: toMovementResponse(result.Movement)}
	if result.Reversal != nil {
		reversal := toMovementResponse(result.Reversal)
		resp.Reversal = &reversal
	}
	if result.Replacement != nil {
		replacement := toMovementResponse(result.Replacement)
		resp.Replacement = &replacement
	}
	h.writeJSON(w, http.StatusOK, resp)
}

// CancelMovement handles POST /movements/{id}/cancel
func (h *movementHandler) CancelMovement(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		h.writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	result, err := h.cancelMovement.Execute(r.Context(), userID, r.PathValue("id"))
	if err != nil {
		h.writeUsecaseError(w, "cancel movement", err)
		return
	}

	h.writeJSON(w, http.StatusOK, toCancelMovementResponse(result))
}

// CancelCreditCardPurchase handles POST /credit-card-purchases/{id}/cancel
func (h *movementHandler) CancelCreditCardPurchase(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		h.writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	result, err := h.cancelPurchase.Execute(r.Context(), userID, r.PathValue("id"))
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

// Cashflow handles GET /cashflow?from=&to=: money in / money out / net
// over the interval, per currency and per account, scoped to the
// authenticated user (BACK-02). from/to accept RFC 3339 or YYYY-MM-DD
// (to is inclusive when date-only).
func (h *movementHandler) Cashflow(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		h.writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	from, err := parseTimeParam(r, "from", false)
	if err != nil || from == nil {
		h.writeError(w, http.StatusBadRequest, "from is required (YYYY-MM-DD or RFC 3339)")
		return
	}
	to, err := parseTimeParam(r, "to", true)
	if err != nil || to == nil {
		h.writeError(w, http.StatusBadRequest, "to is required (YYYY-MM-DD or RFC 3339)")
		return
	}

	result, err := h.getCashflow.Execute(r.Context(), userID, *from, *to)
	if err != nil {
		h.writeUsecaseError(w, "get cashflow", err)
		return
	}

	resp := interfacedto.CashflowResponse{
		From:      result.From,
		To:        result.To,
		Totals:    make([]interfacedto.CurrencyFlowDTO, 0, len(result.Totals)),
		ByAccount: make([]interfacedto.AccountFlowDTO, 0, len(result.ByAccount)),
	}
	for _, t := range result.Totals {
		resp.Totals = append(resp.Totals, interfacedto.CurrencyFlowDTO{
			Currency: t.Currency, In: t.In, Out: t.Out, Net: t.Net,
		})
	}
	for _, f := range result.ByAccount {
		resp.ByAccount = append(resp.ByAccount, interfacedto.AccountFlowDTO{
			AccountID: f.AccountID, Name: f.Name, Currency: f.Currency,
			In: f.In, Out: f.Out, Net: f.Net,
		})
	}
	h.writeJSON(w, http.StatusOK, resp)
}

// ListCategories handles GET /categories: the full global category
// registry (BACK-14 follow-up — id, name, avoidability_percent,
// is_contributor relative to the caller) plus the caller's own
// payment-method registry (BACK-17 — id, name; system rows
// "credit_card"/"bank_transfer" and the ordinary first-run defaults
// lazily ensured first).
func (h *movementHandler) ListCategories(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		h.writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	categoryRows, err := h.listCategories.Execute(r.Context())
	if err != nil {
		h.writeUsecaseError(w, "list categories", err)
		return
	}
	categories := make([]interfacedto.CategoryResponse, 0, len(categoryRows))
	for _, c := range categoryRows {
		categories = append(categories, toCategoryResponse(c, userID))
	}

	methodRows, err := h.listPaymentMethods.Execute(r.Context(), userID)
	if err != nil {
		h.writeUsecaseError(w, "list payment methods", err)
		return
	}
	methods := make([]interfacedto.PaymentMethodResponse, 0, len(methodRows))
	for _, m := range methodRows {
		methods = append(methods, toPaymentMethodResponse(m))
	}

	h.writeJSON(w, http.StatusOK, interfacedto.CategoriesResponse{
		Categories:     categories,
		PaymentMethods: methods,
	})
}

func toPaymentMethodResponse(m *dto.PaymentMethodDTO) interfacedto.PaymentMethodResponse {
	return interfacedto.PaymentMethodResponse{
		ID:   m.ID,
		Name: m.Name,
	}
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

func toMovementResponse(m *dto.MovementDTO) interfacedto.MovementResponse {
	resp := interfacedto.MovementResponse{
		ID:                          m.ID,
		UserID:                      m.UserID,
		Amount:                      m.Amount,
		Currency:                    m.Currency,
		Description:                 m.Description,
		Category:                    m.Category,
		PaymentMethod:               m.PaymentMethod,
		AvoidabilityOverridePercent: m.AvoidabilityOverridePercent,
		Status:                      m.Status,
		SyncStatus:                  m.SyncStatus,
		Timestamp:                   m.Timestamp,
	}
	if m.CategoryID != nil {
		resp.CategoryID = *m.CategoryID
	}
	if m.AccountID != nil {
		resp.AccountID = *m.AccountID
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
	if m.TransferID != nil {
		resp.TransferID = *m.TransferID
	}
	if m.PlanID != nil {
		resp.PlanID = *m.PlanID
	}
	if m.RecurringRuleID != nil {
		resp.RecurringRuleID = *m.RecurringRuleID
	}
	return resp
}

// toCancelMovementResponse is shared by MovementHandler.CancelMovement and
// TransferHandler.CancelTransfer, since a transfer's cancel result is just
// one of these per leg.
func toCancelMovementResponse(result usecases.CancelMovementResult) interfacedto.CancelMovementResponse {
	resp := interfacedto.CancelMovementResponse{Movement: toMovementResponse(result.Movement)}
	if result.Reversal != nil {
		reversal := toMovementResponse(result.Reversal)
		resp.Reversal = &reversal
	}
	return resp
}

func toPurchaseResponse(p *dto.CreditCardPurchaseDTO, movements []*dto.MovementDTO) interfacedto.CreditCardPurchaseResponse {
	resp := interfacedto.CreditCardPurchaseResponse{
		ID:               p.ID,
		UserID:           p.UserID,
		Description:      p.Description,
		Category:         p.Category,
		TotalAmount:      p.TotalAmount,
		Currency:         p.Currency,
		InstallmentCount: p.InstallmentCount,
		PurchaseDate:     p.PurchaseDate,
		Status:           p.Status,
	}
	if p.CategoryID != nil {
		resp.CategoryID = *p.CategoryID
	}
	for _, m := range movements {
		resp.Movements = append(resp.Movements, toMovementResponse(m))
	}
	return resp
}

func (h *movementHandler) writeUsecaseError(w http.ResponseWriter, action string, err error) {
	writeUsecaseError(h.log, w, action, err)
}

func (h *movementHandler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	writeJSON(h.log, w, status, data)
}

func (h *movementHandler) writeError(w http.ResponseWriter, status int, message string) {
	writeError(h.log, w, status, message)
}
