// Package services holds the application layer's service contracts —
// what the application needs from the outside world and what it exposes
// to the interfaces layer. Following "application defines, infrastructure
// adapts": interfaces live here, implementations live in infrastructure
// (LedgerGateway) or application/sync (SyncTrigger, SyncRunner).
package services

import (
	"context"

	"github.com/JorgeSaicoski/financial-tracker/application/dto"
)

// LedgerGateway is the port the sync service needs from the outside
// world: publish one movement, get back the id ledger-service assigned.
// Implemented by infrastructure/ledgerservice — application code never
// imports infrastructure. Expressed in application/dto, like every
// application contract: the adapter narrows the DTO to ledger-service's
// wire format on its side of the boundary.
type LedgerGateway interface {
	Publish(ctx context.Context, movement *dto.MovementDTO) (ledgerTransactionID string, err error)
}

// Summary is the result of one sync pass.
type Summary struct {
	Synced int
	Failed int
}

// SyncTrigger lets cancel usecases kick a best-effort background sync so a
// freshly created reversal reaches ledger-service promptly, without the
// cancel ever blocking on ledger-service being up. Implemented by
// application/sync.Service.
type SyncTrigger interface {
	TriggerAsync()
}

// SyncRunner is what the HTTP handler needs from application/sync: run one
// synchronous pass (POST /sync ignores the retry cooldown — the user
// explicitly asked). Implemented by application/sync.Service.
type SyncRunner interface {
	RunPassNow(ctx context.Context) Summary
}
