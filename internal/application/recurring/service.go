package recurring

import (
	"context"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/logger"
)

type service struct {
	rules repositories.RecurringRuleRepository
	log   logger.Logger
	// now is overridable in tests; defaults to time.Now in NewService.
	now func() time.Time
}

// NewService returns interface type for dependency injection.
func NewService(rules repositories.RecurringRuleRepository, log logger.Logger) Service {
	return &service{rules: rules, log: log, now: time.Now}
}

func (s *service) RunPass(ctx context.Context) Summary {
	var sum Summary

	active, err := s.rules.ListActive(ctx)
	if err != nil {
		s.log.Error("recurring: listing active rules failed: %v", err)
		return sum
	}

	now := s.now().UTC()
	for _, rule := range active {
		sum.RulesProcessed++

		occurrences := dueOccurrences(rule, now)
		if len(occurrences) == 0 {
			continue
		}

		movements := make([]*dto.MovementDTO, len(occurrences))
		for i, occ := range occurrences {
			movements[i] = dto.MovementFromEntity(&entities.Movement{
				UserID:          rule.UserID,
				Amount:          rule.Amount,
				Currency:        rule.Currency,
				Description:     rule.Description,
				CategoryID:      rule.CategoryID,
				PaymentMethod:   rule.PaymentMethod,
				AccountID:       rule.AccountID,
				RecurringRuleID: &rule.ID,
				Status:          entities.MovementStatusActive,
				SyncStatus:      entities.SyncStatusPending,
				Timestamp:       occ,
				CreatedAt:       now,
			})
		}

		newWatermark := occurrences[len(occurrences)-1]
		if _, err := s.rules.GenerateAndAdvance(ctx, rule.ID, movements, newWatermark); err != nil {
			sum.Errors++
			s.log.Error("recurring: generating for rule %s failed: %v", rule.ID, err)
			continue
		}
		sum.MovementsGenerated += len(movements)
	}

	if sum.MovementsGenerated > 0 || sum.Errors > 0 {
		s.log.Info("recurring: pass done, rules=%d generated=%d errors=%d", sum.RulesProcessed, sum.MovementsGenerated, sum.Errors)
	}
	return sum
}

// Start runs a generation pass every interval until ctx is cancelled.
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
