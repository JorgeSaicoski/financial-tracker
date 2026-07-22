package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	interfacedto "github.com/JorgeSaicoski/financial-tracker/interfaces/dto"
	"github.com/JorgeSaicoski/financial-tracker/pkg/logger"
)

// Shared response plumbing for all handlers in this package.

func writeJSON(log logger.Logger, w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Error("failed to encode JSON response: %v", err)
	}
}

func writeError(log logger.Logger, w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(interfacedto.ErrorResponse{Error: message}); err != nil {
		log.Error("failed to encode error response: %v", err)
	}
}

var errBadTimeParam = errors.New("invalid time parameter")

// parseTimeParam accepts RFC 3339 or a bare date. A bare date means the
// whole day: as a lower bound it's that day's midnight UTC; as an upper
// bound (endOfDay) it's the next midnight, pairing with the repository's
// exclusive "timestamp < to".
func parseTimeParam(r *http.Request, name string, endOfDay bool) (*time.Time, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		t = t.UTC()
		return &t, nil
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return nil, errBadTimeParam
	}
	if endOfDay {
		t = t.Add(24 * time.Hour)
	}
	return &t, nil
}
