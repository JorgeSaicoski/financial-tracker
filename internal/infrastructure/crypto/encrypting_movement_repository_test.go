package crypto

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
	"github.com/JorgeSaicoski/financial-tracker/internal/infrastructure/sqlite"
)

// openTestDB spins up a throwaway SQLite database purely as a real,
// working repositories.MovementRepository/AccountRepository to decorate
// — this package tests the encryption *decorator*, not SQLite itself
// (which never uses it in production, see cmd/api/main.go).
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := sqlite.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func testMovement(userID, description string) *dto.MovementDTO {
	now := time.Now().UTC()
	return &dto.MovementDTO{
		UserID:        userID,
		Amount:        -450,
		Currency:      "usd",
		Description:   description,
		PaymentMethod: string(entities.PaymentMethodCash),
		Status:        string(entities.MovementStatusActive),
		SyncStatus:    string(entities.SyncStatusPending),
		Timestamp:     now,
		CreatedAt:     now,
	}
}

func TestEncryptingMovementRepositoryEncryptsAtRest(t *testing.T) {
	db := openTestDB(t)
	inner := sqlite.NewMovementRepository(db)
	cryptor := NewFieldCryptor(testKey(), newFakeUserDataKeyRepo())
	repo := NewEncryptingMovementRepository(inner, cryptor)
	ctx := context.Background()

	created, err := repo.Create(ctx, testMovement("user-1", "therapy session"))
	if err != nil {
		t.Fatal(err)
	}
	if created.Description != "therapy session" {
		t.Errorf("Create must return plaintext to the caller, got %q", created.Description)
	}

	// The raw row underneath must not contain the plaintext.
	rawRow, err := inner.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rawRow.Description == "therapy session" {
		t.Error("underlying repository stored plaintext description, expected ciphertext")
	}

	got, err := repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != "therapy session" {
		t.Errorf("GetByID = %q, want decrypted plaintext", got.Description)
	}
}

func TestEncryptingMovementRepositoryListByUserDecrypts(t *testing.T) {
	db := openTestDB(t)
	repo := NewEncryptingMovementRepository(sqlite.NewMovementRepository(db), NewFieldCryptor(testKey(), newFakeUserDataKeyRepo()))
	ctx := context.Background()

	for _, desc := range []string{"rent", "groceries", "gas"} {
		if _, err := repo.Create(ctx, testMovement("user-1", desc)); err != nil {
			t.Fatal(err)
		}
	}

	list, err := repo.ListByUser(ctx, "user-1", nil, nil, nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("want 3 movements, got %d", len(list))
	}
	seen := map[string]bool{}
	for _, m := range list {
		seen[m.Description] = true
	}
	for _, want := range []string{"rent", "groceries", "gas"} {
		if !seen[want] {
			t.Errorf("missing decrypted description %q in list results: %+v", want, list)
		}
	}
}

func TestEncryptingMovementRepositoryUpdateMetadataEncrypts(t *testing.T) {
	db := openTestDB(t)
	inner := sqlite.NewMovementRepository(db)
	repo := NewEncryptingMovementRepository(inner, NewFieldCryptor(testKey(), newFakeUserDataKeyRepo()))
	ctx := context.Background()

	created, err := repo.Create(ctx, testMovement("user-1", "original"))
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.UpdateMetadata(ctx, created.ID, "renamed description", nil, string(entities.PaymentMethodPix), nil); err != nil {
		t.Fatal(err)
	}

	rawRow, err := inner.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rawRow.Description == "renamed description" {
		t.Error("UpdateMetadata stored plaintext, expected ciphertext underneath")
	}

	got, err := repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != "renamed description" {
		t.Errorf("GetByID after UpdateMetadata = %q, want %q", got.Description, "renamed description")
	}
}

func TestEncryptingMovementRepositoryTransactEncryptsNestedWrites(t *testing.T) {
	db := openTestDB(t)
	repo := NewEncryptingMovementRepository(sqlite.NewMovementRepository(db), NewFieldCryptor(testKey(), newFakeUserDataKeyRepo()))
	ctx := context.Background()

	original, err := repo.Create(ctx, testMovement("user-1", "original purchase"))
	if err != nil {
		t.Fatal(err)
	}

	err = repo.Transact(ctx, func(tx repositories.MovementRepository) error {
		reversal := testMovement("user-1", "reversal of original purchase")
		reversal.Amount = -reversal.Amount
		reversal.CancelsMovementID = &original.ID
		_, err := tx.CreateReversal(ctx, reversal)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetByID(ctx, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ReversedByMovementID == nil {
		t.Fatal("reversal not linked")
	}
	reversal, err := repo.GetByID(ctx, *got.ReversedByMovementID)
	if err != nil {
		t.Fatal(err)
	}
	if reversal.Description != "reversal of original purchase" {
		t.Errorf("reversal created inside Transact must still be decryptable, got %q", reversal.Description)
	}
}

func TestEncryptingMovementRepositoryEmptyDescriptionStaysEmpty(t *testing.T) {
	db := openTestDB(t)
	repo := NewEncryptingMovementRepository(sqlite.NewMovementRepository(db), NewFieldCryptor(testKey(), newFakeUserDataKeyRepo()))
	ctx := context.Background()

	created, err := repo.Create(ctx, testMovement("user-1", ""))
	if err != nil {
		t.Fatal(err)
	}
	if created.Description != "" {
		t.Errorf("empty description must stay empty, got %q", created.Description)
	}
}
