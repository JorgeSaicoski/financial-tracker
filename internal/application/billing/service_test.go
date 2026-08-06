package billing

import (
	"context"
	"testing"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/logger"
)

// fakeSubscriptionRepo is an in-memory SubscriptionRepository — only
// ListLapsable matters to this package's tests.
type fakeSubscriptionRepo struct {
	byUserID map[string]*dto.SubscriptionDTO
}

func newFakeSubscriptionRepo() *fakeSubscriptionRepo {
	return &fakeSubscriptionRepo{byUserID: map[string]*dto.SubscriptionDTO{}}
}

func (f *fakeSubscriptionRepo) Upsert(_ context.Context, sub *dto.SubscriptionDTO) (*dto.SubscriptionDTO, error) {
	cp := *sub
	f.byUserID[sub.UserID] = &cp
	return &cp, nil
}

func (f *fakeSubscriptionRepo) GetByUserID(_ context.Context, userID string) (*dto.SubscriptionDTO, error) {
	s, ok := f.byUserID[userID]
	if !ok {
		return nil, apperrors.ErrNotFound
	}
	cp := *s
	return &cp, nil
}

func (f *fakeSubscriptionRepo) ListLapsable(_ context.Context, asOf time.Time, graceDays int) ([]*dto.SubscriptionDTO, error) {
	var out []*dto.SubscriptionDTO
	for _, s := range f.byUserID {
		if s.Status != dto.SubscriptionStatusPastDue && s.Status != dto.SubscriptionStatusCanceled {
			continue
		}
		if !s.CurrentPeriodEnd.AddDate(0, 0, graceDays).After(asOf) {
			cp := *s
			out = append(out, &cp)
		}
	}
	return out, nil
}

// fakeSettingsRepo is an in-memory UserSettingsRepository — only
// SetCloudStorageEntitled and Get matter to this package's tests.
type fakeSettingsRepo struct {
	byUserID map[string]*dto.UserSettingsDTO
}

func newFakeSettingsRepo() *fakeSettingsRepo {
	return &fakeSettingsRepo{byUserID: map[string]*dto.UserSettingsDTO{}}
}

func (f *fakeSettingsRepo) Get(_ context.Context, userID string) (*dto.UserSettingsDTO, error) {
	if s, ok := f.byUserID[userID]; ok {
		cp := *s
		return &cp, nil
	}
	return dto.DefaultUserSettings(userID, time.Now().UTC()), nil
}

func (f *fakeSettingsRepo) UpdateEnabled(_ context.Context, userID string, enabled bool) (*dto.UserSettingsDTO, error) {
	s, ok := f.byUserID[userID]
	if !ok {
		now := time.Now().UTC()
		s = dto.DefaultUserSettings(userID, now)
		f.byUserID[userID] = s
	}
	s.LedgerSyncEnabled = enabled
	return s, nil
}

func (f *fakeSettingsRepo) ListSyncDisabledUserIDs(_ context.Context) ([]string, error) {
	return nil, nil
}

func (f *fakeSettingsRepo) SetCloudStorageEntitled(_ context.Context, userID string, entitled bool) (*dto.UserSettingsDTO, error) {
	s, ok := f.byUserID[userID]
	if !ok {
		now := time.Now().UTC()
		s = dto.DefaultUserSettings(userID, now)
		f.byUserID[userID] = s
	}
	s.CloudStorageEntitled = entitled
	return s, nil
}

func (f *fakeSettingsRepo) SetDefaultCategory(_ context.Context, userID string, categoryID *string) (*dto.UserSettingsDTO, error) {
	s, ok := f.byUserID[userID]
	if !ok {
		now := time.Now().UTC()
		s = dto.DefaultUserSettings(userID, now)
		f.byUserID[userID] = s
	}
	s.DefaultCategoryID = categoryID
	return s, nil
}

