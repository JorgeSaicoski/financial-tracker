package billing

import (
	"context"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/logger"
)

type service struct {
	subscriptions repositories.SubscriptionRepository
	settings      repositories.UserSettingsRepository
	log           logger.Logger
	graceDays     int
	// now is overridable in tests; defaults to time.Now in NewService.
	now func() time.Time
}

// NewService returns interface type for dependency injection. graceDays
// is how long a past_due subscription keeps its entitlement after
// current_period_end before the sweep lapses it (BACK-19: "a late card
// shouldn't cut off access instantly").
func NewService(subscriptions repositories.SubscriptionRepository, settings repositories.UserSettingsRepository, graceDays int, log logger.Logger) Service {
	return &service{subscriptions: subscriptions, settings: settings, graceDays: graceDays, log: log, now: time.Now}
}

func (s *service) RunPass(ctx context.Context) Summary {
	var sum Summary

	lapsable, err := s.subscriptions.ListLapsable(ctx, s.now().UTC(), s.graceDays)
	if err != nil {
		s.log.Error("billing: listing lapsable subscriptions failed: %v", err)
		return sum
	}

	for _, sub := range lapsable {
		sum.SubscriptionsChecked++

		if _, err := s.settings.SetCloudStorageEntitled(ctx, sub.UserID, false); err != nil {
			sum.Errors++
			s.log.Error("billing: lapsing entitlement for user %s failed: %v", sub.UserID, err)
			continue
		}
		sum.EntitlementsLapsed++
	}

	if sum.EntitlementsLapsed > 0 || sum.Errors > 0 {
		s.log.Info("billing: sweep done, checked=%d lapsed=%d errors=%d", sum.SubscriptionsChecked, sum.EntitlementsLapsed, sum.Errors)
	}
	return sum
}

// Start runs a sweep pass every interval until ctx is cancelled.
func (s *service) Start(ctx context.Context, interval time.Duration) {
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
