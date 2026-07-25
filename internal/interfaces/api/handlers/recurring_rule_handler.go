package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/usecases"
	interfacedto "github.com/JorgeSaicoski/financial-tracker/internal/interfaces/dto"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/logger"
)

type recurringRuleHandler struct {
	createRule usecases.CreateRecurringRuleUseCase
	listRules  usecases.ListRecurringRulesUseCase
	updateRule usecases.UpdateRecurringRuleUseCase

	defaultUserID string
	log           logger.Logger
}

// NewRecurringRuleHandler returns interface type for dependency injection.
func NewRecurringRuleHandler(
	createRule usecases.CreateRecurringRuleUseCase,
	listRules usecases.ListRecurringRulesUseCase,
	updateRule usecases.UpdateRecurringRuleUseCase,
	defaultUserID string,
	log logger.Logger,
) RecurringRuleHandler {
	return &recurringRuleHandler{
		createRule:    createRule,
		listRules:     listRules,
		updateRule:    updateRule,
		defaultUserID: defaultUserID,
		log:           log,
	}
}

// CreateRecurringRule handles POST /recurring-rules.
func (h *recurringRuleHandler) CreateRecurringRule(w http.ResponseWriter, r *http.Request) {
	var req interfacedto.CreateRecurringRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(h.log, w, http.StatusBadRequest, "invalid request body")
		return
	}

	userID := req.UserID
	if userID == "" {
		userID = h.defaultUserID
	}
	var accountID *string
	if req.AccountID != "" {
		accountID = &req.AccountID
	}

	input := usecases.CreateRecurringRuleInput{
		UserID:        userID,
		Amount:        req.Amount,
		Currency:      req.Currency,
		Description:   req.Description,
		Category:      req.Category,
		PaymentMethod: req.PaymentMethod,
		AccountID:     accountID,
		DayOfMonth:    req.DayOfMonth,
	}
	if req.StartsAt != nil {
		input.StartsAt = *req.StartsAt
	}
	input.EndsAt = req.EndsAt

	rule, err := h.createRule.Execute(r.Context(), input)
	if err != nil {
		h.writeUsecaseError(w, "create recurring rule", err)
		return
	}
	writeJSON(h.log, w, http.StatusCreated, toRecurringRuleResponse(rule))
}

// ListRecurringRules handles GET /recurring-rules.
func (h *recurringRuleHandler) ListRecurringRules(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		userID = h.defaultUserID
	}

	rules, err := h.listRules.Execute(r.Context(), userID)
	if err != nil {
		h.writeUsecaseError(w, "list recurring rules", err)
		return
	}

	resp := interfacedto.ListRecurringRulesResponse{RecurringRules: make([]interfacedto.RecurringRuleResponse, 0, len(rules))}
	for _, rule := range rules {
		resp.RecurringRules = append(resp.RecurringRules, toRecurringRuleResponse(rule))
	}
	writeJSON(h.log, w, http.StatusOK, resp)
}

// UpdateRecurringRule handles PATCH /recurring-rules/{id}.
func (h *recurringRuleHandler) UpdateRecurringRule(w http.ResponseWriter, r *http.Request) {
	var req interfacedto.UpdateRecurringRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(h.log, w, http.StatusBadRequest, "invalid request body")
		return
	}

	rule, err := h.updateRule.Execute(r.Context(), r.PathValue("id"), usecases.UpdateRecurringRuleInput{
		Description:   req.Description,
		Category:      req.Category,
		PaymentMethod: req.PaymentMethod,
		AccountID:     req.AccountID,
		Amount:        req.Amount,
		Currency:      req.Currency,
		DayOfMonth:    req.DayOfMonth,
		EndsAt:        req.EndsAt,
		Active:        req.Active,
	})
	if err != nil {
		h.writeUsecaseError(w, "update recurring rule", err)
		return
	}
	writeJSON(h.log, w, http.StatusOK, toRecurringRuleResponse(rule))
}

func toRecurringRuleResponse(r *dto.RecurringRuleDTO) interfacedto.RecurringRuleResponse {
	resp := interfacedto.RecurringRuleResponse{
		ID:              r.ID,
		UserID:          r.UserID,
		Amount:          r.Amount,
		Currency:        r.Currency,
		Description:     r.Description,
		Category:        r.Category,
		PaymentMethod:   r.PaymentMethod,
		DayOfMonth:      r.DayOfMonth,
		StartsAt:        r.StartsAt,
		EndsAt:          r.EndsAt,
		Active:          r.Active,
		LastGeneratedAt: r.LastGeneratedAt,
		CreatedAt:       r.CreatedAt,
	}
	if r.AccountID != nil {
		resp.AccountID = *r.AccountID
	}
	return resp
}

func (h *recurringRuleHandler) writeUsecaseError(w http.ResponseWriter, action string, err error) {
	switch {
	case apperrors.Is(err, apperrors.ErrInvalidInput):
		writeError(h.log, w, http.StatusBadRequest, err.Error())
	case apperrors.Is(err, apperrors.ErrNotFound):
		writeError(h.log, w, http.StatusNotFound, "recurring rule not found")
	case apperrors.Is(err, apperrors.ErrConflict):
		writeError(h.log, w, http.StatusConflict, err.Error())
	default:
		h.log.Error("%s failed: %v", action, err)
		writeError(h.log, w, http.StatusInternalServerError, "internal error")
	}
}
