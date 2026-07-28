package handlers

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/usecases"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
	csvformat "github.com/JorgeSaicoski/financial-tracker/internal/infrastructure/csv"
	"github.com/JorgeSaicoski/financial-tracker/internal/interfaces/api/reqctx"
	interfacedto "github.com/JorgeSaicoski/financial-tracker/internal/interfaces/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/logger"
)

const (
	// maxImportFileBytes bounds the request body BACK-03 asks for
	// ("max file size (e.g. 1 MB)").
	maxImportFileBytes = 1 << 20 // 1 MiB
	// maxImportRows bounds row count ("max rows (e.g. 10k)").
	maxImportRows = 10_000
)

type importHandler struct {
	importMovements usecases.ImportMovementsUseCase
	listAccounts    usecases.ListAccountsUseCase
	listCurrencies  usecases.ListCurrenciesUseCase

	log logger.Logger
}

// NewImportHandler returns interface type for dependency injection.
func NewImportHandler(
	importMovements usecases.ImportMovementsUseCase,
	listAccounts usecases.ListAccountsUseCase,
	listCurrencies usecases.ListCurrenciesUseCase,
	log logger.Logger,
) ImportHandler {
	return &importHandler{
		importMovements: importMovements,
		listAccounts:    listAccounts,
		listCurrencies:  listCurrencies,
		log:             log,
	}
}

// GetImportSpec handles GET /import/movements/spec: the CSV model's
// columns (with real allowed values, not hardcoded in the frontend) plus
// a ready-to-copy template.
func (h *importHandler) GetImportSpec(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		h.writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	currencies, err := h.listCurrencies.Execute(r.Context())
	if err != nil {
		h.log.Error("import spec: list currencies failed: %v", err)
		writeError(h.log, w, http.StatusInternalServerError, "internal error")
		return
	}
	accountViews, err := h.listAccounts.Execute(r.Context(), userID)
	if err != nil {
		h.log.Error("import spec: list accounts failed: %v", err)
		writeError(h.log, w, http.StatusInternalServerError, "internal error")
		return
	}
	accountNames := make([]string, 0, len(accountViews))
	for _, a := range accountViews {
		accountNames = append(accountNames, a.Account.Name)
	}

	categories := make([]string, 0, len(entities.Categories()))
	for _, c := range entities.Categories() {
		categories = append(categories, string(c))
	}
	paymentMethods := make([]string, 0, len(entities.PaymentMethods()))
	for _, m := range entities.PaymentMethods() {
		paymentMethods = append(paymentMethods, string(m))
	}

	exampleCurrency := "usd"
	if len(currencies) > 0 {
		exampleCurrency = currencies[0]
	}

	writeJSON(h.log, w, http.StatusOK, interfacedto.ImportSpecResponse{
		Columns: []interfacedto.ImportSpecColumn{
			{Name: "date", Type: "date (YYYY-MM-DD)", Required: true},
			{Name: "amount", Type: "integer (smallest currency unit, sign = direction, != 0)", Required: true},
			{Name: "currency", Type: "string", Required: true, AllowedValues: currencies},
			{Name: "description", Type: "string", Required: false},
			{Name: "category", Type: "string", Required: false, AllowedValues: categories},
			{Name: "payment_method", Type: "string", Required: false, AllowedValues: paymentMethods},
			{Name: "account", Type: "string (account name, case-insensitive)", Required: false, AllowedValues: accountNames},
		},
		Template: csvformat.BuildTemplate(exampleCurrency),
	})
}

// ImportMovements handles POST /import/movements: multipart file (field
// "file") or a raw text/csv body. Query params: dry_run, allow_partial,
// skip_duplicates (all "true"/"false", default false).
func (h *importHandler) ImportMovements(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		h.writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	dryRun, err := boolQueryParam(r, "dry_run")
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid dry_run (want true or false)")
		return
	}
	allowPartial, err := boolQueryParam(r, "allow_partial")
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid allow_partial (want true or false)")
		return
	}
	skipDuplicates, err := boolQueryParam(r, "skip_duplicates")
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid skip_duplicates (want true or false)")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxImportFileBytes)
	reader, err := csvReaderFromRequest(r)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			h.writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("file too large (max %d bytes)", maxImportFileBytes))
			return
		}
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	csvRows, err := csvformat.ParseRows(reader)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			h.writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("file too large (max %d bytes)", maxImportFileBytes))
			return
		}
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(csvRows) > maxImportRows {
		h.writeError(w, http.StatusBadRequest, fmt.Sprintf("too many rows (max %d)", maxImportRows))
		return
	}
	rows := make([]usecases.ImportRowInput, len(csvRows))
	for i, cr := range csvRows {
		rows[i] = usecases.ImportRowInput(cr)
	}

	result, err := h.importMovements.Execute(r.Context(), usecases.ImportMovementsInput{
		UserID:         userID,
		Rows:           rows,
		DryRun:         dryRun,
		AllowPartial:   allowPartial,
		SkipDuplicates: skipDuplicates,
	})
	if err != nil {
		h.writeUsecaseError(w, "import movements", err)
		return
	}

	h.writeJSON(w, http.StatusOK, toImportResponse(result))
}

// csvReaderFromRequest returns the uploaded CSV's bytes as a reader,
// whichever of the two accepted shapes the request used: a multipart
// file upload (field "file") or a raw text/csv body. The multipart file
// handle is read fully and closed here rather than handed back open —
// callers only need an io.Reader, and leaving a multipart.File open for
// the rest of the request risks leaking file descriptors/temp files.
func csvReaderFromRequest(r *http.Request) (io.Reader, error) {
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/") {
		file, _, err := r.FormFile("file")
		if err != nil {
			return nil, fmt.Errorf("missing multipart \"file\" field: %w", err)
		}
		defer file.Close()
		data, err := io.ReadAll(file) // bounded by the MaxBytesReader already wrapping r.Body
		if err != nil {
			return nil, err
		}
		return bytes.NewReader(data), nil
	}
	return r.Body, nil
}

func toImportResponse(result usecases.ImportMovementsResult) interfacedto.ImportMovementsResponse {
	errs := make([]interfacedto.ImportRowErrorResponse, 0, len(result.Errors))
	for _, e := range result.Errors {
		errs = append(errs, interfacedto.ImportRowErrorResponse{Row: e.Row, Field: e.Field, Message: e.Message})
	}
	dupes := make([]interfacedto.ImportDuplicateResponse, 0, len(result.Duplicates))
	for _, d := range result.Duplicates {
		dupes = append(dupes, interfacedto.ImportDuplicateResponse{
			Row:                d.Row,
			ExistingMovementID: d.ExistingMovementID,
			DuplicateOfRow:     d.DuplicateOfRow,
		})
	}
	return interfacedto.ImportMovementsResponse{
		Imported:   result.Imported,
		Skipped:    result.Skipped,
		Errors:     errs,
		Duplicates: dupes,
	}
}

// boolQueryParam parses a "true"/"false" query param, defaulting to
// false when absent.
func boolQueryParam(r *http.Request, name string) (bool, error) {
	raw := r.URL.Query().Get(name)
	switch raw {
	case "", "false":
		return false, nil
	case "true":
		return true, nil
	default:
		return false, errInvalidParam
	}
}

func (h *importHandler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	writeJSON(h.log, w, status, data)
}

func (h *importHandler) writeError(w http.ResponseWriter, status int, message string) {
	writeError(h.log, w, status, message)
}

func (h *importHandler) writeUsecaseError(w http.ResponseWriter, action string, err error) {
	writeUsecaseError(h.log, w, action, err)
}
