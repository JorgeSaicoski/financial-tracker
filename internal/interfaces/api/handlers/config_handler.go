package handlers

import (
	"net/http"

	interfacedto "github.com/JorgeSaicoski/financial-tracker/internal/interfaces/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/logger"
)

type configHandler struct {
	standalone  bool
	authEnabled bool
	log         logger.Logger
}

// NewConfigHandler returns interface type for dependency injection.
//
// This is intentionally a minimal seed, not the final shape of either
// flag: `standalone` is hardcoded false today because BACK-09 (single
// binary, local mode) doesn't exist yet, and `authEnabled` is driven by
// a standalone AUTH_ENABLED env var rather than BACK-02's AUTH_DISABLED
// (PR #16, not yet merged into main at the time this was written) so
// this endpoint doesn't depend on unmerged work. Both are cheap for a
// human to fold into BACK-02/BACK-09's real config once those land — see
// FRONT-04's PR description for the reasoning.
func NewConfigHandler(standalone, authEnabled bool, log logger.Logger) ConfigHandler {
	return &configHandler{standalone: standalone, authEnabled: authEnabled, log: log}
}

// GetConfig handles GET /config. Deliberately unauthenticated — the
// frontend calls this before it knows whether it has a token, so it must
// stay reachable without one even after BACK-02's JWT middleware lands;
// whoever wires that middleware in should exempt this route explicitly.
func (h *configHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(h.log, w, http.StatusOK, interfacedto.ConfigResponse{
		Standalone:  h.standalone,
		AuthEnabled: h.authEnabled,
	})
}
