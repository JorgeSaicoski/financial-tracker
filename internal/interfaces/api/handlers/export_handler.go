package handlers

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/usecases"
	csvformat "github.com/JorgeSaicoski/financial-tracker/internal/infrastructure/csv"
	"github.com/JorgeSaicoski/financial-tracker/internal/interfaces/api/reqctx"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/logger"
)

type exportHandler struct {
	exportMovements usecases.ExportMovementsUseCase
	log             logger.Logger
}

// NewExportHandler returns interface type for dependency injection.
func NewExportHandler(exportMovements usecases.ExportMovementsUseCase, log logger.Logger) ExportHandler {
	return &exportHandler{exportMovements: exportMovements, log: log}
}

// ExportMovements handles GET /export/movements?include_cancelled=: a
// streaming CSV in exactly BACK-03's import model (same fixed header,
// same column order) so export -> import round-trips. Available in every
// mode, not just standalone (BACK-09) — data must always be portable.
// include_cancelled=true appends status/cancels_movement_id/
// reversed_by_movement_id columns and includes voided/reversal rows;
// the default excludes them so a re-import reads like real history.
func (h *exportHandler) ExportMovements(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		writeError(h.log, w, http.StatusUnauthorized, "unauthorized")
		return
	}

	includeCancelled, err := boolQueryParam(r, "include_cancelled")
	if err != nil {
		writeError(h.log, w, http.StatusBadRequest, "invalid include_cancelled (want true or false)")
		return
	}

	rows, err := h.exportMovements.Execute(r.Context(), userID, includeCancelled)
	if err != nil {
		writeUsecaseError(h.log, w, "export movements", err)
		return
	}

	filename := fmt.Sprintf("financial-tracker-export-%s.csv", time.Now().UTC().Format("2006-01-02"))
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.WriteHeader(http.StatusOK)

	writer := csv.NewWriter(w)
	header := append([]string{}, csvformat.Header...)
	if includeCancelled {
		header = append(header, "status", "cancels_movement_id", "reversed_by_movement_id")
	}
	if err := writer.Write(header); err != nil {
		h.log.Error("export movements: writing header failed: %v", err)
		return
	}
	for _, row := range rows {
		record := []string{
			row.Date,
			strconv.FormatInt(row.Amount, 10),
			row.Currency,
			row.Description,
			row.Category,
			row.PaymentMethod,
			row.Account,
		}
		if includeCancelled {
			record = append(record, row.Status, row.CancelsMovementID, row.ReversedByMovementID)
		}
		if err := writer.Write(record); err != nil {
			h.log.Error("export movements: writing row failed: %v", err)
			return
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		h.log.Error("export movements: flush failed: %v", err)
	}
}
