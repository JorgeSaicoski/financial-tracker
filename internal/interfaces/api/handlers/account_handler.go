package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/usecases"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
	"github.com/JorgeSaicoski/financial-tracker/internal/interfaces/api/reqctx"
	interfacedto "github.com/JorgeSaicoski/financial-tracker/internal/interfaces/dto"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/logger"
)

type accountHandler struct {
	createAccount usecases.CreateAccountUseCase
	listAccounts  usecases.ListAccountsUseCase
	reportBalance usecases.ReportAccountBalanceUseCase
	listSnapshots usecases.ListAccountSnapshotsUseCase

	log logger.Logger
}

// NewAccountHandler returns interface type for dependency injection.
func NewAccountHandler(
	createAccount usecases.CreateAccountUseCase,
	listAccounts usecases.ListAccountsUseCase,
	reportBalance usecases.ReportAccountBalanceUseCase,
	listSnapshots usecases.ListAccountSnapshotsUseCase,
	log logger.Logger,
) AccountHandler {
	return &accountHandler{
		createAccount: createAccount,
		listAccounts:  listAccounts,
		reportBalance: reportBalance,
		listSnapshots: listSnapshots,
		log:           log,
	}
}

// CreateAccount handles POST /accounts.
func (h *accountHandler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		writeError(h.log, w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req interfacedto.CreateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(h.log, w, http.StatusBadRequest, "invalid request body")
		return
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
// balance, last reported balance and last computed return, scoped to the
// authenticated user (BACK-02).
func (h *accountHandler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		writeError(h.log, w, http.StatusUnauthorized, "unauthorized")
		return
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
// what the account really holds right now (or, with an explicit
// timestamp, held on an earlier date — backfilling a past report).
// Scoped to the authenticated user (BACK-02) — {id} must be one of their
// own accounts.
func (h *accountHandler) ReportBalance(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		writeError(h.log, w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req interfacedto.ReportBalanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Balance == nil {
		writeError(h.log, w, http.StatusBadRequest, "balance is required (smallest currency unit)")
		return
	}

	var timestamp time.Time
	if req.Timestamp != nil {
		timestamp = *req.Timestamp
	}

	view, err := h.reportBalance.Execute(r.Context(), userID, r.PathValue("id"), *req.Balance, timestamp)
	if err != nil {
		h.writeUsecaseError(w, "report balance", err)
		return
	}
	writeJSON(h.log, w, http.StatusOK, toAccountResponse(view))
}

// ListSnapshots handles GET /accounts/{id}/balance: the account's full
// reported-balance history, each entry paired with its own return.
// Scoped to the authenticated user (BACK-02) — {id} must be one of their
// own accounts.
func (h *accountHandler) ListSnapshots(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		writeError(h.log, w, http.StatusUnauthorized, "unauthorized")
		return
	}

	views, err := h.listSnapshots.Execute(r.Context(), userID, r.PathValue("id"))
	if err != nil {
		h.writeUsecaseError(w, "list account snapshots", err)
		return
	}

	resp := interfacedto.AccountSnapshotsResponse{Snapshots: make([]interfacedto.AccountSnapshotResponse, 0, len(views))}
	for _, v := range views {
		resp.Snapshots = append(resp.Snapshots, interfacedto.AccountSnapshotResponse{
			ID:         v.Snapshot.ID,
			Balance:    v.Snapshot.Balance,
			Timestamp:  v.Snapshot.Timestamp,
			Return:     v.Return,
			ReturnFrom: v.ReturnFrom,
			ReturnTo:   v.ReturnTo,
			CreatedAt:  v.Snapshot.CreatedAt,
		})
	}
	writeJSON(h.log, w, http.StatusOK, resp)
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
