// Package ledgerservice is the only place in financial-tracker that knows
// ledger-service's wire format (POST/GET /transactions). Everything else
// talks to the domain/repositories.MovementRepository interface.
package ledgerservice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	wire "github.com/JorgeSaicoski/financial-tracker/infrastructure/ledgerservice/entities"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// apiError carries the HTTP status code back so the repository layer can
// map it onto financial-tracker's own error kinds.
type apiError struct {
	StatusCode int
	Message    string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("ledger-service: %d %s", e.StatusCode, e.Message)
}

// CreateTransaction posts a new transaction. ledger-service's POST
// /transactions only returns the created id (a bare JSON string), not the
// full record, so this fetches the created transaction before returning.
func (c *Client) CreateTransaction(ctx context.Context, req wire.TransactionRequest) (wire.Transaction, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return wire.Transaction{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/transactions", bytes.NewReader(body))
	if err != nil {
		return wire.Transaction{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	var id string
	if err := c.do(httpReq, &id); err != nil {
		return wire.Transaction{}, err
	}

	return c.GetTransaction(ctx, id)
}

func (c *Client) GetTransaction(ctx context.Context, id string) (wire.Transaction, error) {
	u := c.baseURL + "/transactions?" + url.Values{"id": {id}}.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return wire.Transaction{}, err
	}

	var tx wire.Transaction
	if err := c.do(httpReq, &tx); err != nil {
		return wire.Transaction{}, err
	}
	return tx, nil
}

func (c *Client) ListTransactions(ctx context.Context, userID string, currency *string, limit, offset int) ([]wire.Transaction, error) {
	q := url.Values{"user_id": {userID}}
	if currency != nil {
		q.Set("currency", *currency)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}

	u := c.baseURL + "/transactions?" + q.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	var list wire.TransactionListResponse
	if err := c.do(httpReq, &list); err != nil {
		return nil, err
	}
	return list.Transactions, nil
}

func (c *Client) do(req *http.Request, out interface{}) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode >= http.StatusBadRequest {
		var errResp wire.ErrorResponse
		msg := string(respBody)
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != "" {
			msg = errResp.Error
		}
		return &apiError{StatusCode: resp.StatusCode, Message: msg}
	}

	if len(respBody) == 0 {
		return nil
	}
	return json.Unmarshal(respBody, out)
}
