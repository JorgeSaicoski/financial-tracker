package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/usecases"
	"github.com/JorgeSaicoski/financial-tracker/internal/interfaces/api/reqctx"
	interfacedto "github.com/JorgeSaicoski/financial-tracker/internal/interfaces/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/logger"
)

type settingsHandler struct {
	getSettings    usecases.GetUserSettingsUseCase
	updateSettings usecases.UpdateUserSettingsUseCase

	log logger.Logger
}

// NewSettingsHandler returns interface type for dependency injection.
func NewSettingsHandler(
	getSettings usecases.GetUserSettingsUseCase,
	updateSettings usecases.UpdateUserSettingsUseCase,
	log logger.Logger,
) SettingsHandler {
	return &settingsHandler{
		getSettings:    getSettings,
		updateSettings: updateSettings,
		log:            log,
	}
}

// GetSettings handles GET /settings: the caller's own settings,
// entitlements included (read-only through this API).
func (h *settingsHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		h.writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	settings, err := h.getSettings.Execute(r.Context(), userID)
	if err != nil {
		h.writeUsecaseError(w, "get settings", err)
		return
	}
	h.writeJSON(w, http.StatusOK, settingsResponseFromView(settings))
}

// PatchSettings handles PATCH /settings. Body:
// {"ledger_sync_enabled": bool, "default_category_id": string} — either
// or both fields may be set; DisallowUnknownFields rejects an attempt to
// set either entitlement field (or anything else) with 400, per BACK-13's
// acceptance criteria.
func (h *settingsHandler) PatchSettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		h.writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req interfacedto.PatchSettingsRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body (only ledger_sync_enabled/default_category_id may be set)")
		return
	}
	if req.LedgerSyncEnabled == nil && req.DefaultCategoryID == nil {
		h.writeError(w, http.StatusBadRequest, "ledger_sync_enabled or default_category_id is required")
		return
	}

	settings, err := h.updateSettings.Execute(r.Context(), userID, usecases.UpdateUserSettingsInput{
		LedgerSyncEnabled: req.LedgerSyncEnabled,
		DefaultCategoryID: req.DefaultCategoryID,
	})
	if err != nil {
		h.writeUsecaseError(w, "update settings", err)
		return
	}
	h.writeJSON(w, http.StatusOK, settingsResponseFromView(settings))
}

func settingsResponseFromView(s usecases.UserSettingsView) interfacedto.SettingsResponse {
	resp := interfacedto.SettingsResponse{
		UserID:                       s.UserID,
		LedgerSyncEntitled:           s.LedgerSyncEntitled,
		LedgerSyncEnabled:            s.LedgerSyncEnabled,
		CloudStorageEntitled:         s.CloudStorageEntitled,
		CreatedAt:                    s.CreatedAt,
		UpdatedAt:                    s.UpdatedAt,
		SubscriptionStatus:           s.SubscriptionStatus,
		SubscriptionCurrentPeriodEnd: s.SubscriptionCurrentPeriodEnd,
	}
	if s.DefaultCategoryID != nil {
		resp.DefaultCategoryID = *s.DefaultCategoryID
	}
	return resp
}

func (h *settingsHandler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	writeJSON(h.log, w, status, data)
}

func (h *settingsHandler) writeError(w http.ResponseWriter, status int, message string) {
	writeError(h.log, w, status, message)
}

func (h *settingsHandler) writeUsecaseError(w http.ResponseWriter, action string, err error) {
	writeUsecaseError(h.log, w, action, err)
}
