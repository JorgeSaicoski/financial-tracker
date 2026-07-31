package usecases

import (
	"context"
	"fmt"
	"strings"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

// defaultPaymentMethods are seeded once per user for first-run
// convenience — chosen because they describe a payment *mechanism*, not
// a specific country's product. Notably absent: "pix" (Brazil's instant-
// payment rail) — a user who wants it, or Uruguay's bank-clearing
// transfers, or anything else, adds it themselves as a free-form name,
// exactly how a user adds a new code to the currency registry today.
// "credit_card" and "bank_transfer" are seeded separately, as system
// entries (see ensureSystemPaymentMethods) — listed here too since a
// fresh user's registry should contain both groups, but never
// duplicated: EnsureByName is idempotent.
var defaultPaymentMethods = []string{"cash", "debit_card", "other"}

// isSystemPaymentMethod reports whether name (case-insensitive) is one
// of the two reserved payment-method names code branches on directly
// (movement_handler.go's installment check, Account.Send/Receive) — they
// can't be created/renamed/deleted through the API.
func isSystemPaymentMethod(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	return lower == entities.PaymentMethodCreditCard || lower == entities.PaymentMethodBankTransfer
}

// resolvePaymentMethod returns the effective payment-method name to
// store on a movement: empty input defaults to "other", otherwise the
// name is resolved against the user's payment-method registry,
// implicitly registering it on first use (same idempotent-Add shape as
// AddCurrencyUseCase) if it doesn't already exist.
func resolvePaymentMethod(ctx context.Context, methods repositories.PaymentMethodRepository, userID, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "other"
	}
	method, err := methods.EnsureByName(ctx, userID, name)
	if err != nil {
		return "", err
	}
	return method.Name, nil
}

// ensureSystemPaymentMethods lazily creates "credit_card" and
// "bank_transfer" for userID if they don't exist yet, plus the ordinary
// first-run defaults — absence-safe, same pattern as user_settings.
// Idempotent: EnsureByName no-ops when a row already exists.
func ensureSystemPaymentMethods(ctx context.Context, methods repositories.PaymentMethodRepository, userID string) error {
	if _, err := methods.EnsureByName(ctx, userID, entities.PaymentMethodCreditCard); err != nil {
		return err
	}
	if _, err := methods.EnsureByName(ctx, userID, entities.PaymentMethodBankTransfer); err != nil {
		return err
	}
	for _, name := range defaultPaymentMethods {
		if _, err := methods.EnsureByName(ctx, userID, name); err != nil {
			return err
		}
	}
	return nil
}

type createPaymentMethodUseCase struct {
	methods repositories.PaymentMethodRepository
}

// NewCreatePaymentMethod returns interface type for dependency injection.
func NewCreatePaymentMethod(methods repositories.PaymentMethodRepository) CreatePaymentMethodUseCase {
	return &createPaymentMethodUseCase{methods: methods}
}

func (uc *createPaymentMethodUseCase) Execute(ctx context.Context, input CreatePaymentMethodInput) (*dto.PaymentMethodDTO, error) {
	name := strings.TrimSpace(input.Name)
	if input.UserID == "" || name == "" {
		return nil, fmt.Errorf("%w: payment method name is required", apperrors.ErrInvalidInput)
	}
	if isSystemPaymentMethod(name) {
		return nil, fmt.Errorf("%w: %q is a reserved payment method name", apperrors.ErrInvalidInput, name)
	}

	existing, err := uc.methods.ListByUser(ctx, input.UserID)
	if err != nil {
		return nil, err
	}
	for _, m := range existing {
		if strings.EqualFold(m.Name, name) {
			return nil, fmt.Errorf("%w: payment method %q already exists", apperrors.ErrConflict, name)
		}
	}

	return uc.methods.Create(ctx, &dto.PaymentMethodDTO{
		UserID: input.UserID,
		Name:   name,
	})
}

type listPaymentMethodsUseCase struct {
	methods repositories.PaymentMethodRepository
}

// NewListPaymentMethods returns interface type for dependency injection.
func NewListPaymentMethods(methods repositories.PaymentMethodRepository) ListPaymentMethodsUseCase {
	return &listPaymentMethodsUseCase{methods: methods}
}

func (uc *listPaymentMethodsUseCase) Execute(ctx context.Context, userID string) ([]*dto.PaymentMethodDTO, error) {
	if err := ensureSystemPaymentMethods(ctx, uc.methods, userID); err != nil {
		return nil, err
	}
	return uc.methods.ListByUser(ctx, userID)
}

type updatePaymentMethodUseCase struct {
	methods repositories.PaymentMethodRepository
}

// NewUpdatePaymentMethod returns interface type for dependency injection.
func NewUpdatePaymentMethod(methods repositories.PaymentMethodRepository) UpdatePaymentMethodUseCase {
	return &updatePaymentMethodUseCase{methods: methods}
}

func (uc *updatePaymentMethodUseCase) Execute(ctx context.Context, userID, id string, input UpdatePaymentMethodInput) (*dto.PaymentMethodDTO, error) {
	if userID == "" || id == "" {
		return nil, apperrors.ErrInvalidInput
	}
	existing, err := uc.methods.GetByID(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if isSystemPaymentMethod(existing.Name) {
		return nil, fmt.Errorf("%w: %q is a reserved payment method and can't be edited", apperrors.ErrInvalidInput, existing.Name)
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: payment method name is required", apperrors.ErrInvalidInput)
	}
	if isSystemPaymentMethod(name) {
		return nil, fmt.Errorf("%w: %q is a reserved payment method name", apperrors.ErrInvalidInput, name)
	}

	if !strings.EqualFold(name, existing.Name) {
		siblings, err := uc.methods.ListByUser(ctx, userID)
		if err != nil {
			return nil, err
		}
		for _, m := range siblings {
			if m.ID != id && strings.EqualFold(m.Name, name) {
				return nil, fmt.Errorf("%w: payment method %q already exists", apperrors.ErrConflict, name)
			}
		}
	}

	if err := uc.methods.Update(ctx, userID, id, name); err != nil {
		return nil, err
	}
	return uc.methods.GetByID(ctx, userID, id)
}

type deletePaymentMethodUseCase struct {
	methods repositories.PaymentMethodRepository
}

// NewDeletePaymentMethod returns interface type for dependency injection.
func NewDeletePaymentMethod(methods repositories.PaymentMethodRepository) DeletePaymentMethodUseCase {
	return &deletePaymentMethodUseCase{methods: methods}
}

func (uc *deletePaymentMethodUseCase) Execute(ctx context.Context, userID, id string) error {
	if userID == "" || id == "" {
		return apperrors.ErrInvalidInput
	}
	existing, err := uc.methods.GetByID(ctx, userID, id)
	if err != nil {
		return err
	}
	if isSystemPaymentMethod(existing.Name) {
		return fmt.Errorf("%w: %q is a reserved payment method and can't be deleted", apperrors.ErrInvalidInput, existing.Name)
	}
	return uc.methods.Delete(ctx, userID, id)
}
