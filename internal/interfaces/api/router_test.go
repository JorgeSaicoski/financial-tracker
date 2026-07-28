package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/JorgeSaicoski/financial-tracker/internal/interfaces/api/handlers"
)

// stubHandlers implements every handler interface NewRouter takes, each
// method just identifying itself in the response body — enough to prove
// routing reached (or didn't reach) a given handler, without pulling in
// any real usecase/repository wiring.
type stubHandlers struct{}

func (stubHandlers) reply(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(name))
	}
}

func (s stubHandlers) CreateMovement(w http.ResponseWriter, r *http.Request) {
	s.reply("CreateMovement")(w, r)
}
func (s stubHandlers) GetMovement(w http.ResponseWriter, r *http.Request) {
	s.reply("GetMovement")(w, r)
}
func (s stubHandlers) ListMovements(w http.ResponseWriter, r *http.Request) {
	s.reply("ListMovements")(w, r)
}
func (s stubHandlers) UpdateMovement(w http.ResponseWriter, r *http.Request) {
	s.reply("UpdateMovement")(w, r)
}
func (s stubHandlers) CancelMovement(w http.ResponseWriter, r *http.Request) {
	s.reply("CancelMovement")(w, r)
}
func (s stubHandlers) CancelCreditCardPurchase(w http.ResponseWriter, r *http.Request) {
	s.reply("CancelCreditCardPurchase")(w, r)
}
func (s stubHandlers) Sync(w http.ResponseWriter, r *http.Request) { s.reply("Sync")(w, r) }
func (s stubHandlers) ListCategories(w http.ResponseWriter, r *http.Request) {
	s.reply("ListCategories")(w, r)
}
func (s stubHandlers) Cashflow(w http.ResponseWriter, r *http.Request) { s.reply("Cashflow")(w, r) }

func (s stubHandlers) CreateAccount(w http.ResponseWriter, r *http.Request) {
	s.reply("CreateAccount")(w, r)
}
func (s stubHandlers) ListAccounts(w http.ResponseWriter, r *http.Request) {
	s.reply("ListAccounts")(w, r)
}
func (s stubHandlers) ReportBalance(w http.ResponseWriter, r *http.Request) {
	s.reply("ReportBalance")(w, r)
}
func (s stubHandlers) ListSnapshots(w http.ResponseWriter, r *http.Request) {
	s.reply("ListSnapshots")(w, r)
}

func (s stubHandlers) CreateCard(w http.ResponseWriter, r *http.Request) {
	s.reply("CreateCard")(w, r)
}
func (s stubHandlers) ListCards(w http.ResponseWriter, r *http.Request) {
	s.reply("ListCards")(w, r)
}
func (s stubHandlers) GetCard(w http.ResponseWriter, r *http.Request) {
	s.reply("GetCard")(w, r)
}
func (s stubHandlers) UpdateCard(w http.ResponseWriter, r *http.Request) {
	s.reply("UpdateCard")(w, r)
}
func (s stubHandlers) DeleteCard(w http.ResponseWriter, r *http.Request) {
	s.reply("DeleteCard")(w, r)
}

func (s stubHandlers) CreatePaymentMethod(w http.ResponseWriter, r *http.Request) {
	s.reply("CreatePaymentMethod")(w, r)
}
func (s stubHandlers) UpdatePaymentMethod(w http.ResponseWriter, r *http.Request) {
	s.reply("UpdatePaymentMethod")(w, r)
}
func (s stubHandlers) DeletePaymentMethod(w http.ResponseWriter, r *http.Request) {
	s.reply("DeletePaymentMethod")(w, r)
}

func (s stubHandlers) CreatePlan(w http.ResponseWriter, r *http.Request) {
	s.reply("CreatePlan")(w, r)
}
func (s stubHandlers) ListPlans(w http.ResponseWriter, r *http.Request) {
	s.reply("ListPlans")(w, r)
}
func (s stubHandlers) GetPlan(w http.ResponseWriter, r *http.Request) {
	s.reply("GetPlan")(w, r)
}
func (s stubHandlers) UpdatePlan(w http.ResponseWriter, r *http.Request) {
	s.reply("UpdatePlan")(w, r)
}

