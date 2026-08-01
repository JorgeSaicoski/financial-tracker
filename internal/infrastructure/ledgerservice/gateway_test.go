package ledgerservice

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	wire "github.com/JorgeSaicoski/financial-tracker/internal/infrastructure/ledgerservice/entities"
)

// fakePseudonymizer is a minimal services.LedgerPseudonymizer for testing
// gateway.Publish's narrowing point in isolation, without a real
// database.
type fakePseudonymizer struct {
	pseudonyms map[string]string
	tokens     map[string]string
}

func (f *fakePseudonymizer) PseudonymFor(_ context.Context, userID string) (string, error) {
	return f.pseudonyms[userID], nil
}

func (f *fakePseudonymizer) TokenizeCurrency(_ context.Context, userID, currencyCode string) (string, error) {
	return f.tokens[userID+":"+currencyCode], nil
}

// TestPublishNeverSendsRealUserIDOrCurrency is BACK-16's acceptance
// criterion at the gateway.Publish call boundary: ledger-service must
// receive the pseudonym and the currency token, never the real values.
func TestPublishNeverSendsRealUserIDOrCurrency(t *testing.T) {
	const (
		realUserID = "11111111-1111-1111-1111-111111111111"
		pseudonym  = "22222222-2222-2222-2222-222222222222"
		token      = "c_deadbeefcafef00d"
	)

	var received wire.TransactionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				t.Fatal(err)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode("tx-1")
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(wire.Transaction{
				ID: "tx-1", UserID: received.UserID, Amount: received.Amount,
				Currency: received.Currency, Timestamp: time.Now(),
			})
		}
	}))
	defer server.Close()

	client := NewClient(server.URL)
	pseudonymizer := &fakePseudonymizer{
		pseudonyms: map[string]string{realUserID: pseudonym},
		tokens:     map[string]string{realUserID + ":usd": token},
	}
	gateway := NewLedgerGateway(client, pseudonymizer)

	movement := &dto.MovementDTO{UserID: realUserID, Amount: -500, Currency: "usd"}
	txID, err := gateway.Publish(context.Background(), movement)
	if err != nil {
		t.Fatal(err)
	}
	if txID != "tx-1" {
		t.Errorf("txID = %q, want tx-1", txID)
	}

	if received.UserID == realUserID {
		t.Error("ledger-service received the real user id")
	}
	if received.UserID != pseudonym {
		t.Errorf("received.UserID = %q, want pseudonym %q", received.UserID, pseudonym)
	}
	if received.Currency == "usd" {
		t.Error("ledger-service received the plain currency code")
	}
	if received.Currency != token {
		t.Errorf("received.Currency = %q, want token %q", received.Currency, token)
	}
	if received.Amount != -500 {
		t.Error("amount must never be hidden or tokenized")
	}
}
