package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/JorgeSaicoski/financial-tracker/application/usecases"
	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
	interfacedto "github.com/JorgeSaicoski/financial-tracker/interfaces/dto"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
	"github.com/JorgeSaicoski/financial-tracker/pkg/logger"
)

// AccountHandler exposes the accounts API: the places money sits, plus
// user-reported balances that let us compute each account's return.
type AccountHandler interface {
	CreateAccount(w http.ResponseWriter, r *http.Request)
	ListAccounts(w http.ResponseWriter, r *http.Request)
	ReportBalance(w http.ResponseWriter, r *http.Request)
}

type accountHandler struct {
	createAccount usecases.CreateAccountUseCase
	listAccounts  usecases.ListAccountsUseCase
	reportBalance usecases.ReportAccountBalanceUseCase

	defaultUserID string
	log           logger.Logger
}

// NewAccountHandler returns interface type for dependency injection.
func NewAccountHandler(
	createAccount usecases.CreateAccountUseCase,
	listAccounts usecases.ListAccountsUseCase,
	reportBalance usecases.ReportAccountBalanceUseCase,
	defaultUserID string,
	log logger.Logger,
) AccountHandler {
	return &accountHandler{
		createAccount: createAccount,
		listAccounts:  listAccounts,
		reportBalance: reportBalance,
		defaultUserID: defaultUserID,
		log:           log,
	}
}

// CreateAccount handles POST /accounts.
func (h *accountHandler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var req interfacedto.CreateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(h.log, w, http.StatusBadRequest, "invalid request body")
		return
	}

	userID := req.UserID
	if userID == "" {
		userID = h.defaultUserID
	}

	account, err := h.createAccount.Execute(r.Context(), usecases.CreateAccountInput{
		UserID:   userID,
		Name:     req.Name,
		Type:     req.Type,
		Currency: req.Currency,
	})
	if err != nil {
		h.writeUsecaseError(w, "create account", err)
		return
	}

	// A brand-new account has no movements or snapshots: its view is
	// all zeros, no lookups needed.
	writeJSON(h.log, w, http.StatusCreated, toAccountResponse(usecases.AccountView{Account: account}))
}

// ListAccounts handles GET /accounts: every account with its estimated
// balance, last reported balance and last computed return.
func (h *accountHandler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		userID = h.defaultUserID
	}

	views, err := h.listAccounts.Execute(r.Context(), userID)
	if err != nil {
		h.writeUsecaseError(w, "list accounts", err)
		return
	}

	resp := interfacedto.AccountsResponse{
		Accounts:     make([]interfacedto.AccountResponse, 0, len(views)),
		AccountTypes: make([]string, 0),
	}
	for _, v := range views {
		resp.Accounts = append(resp.Accounts, toAccountResponse(v))
	}
	for _, t := range entities.AccountTypes() {
		resp.AccountTypes = append(resp.AccountTypes, string(t))
	}
	writeJSON(h.log, w, http.StatusOK, resp)
}

// ReportBalance handles POST /accounts/{id}/balance: the user reports
// what the account really holds right now.
func (h *accountHandler) ReportBalance(w http.ResponseWriter, r *http.Request) {
	var req interfacedto.ReportBalanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Balance == nil {
		writeError(h.log, w, http.StatusBadRequest, "balance is required (smallest currency unit)")
		return
	}

	view, err := h.reportBalance.Execute(r.Context(), r.PathValue("id"), *req.Balance)
	if err != nil {
		h.writeUsecaseError(w, "report balance", err)
		return
	}
	writeJSON(h.log, w, http.StatusOK, toAccountResponse(view))
}

func toAccountResponse(v usecases.AccountView) interfacedto.AccountResponse {
	return interfacedto.AccountResponse{
		ID:                   v.Account.ID,
		UserID:               v.Account.UserID,
		Name:                 v.Account.Name,
		Type:                 v.Account.Type,
		Currency:             v.Account.Currency,
		CreatedAt:            v.Account.CreatedAt,
		EstimatedBalance:     v.EstimatedBalance,
		MovementsSinceReport: v.MovementsSinceReport,
		ReportedBalance:      v.ReportedBalance,
		ReportedAt:           v.ReportedAt,
		LastReturn:           v.LastReturn,
		LastReturnFrom:       v.LastReturnFrom,
		LastReturnTo:         v.LastReturnTo,
	}
}

func (h *accountHandler) writeUsecaseError(w http.ResponseWriter, action string, err error) {
	switch {
	case apperrors.Is(err, apperrors.ErrInvalidInput):
		writeError(h.log, w, http.StatusBadRequest, err.Error())
	case apperrors.Is(err, apperrors.ErrNotFound):
		writeError(h.log, w, http.StatusNotFound, "account not found")
	case apperrors.Is(err, apperrors.ErrConflict):
		writeError(h.log, w, http.StatusConflict, err.Error())
	default:
		h.log.Error("%s failed: %v", action, err)
		writeError(h.log, w, http.StatusInternalServerError, "internal error")
	}
}