func (s stubHandlers) ListCurrencies(w http.ResponseWriter, r *http.Request) {
	s.reply("ListCurrencies")(w, r)
}
func (s stubHandlers) AddCurrency(w http.ResponseWriter, r *http.Request) {
	s.reply("AddCurrency")(w, r)
}

func (s stubHandlers) CreateTransfer(w http.ResponseWriter, r *http.Request) {
	s.reply("CreateTransfer")(w, r)
}
func (s stubHandlers) CancelTransfer(w http.ResponseWriter, r *http.Request) {
	s.reply("CancelTransfer")(w, r)
}

func (s stubHandlers) SetExchangeRate(w http.ResponseWriter, r *http.Request) {
	s.reply("SetExchangeRate")(w, r)
}
func (s stubHandlers) ListExchangeRates(w http.ResponseWriter, r *http.Request) {
	s.reply("ListExchangeRates")(w, r)
}
func (s stubHandlers) DeleteExchangeRate(w http.ResponseWriter, r *http.Request) {
	s.reply("DeleteExchangeRate")(w, r)
}

func (s stubHandlers) CreateRecurringRule(w http.ResponseWriter, r *http.Request) {
	s.reply("CreateRecurringRule")(w, r)
}
func (s stubHandlers) ListRecurringRules(w http.ResponseWriter, r *http.Request) {
	s.reply("ListRecurringRules")(w, r)
}
func (s stubHandlers) UpdateRecurringRule(w http.ResponseWriter, r *http.Request) {
	s.reply("UpdateRecurringRule")(w, r)
}

func (s stubHandlers) GetLocalArchiveSetting(w http.ResponseWriter, r *http.Request) {
	s.reply("GetLocalArchiveSetting")(w, r)
}
func (s stubHandlers) SetLocalArchiveSetting(w http.ResponseWriter, r *http.Request) {
	s.reply("SetLocalArchiveSetting")(w, r)
}
func (s stubHandlers) ExportArchive(w http.ResponseWriter, r *http.Request) {
	s.reply("ExportArchive")(w, r)
}
func (s stubHandlers) ImportArchive(w http.ResponseWriter, r *http.Request) {
	s.reply("ImportArchive")(w, r)
}

func (s stubHandlers) GetImportSpec(w http.ResponseWriter, r *http.Request) {
	s.reply("GetImportSpec")(w, r)
}
func (s stubHandlers) ImportMovements(w http.ResponseWriter, r *http.Request) {
	s.reply("ImportMovements")(w, r)
}

func (s stubHandlers) ExportMovements(w http.ResponseWriter, r *http.Request) {
	s.reply("ExportMovements")(w, r)
}

func (s stubHandlers) GetSettings(w http.ResponseWriter, r *http.Request) {
	s.reply("GetSettings")(w, r)
}
func (s stubHandlers) PatchSettings(w http.ResponseWriter, r *http.Request) {
	s.reply("PatchSettings")(w, r)
}

func (s stubHandlers) Me(w http.ResponseWriter, r *http.Request) { s.reply("Me")(w, r) }

func (s stubHandlers) GetConfig(w http.ResponseWriter, r *http.Request) { s.reply("GetConfig")(w, r) }

// Webhook and GetPlan (below) satisfy handlers.BillingHandler.
// GetPlan's method already exists above for handlers.PlanHandler — the
// same implementation structurally satisfies both interfaces, since Go
// interface satisfaction only cares about the method signature.
func (s stubHandlers) Webhook(w http.ResponseWriter, r *http.Request) { s.reply("Webhook")(w, r) }

func noopAuthMiddleware(next http.Handler) http.Handler { return next }

func TestStandaloneRejectsSyncRoute(t *testing.T) {
	s := stubHandlers{}
	router := NewRouter(
		handlers.MovementHandler(s), handlers.AccountHandler(s), handlers.CardHandler(s), handlers.CurrencyHandler(s),
		handlers.TransferHandler(s), handlers.ExchangeRateHandler(s),
		handlers.RecurringRuleHandler(s), handlers.ArchiveHandler(s), handlers.ImportHandler(s),
		handlers.ExportHandler(s), handlers.PaymentMethodHandler(s), handlers.PlanHandler(s), handlers.SettingsHandler(s), handlers.UserHandler(s), handlers.ConfigHandler(s), handlers.BillingHandler(s),
		AuthMiddleware(noopAuthMiddleware), "*", true, nil,
	)

	req := httptest.NewRequest(http.MethodPost, "/sync", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("POST /sync in standalone mode = %d, want 404 (no ledger-service to sync to)", rec.Code)
	}
}