// TestRunPassLapsesEntitlementPastGracePeriod is BACK-19's grace-period
// acceptance criterion at the sweep level: cancelling flips entitlement
// back after the grace period, not immediately.
func TestRunPassLapsesEntitlementPastGracePeriod(t *testing.T) {
	subs := newFakeSubscriptionRepo()
	settings := newFakeSettingsRepo()
	now := time.Date(2027, 3, 15, 0, 0, 0, 0, time.UTC)
	svc := NewService(subs, settings, 7, logger.New())
	svc.(*service).now = func() time.Time { return now }

	if _, err := settings.SetCloudStorageEntitled(context.Background(), "past-grace", true); err != nil {
		t.Fatal(err)
	}
	subs.byUserID["past-grace"] = &dto.SubscriptionDTO{
		UserID: "past-grace", Provider: "stripe", ProviderSubscriptionID: "sub_1",
		Status: dto.SubscriptionStatusPastDue, CurrentPeriodEnd: now.AddDate(0, 0, -10),
	}

	sum := svc.RunPass(context.Background())
	if sum.EntitlementsLapsed != 1 || sum.Errors != 0 {
		t.Fatalf("Summary = %+v, want 1 lapsed, 0 errors", sum)
	}

	s, err := settings.Get(context.Background(), "past-grace")
	if err != nil {
		t.Fatal(err)
	}
	if s.CloudStorageEntitled {
		t.Error("cloud_storage_entitled should be false once the grace period has elapsed")
	}
}

func TestRunPassLeavesEntitlementInsideGracePeriod(t *testing.T) {
	subs := newFakeSubscriptionRepo()
	settings := newFakeSettingsRepo()
	now := time.Date(2027, 3, 15, 0, 0, 0, 0, time.UTC)
	svc := NewService(subs, settings, 7, logger.New())
	svc.(*service).now = func() time.Time { return now }

	if _, err := settings.SetCloudStorageEntitled(context.Background(), "still-in-grace", true); err != nil {
		t.Fatal(err)
	}
	subs.byUserID["still-in-grace"] = &dto.SubscriptionDTO{
		UserID: "still-in-grace", Provider: "stripe", ProviderSubscriptionID: "sub_2",
		Status: dto.SubscriptionStatusPastDue, CurrentPeriodEnd: now.AddDate(0, 0, -2),
	}

	sum := svc.RunPass(context.Background())
	if sum.EntitlementsLapsed != 0 {
		t.Fatalf("Summary = %+v, want nothing lapsed while still inside the grace period", sum)
	}

	s, err := settings.Get(context.Background(), "still-in-grace")
	if err != nil {
		t.Fatal(err)
	}
	if !s.CloudStorageEntitled {
		t.Error("cloud_storage_entitled must stay true while inside the grace period")
	}
}

// TestRunPassLapsesCanceledSubscriptionsPastGracePeriodToo guards
// BACK-19's acceptance criterion in its own words: an explicit
// cancellation "flips it back after the grace period, not immediately"
// — same leniency as a late payment, not instant cutoff.
func TestRunPassLapsesCanceledSubscriptionsPastGracePeriodToo(t *testing.T) {
	subs := newFakeSubscriptionRepo()
	settings := newFakeSettingsRepo()
	now := time.Date(2027, 3, 15, 0, 0, 0, 0, time.UTC)
	svc := NewService(subs, settings, 7, logger.New())
	svc.(*service).now = func() time.Time { return now }

	if _, err := settings.SetCloudStorageEntitled(context.Background(), "canceled-past-grace", true); err != nil {
		t.Fatal(err)
	}
	subs.byUserID["canceled-past-grace"] = &dto.SubscriptionDTO{
		UserID: "canceled-past-grace", Provider: "stripe", ProviderSubscriptionID: "sub_3",
		Status: dto.SubscriptionStatusCanceled, CurrentPeriodEnd: now.AddDate(0, 0, -10),
	}

	sum := svc.RunPass(context.Background())
	if sum.EntitlementsLapsed != 1 {
		t.Fatalf("Summary = %+v, want 1 lapsed", sum)
	}

	s, err := settings.Get(context.Background(), "canceled-past-grace")
	if err != nil {
		t.Fatal(err)
	}
	if s.CloudStorageEntitled {
		t.Error("cloud_storage_entitled should be false once a cancellation's grace period has elapsed")
	}
}
