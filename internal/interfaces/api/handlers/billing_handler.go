package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/services"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/usecases"
	"github.com/JorgeSaicoski/financial-tracker/internal/interfaces/api/reqctx"
	interfacedto "github.com/JorgeSaicoski/financial-tracker/internal/interfaces/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/logger"
)

// billingSignatureHeader carries the payment provider's signature over
// the raw request body. Generic name, not e.g. "Stripe-Signature" — the
// provider is a build-time choice (see services.PaymentWebhookVerifier's
// doc comment), and a real integration's own adapter is what would
// translate a provider-specific header into this one.
const billingSignatureHeader = "X-Billing-Signature"

type billingHandler struct {
	processWebhook usecases.ProcessBillingWebhookUseCase
	getPlan        usecases.GetBillingPlanUseCase
	verifier       services.PaymentWebhookVerifier
	log            logger.Logger
}

// NewBillingHandler returns interface type for dependency injection.
func NewBillingHandler(
	processWebhook usecases.ProcessBillingWebhookUseCase,
	getPlan usecases.GetBillingPlanUseCase,
	verifier services.PaymentWebhookVerifier,
	log logger.Logger,
) BillingHandler {
	return &billingHandler{processWebhook: processWebhook, getPlan: getPlan, verifier: verifier, log: log}
}

// Webhook handles POST /billing/webhook. Deliberately mounted outside
// the user-token auth middleware (see router.go) — the payment provider
// has no financial-tracker session, so authenticity comes from
// billingSignatureHeader instead of a bearer token. The signature is
// checked over the exact raw bytes before any JSON decoding happens, so
// a tampered body never even reaches the decoder.
func (h *billingHandler) Webhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	signature := r.Header.Get(billingSignatureHeader)
	if signature == "" {
		h.writeError(w, http.StatusUnauthorized, "missing "+billingSignatureHeader)
		return
	}
	if err := h.verifier.Verify(body, signature); err != nil {
		h.writeError(w, http.StatusUnauthorized, "invalid webhook signature")
		return
	}

	var req interfacedto.BillingWebhookRequest
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	sub, err := h.processWebhook.Execute(r.Context(), usecases.ProcessBillingWebhookInput{
		UserID:                 req.UserID,
		Provider:               req.Provider,
		ProviderSubscriptionID: req.ProviderSubscriptionID,
		Status:                 req.Status,
		CurrentPeriodEnd:       req.CurrentPeriodEnd,
	})
	if err != nil {
		h.writeUsecaseError(w, "process billing webhook", err)
		return
	}

	h.writeJSON(w, http.StatusOK, interfacedto.BillingWebhookResponse{
		UserID:           sub.UserID,
		Status:           sub.Status,
		CurrentPeriodEnd: sub.CurrentPeriodEnd,
	})
}

// GetPlan handles GET /billing/plan?currency=xxx — protected, since the
// converted price depends on the caller's own BACK-11 exchange rates.
func (h *billingHandler) GetPlan(w http.ResponseWriter, r *http.Request) {
	userID, ok := reqctx.UserID(r.Context())
	if !ok {
		h.writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	view, err := h.getPlan.Execute(r.Context(), userID, r.URL.Query().Get("currency"))
	if err != nil {
		h.writeUsecaseError(w, "get billing plan", err)
		return
	}
	h.writeJSON(w, http.StatusOK, interfacedto.BillingPlanResponse{Currency: view.Currency, Amount: view.Amount})
}

func (h *billingHandler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	writeJSON(h.log, w, status, data)
}

func (h *billingHandler) writeError(w http.ResponseWriter, status int, message string) {
	writeError(h.log, w, status, message)
}

func (h *billingHandler) writeUsecaseError(w http.ResponseWriter, action string, err error) {
	writeUsecaseError(h.log, w, action, err)
}
