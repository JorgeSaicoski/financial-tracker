package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/usecases"
	"github.com/JorgeSaicoski/financial-tracker/internal/interfaces/api/reqctx"
	interfacedto "github.com/JorgeSaicoski/financial-tracker/internal/interfaces/dto"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/logger"
)

// maxArchiveImportBytes bounds the archive body ImportArchive will decode.
// A full account archive (years of movements) is much larger than a CSV
// import, so this is well above jwks.go's 1 MiB discovery-body limit.
const maxArchiveImportBytes = 64 << 20 // 64 MiB

type archiveHandler struct {
	getSetting    usecases.GetLocalArchiveSettingUseCase
	setSetting    usecases.SetLocalArchiveSettingUseCase
	exportArchive usecases.ExportArchiveUseCase
	importArchive usecases.ImportArchiveUseCase

	log logger.Logger
}

// NewArchiveHandler returns interface type for dependency injection.
func NewArchiveHandler(
	getSetting usecases.GetLocalArchiveSettingUseCase,
	setSetting usecases.SetLocalArchiveSettingUseCase,
	exportArchive usecases.ExportArchiveUseCase,
	importArchive usecases.ImportArchiveUseCase,
	log logger.Logger,
) ArchiveHandler {
	return &archiveHandler{
		getSetting:    getSetting,
		setSetting:    setSetting,
		exportArchive: exportArchive,
		importArchive: importArchive,
		log:           log,
	}
}

// GetLocalArchiveSetting handles GET /settings/local-archive.
func (h *archiveHandler) GetLocalArchiveSetting(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		writeError(h.log, w, http.StatusUnauthorized, "unauthorized")
		return
	}

	enabled, err := h.getSetting.Execute(r.Context(), userID)
	if err != nil {
		h.writeUsecaseError(w, "get local archive setting", err)
		return
	}
	writeJSON(h.log, w, http.StatusOK, interfacedto.LocalArchiveSettingResponse{Enabled: enabled})
}

// SetLocalArchiveSetting handles PUT /settings/local-archive. Toggling
// this never touches cloud_storage_enabled (BACK-16) or deletes any
// server-side row — the two settings are independent.
func (h *archiveHandler) SetLocalArchiveSetting(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		writeError(h.log, w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req interfacedto.SetLocalArchiveSettingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(h.log, w, http.StatusBadRequest, "invalid request body")
		return
	}

	enabled, err := h.setSetting.Execute(r.Context(), userID, req.Enabled)
	if err != nil {
		h.writeUsecaseError(w, "set local archive setting", err)
		return
	}
	writeJSON(h.log, w, http.StatusOK, interfacedto.LocalArchiveSettingResponse{Enabled: enabled})
}

// ExportArchive handles GET /export/archive: the complete, restorable
// account state (BACK-15). The frontend encrypts this client-side before
// it ever leaves the page — this handler always returns plaintext JSON.
func (h *archiveHandler) ExportArchive(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		writeError(h.log, w, http.StatusUnauthorized, "unauthorized")
		return
	}

	bundle, err := h.exportArchive.Execute(r.Context(), userID)
	if err != nil {
		h.writeUsecaseError(w, "export archive", err)
		return
	}
	writeJSON(h.log, w, http.StatusOK, toArchiveResponse(userID, bundle))
}

// ImportArchive handles POST /import/archive: restores a previously
// exported (and, on the frontend, already-decrypted) archive. Safe to
// call more than once — rows that already exist are skipped, not
// duplicated or overwritten.
func (h *archiveHandler) ImportArchive(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		writeError(h.log, w, http.StatusUnauthorized, "unauthorized")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxArchiveImportBytes)

	var req interfacedto.ImportArchiveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(h.log, w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := h.importArchive.Execute(r.Context(), userID, fromArchiveRequest(req))
	if err != nil {
		h.writeUsecaseError(w, "import archive", err)
		return
	}
	writeJSON(h.log, w, http.StatusOK, interfacedto.ImportArchiveResponse{
		AccountsRestored:            result.AccountsRestored,
		AccountsSkipped:             result.AccountsSkipped,
		MovementsRestored:           result.MovementsRestored,
		MovementsSkipped:            result.MovementsSkipped,
		CreditCardPurchasesRestored: result.CreditCardPurchasesRestored,
		CreditCardPurchasesSkipped:  result.CreditCardPurchasesSkipped,
	})
}

func toArchiveResponse(userID string, bundle usecases.ArchiveBundle) interfacedto.ArchiveResponse {
	resp := interfacedto.ArchiveResponse{
		ExportedAt:          time.Now().UTC(),
		UserID:              userID,
		Accounts:            make([]interfacedto.ArchiveAccountDTO, 0, len(bundle.Accounts)),
		Movements:           make([]interfacedto.ArchiveMovementDTO, 0, len(bundle.Movements)),
		CreditCardPurchases: make([]interfacedto.ArchiveCreditCardPurchaseDTO, 0, len(bundle.CreditCardPurchases)),
	}
	for _, a := range bundle.Accounts {
		resp.Accounts = append(resp.Accounts, interfacedto.ArchiveAccountDTO{
			ID:        a.ID,
			UserID:    a.UserID,
			Name:      a.Name,
			Type:      a.Type,
			Currency:  a.Currency,
			CreatedAt: a.CreatedAt,
		})
	}
	for _, m := range bundle.Movements {
		resp.Movements = append(resp.Movements, toArchiveMovementDTO(m))
	}
	for _, p := range bundle.CreditCardPurchases {
		resp.CreditCardPurchases = append(resp.CreditCardPurchases, interfacedto.ArchiveCreditCardPurchaseDTO{
			ID:               p.ID,
			UserID:           p.UserID,
			Description:      p.Description,
			Category:         p.Category,
			TotalAmount:      p.TotalAmount,
			Currency:         p.Currency,
			InstallmentCount: p.InstallmentCount,
			PurchaseDate:     p.PurchaseDate,
			Status:           p.Status,
			CreatedAt:        p.CreatedAt,
		})
	}
	return resp
}