func TestNonStandaloneServesSyncRoute(t *testing.T) {
	s := stubHandlers{}
	router := NewRouter(
		handlers.MovementHandler(s), handlers.AccountHandler(s), handlers.CardHandler(s), handlers.CurrencyHandler(s),
		handlers.TransferHandler(s), handlers.ExchangeRateHandler(s),
		handlers.RecurringRuleHandler(s), handlers.ArchiveHandler(s), handlers.ImportHandler(s),
		handlers.ExportHandler(s), handlers.PaymentMethodHandler(s), handlers.PlanHandler(s), handlers.SettingsHandler(s), handlers.UserHandler(s), handlers.ConfigHandler(s), handlers.BillingHandler(s),
		AuthMiddleware(noopAuthMiddleware), "*", false, nil,
	)

	req := httptest.NewRequest(http.MethodPost, "/sync", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || rec.Body.String() != "Sync" {
		t.Errorf("POST /sync outside standalone = %d %q, want 200 Sync", rec.Code, rec.Body.String())
	}
}

func TestExportRouteAvailableInEveryMode(t *testing.T) {
	for _, standalone := range []bool{false, true} {
		s := stubHandlers{}
		router := NewRouter(
			handlers.MovementHandler(s), handlers.AccountHandler(s), handlers.CardHandler(s), handlers.CurrencyHandler(s),
			handlers.TransferHandler(s), handlers.ExchangeRateHandler(s),
			handlers.RecurringRuleHandler(s), handlers.ArchiveHandler(s), handlers.ImportHandler(s),
			handlers.ExportHandler(s), handlers.PaymentMethodHandler(s), handlers.PlanHandler(s), handlers.SettingsHandler(s), handlers.UserHandler(s), handlers.ConfigHandler(s), handlers.BillingHandler(s),
			AuthMiddleware(noopAuthMiddleware), "*", standalone, nil,
		)

		req := httptest.NewRequest(http.MethodGet, "/export/movements", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK || rec.Body.String() != "ExportMovements" {
			t.Errorf("GET /export/movements (standalone=%v) = %d %q, want 200 ExportMovements", standalone, rec.Code, rec.Body.String())
		}
	}
}

func TestStandaloneServesEmbeddedFrontendAsFallback(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":              &fstest.MapFile{Data: []byte("<html>app shell</html>")},
		"_app/immutable/chunk.js": &fstest.MapFile{Data: []byte("console.log(1)")},
	}
	s := stubHandlers{}
	router := NewRouter(
		handlers.MovementHandler(s), handlers.AccountHandler(s), handlers.CardHandler(s), handlers.CurrencyHandler(s),
		handlers.TransferHandler(s), handlers.ExchangeRateHandler(s),
		handlers.RecurringRuleHandler(s), handlers.ArchiveHandler(s), handlers.ImportHandler(s),
		handlers.ExportHandler(s), handlers.PaymentMethodHandler(s), handlers.PlanHandler(s), handlers.SettingsHandler(s), handlers.UserHandler(s), handlers.ConfigHandler(s), handlers.BillingHandler(s),
		AuthMiddleware(noopAuthMiddleware), "*", true, fsys,
	)

	// A real asset is served as-is.
	req := httptest.NewRequest(http.MethodGet, "/_app/immutable/chunk.js", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "console.log(1)" {
		t.Errorf("static asset = %d %q, want the file's real content", rec.Code, rec.Body.String())
	}

	// A client-side route (not a real file) falls back to index.html so
	// SvelteKit's own router can resolve it.
	req = httptest.NewRequest(http.MethodGet, "/import", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "<html>app shell</html>" {
		t.Errorf("deep link fallback = %d %q, want index.html's content", rec.Code, rec.Body.String())
	}

	// API routes still take priority over the SPA fallback.
	req = httptest.NewRequest(http.MethodGet, "/export/movements", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Body.String() != "ExportMovements" {
		t.Errorf("API route body = %q, want ExportMovements (must not be shadowed by the SPA fallback)", rec.Body.String())
	}
}
