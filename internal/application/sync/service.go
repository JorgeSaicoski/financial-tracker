// Package sync pushes locally-stored movements to ledger-service in the
// background. Writes to financial-tracker never block on ledger-service
// being reachable — this service is the only place that catches it up.
package sync

import (
	"context"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/services"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/logger"
)

type Service struct {
	repo          repositories.MovementRepository
	gateway       services.LedgerGateway
	log           logger.Logger
	retryCooldown time.Duration
	// asyncTimeout bounds the detached pass triggered after a cancel.
	asyncTimeout time.Duration
}

func NewService(repo repositories.MovementRepository, gateway services.LedgerGateway, log logger.Logger, retryCooldown time.Duration) *Service {
	return &Service{
		repo:          repo,
		gateway:       gateway,
		log:           log,
		retryCooldown: retryCooldown,
		asyncTimeout:  10 * time.Second,
	}
}

// RunPass syncs due movements, skipping rows attempted within the retry
// cooldown — this is what the background ticker calls.
func (s *Service) RunPass(ctx context.Context) services.Summary {
	return s.run(ctx, s.retryCooldown)
}

// RunPassNow ignores the cooldown — the user explicitly asked to sync.
func (s *Service) RunPassNow(ctx context.Context) services.Summary {
	return s.run(ctx, 0)
}

func (s *Service) run(ctx context.Context, cooldown time.Duration) services.Summary {
	var sum services.Summary

	pending, err := s.repo.ListPendingSync(ctx, time.Now().UTC(), cooldown)
	if err != nil {
		s.log.Error("sync: listing pending movements failed: %v", err)
		return sum
	}

	for _, m := range pending {
		ledgerID, err := s.gateway.Publish(ctx, m)
		now := time.Now().UTC()
		if err != nil {
			sum.Failed++
			s.log.Error("sync: movement %s failed: %v", m.ID, err)
			if markErr := s.repo.MarkSyncFailed(ctx, m.ID, err.Error(), now); markErr != nil {
				s.log.Error("sync: recording failure for %s failed: %v", m.ID, markErr)
			}
			continue
		}
		sum.Synced++
		if markErr := s.repo.MarkSynced(ctx, m.ID, ledgerID, now); markErr != nil {
			// The transaction reached ledger-service but we could not
			// record that — the retry will duplicate it (see the
			// idempotency limitation in PLAN.md). Loud log, nothing else
			// we can do from here.
			s.log.Error("sync: movement %s synced as %s but recording failed: %v", m.ID, ledgerID, markErr)
		}
	}

	if sum.Synced > 0 || sum.Failed > 0 {
		s.log.Info("sync: pass done, synced=%d failed=%d", sum.Synced, sum.Failed)
	}
	return sum
}

// TriggerAsync runs one best-effort pass in the background, detached from
// the caller's request. Used after cancels so a reversal reaches
// ledger-service promptly without the cancel blocking on it.
func (s *Service) TriggerAsync() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), s.asyncTimeout)
		defer cancel()
		s.RunPass(ctx)
	}()
}

// Start runs a sync pass every interval until ctx is cancelled.
func (s *Service) Start(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.RunPass(ctx)
			}
		}
	}()
}