func toArchiveMovementDTO(m *dto.MovementDTO) interfacedto.ArchiveMovementDTO {
	out := interfacedto.ArchiveMovementDTO{
		ID:                m.ID,
		UserID:            m.UserID,
		Amount:            m.Amount,
		Currency:          m.Currency,
		Description:       m.Description,
		Category:          m.Category,
		PaymentMethod:     m.PaymentMethod,
		Status:            m.Status,
		Timestamp:         m.Timestamp,
		SyncStatus:        m.SyncStatus,
		SyncAttempts:      m.SyncAttempts,
		LastSyncAttemptAt: m.LastSyncAttemptAt,
		SyncedAt:          m.SyncedAt,
		CreatedAt:         m.CreatedAt,
	}
	if m.AccountID != nil {
		out.AccountID = *m.AccountID
	}
	if m.TransferID != nil {
		out.TransferID = *m.TransferID
	}
	if m.CreditCardPurchaseID != nil {
		out.CreditCardPurchaseID = *m.CreditCardPurchaseID
	}
	if m.InstallmentNumber != nil {
		out.InstallmentNumber = *m.InstallmentNumber
	}
	if m.CancelsMovementID != nil {
		out.CancelsMovementID = *m.CancelsMovementID
	}
	if m.ReversedByMovementID != nil {
		out.ReversedByMovementID = *m.ReversedByMovementID
	}
	if m.LedgerTransactionID != nil {
		out.LedgerTransactionID = *m.LedgerTransactionID
	}
	if m.LastSyncError != nil {
		out.LastSyncError = *m.LastSyncError
	}
	return out
}

func fromArchiveRequest(req interfacedto.ImportArchiveRequest) usecases.ArchiveBundle {
	bundle := usecases.ArchiveBundle{
		Accounts:            make([]*dto.AccountDTO, 0, len(req.Accounts)),
		Movements:           make([]*dto.MovementDTO, 0, len(req.Movements)),
		CreditCardPurchases: make([]*dto.CreditCardPurchaseDTO, 0, len(req.CreditCardPurchases)),
	}
	for _, a := range req.Accounts {
		bundle.Accounts = append(bundle.Accounts, &dto.AccountDTO{
			ID:        a.ID,
			UserID:    a.UserID,
			Name:      a.Name,
			Type:      a.Type,
			Currency:  a.Currency,
			CreatedAt: a.CreatedAt,
		})
	}
	for _, m := range req.Movements {
		bundle.Movements = append(bundle.Movements, fromArchiveMovementDTO(m))
	}
	for _, p := range req.CreditCardPurchases {
		bundle.CreditCardPurchases = append(bundle.CreditCardPurchases, &dto.CreditCardPurchaseDTO{
			ID:               p.ID,
			UserID:           p.UserID,
			Description:      p.Description,
			Category:         p.Category,
			TotalAmount:      p.TotalAmount,
			Currency:         p.Currency,
			InstallmentCount: p.InstallmentCount,
			PurchaseDate:     p.PurchaseDate,
			Status:           p.Status,
			CreatedAt:        p.CreatedAt,
		})
	}
	return bundle
}

func fromArchiveMovementDTO(m interfacedto.ArchiveMovementDTO) *dto.MovementDTO {
	out := &dto.MovementDTO{
		ID:                m.ID,
		UserID:            m.UserID,
		Amount:            m.Amount,
		Currency:          m.Currency,
		Description:       m.Description,
		Category:          m.Category,
		PaymentMethod:     m.PaymentMethod,
		Status:            m.Status,
		Timestamp:         m.Timestamp,
		SyncStatus:        m.SyncStatus,
		SyncAttempts:      m.SyncAttempts,
		LastSyncAttemptAt: m.LastSyncAttemptAt,
		SyncedAt:          m.SyncedAt,
		CreatedAt:         m.CreatedAt,
	}
	if m.AccountID != "" {
		out.AccountID = &m.AccountID
	}
	if m.TransferID != "" {
		out.TransferID = &m.TransferID
	}
	if m.CreditCardPurchaseID != "" {
		out.CreditCardPurchaseID = &m.CreditCardPurchaseID
	}
	if m.InstallmentNumber != 0 {
		out.InstallmentNumber = &m.InstallmentNumber
	}
	if m.CancelsMovementID != "" {
		out.CancelsMovementID = &m.CancelsMovementID
	}
	if m.ReversedByMovementID != "" {
		out.ReversedByMovementID = &m.ReversedByMovementID
	}
	if m.LedgerTransactionID != "" {
		out.LedgerTransactionID = &m.LedgerTransactionID
	}
	if m.LastSyncError != "" {
		out.LastSyncError = &m.LastSyncError
	}
	return out
}

func (h *archiveHandler) writeUsecaseError(w http.ResponseWriter, action string, err error) {
	switch {
	case apperrors.Is(err, apperrors.ErrInvalidInput):
		writeError(h.log, w, http.StatusBadRequest, err.Error())
	case apperrors.Is(err, apperrors.ErrNotFound):
		writeError(h.log, w, http.StatusNotFound, "not found")
	case apperrors.Is(err, apperrors.ErrConflict):
		writeError(h.log, w, http.StatusConflict, err.Error())
	default:
		h.log.Error("%s failed: %v", action, err)
		writeError(h.log, w, http.StatusInternalServerError, "internal error")
	}
}
