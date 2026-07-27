package crypto

import (
	"context"
	"testing"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
	"github.com/JorgeSaicoski/financial-tracker/internal/infrastructure/sqlite"
)

func TestEncryptingAccountRepositoryEncryptsAtRest(t *testing.T) {
	db := openTestDB(t)
	inner := sqlite.NewAccountRepository(db)
	repo := NewEncryptingAccountRepository(inner, NewFieldCryptor(testKey(), newFakeUserDataKeyRepo()))
	ctx := context.Background()

	created, err := repo.Create(ctx, &dto.AccountDTO{
		UserID: "user-1", Name: "Chase Checking ****1234", Type: string(entities.AccountTypeBank),
		Currency: "usd", CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "Chase Checking ****1234" {
		t.Errorf("Create must return plaintext to the caller, got %q", created.Name)
	}

	rawRow, err := inner.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rawRow.Name == "Chase Checking ****1234" {
		t.Error("underlying repository stored plaintext name, expected ciphertext")
	}

	got, err := repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Chase Checking ****1234" {
		t.Errorf("GetByID = %q, want decrypted plaintext", got.Name)
	}
}

// TestEncryptingAccountRepositoryListByUserSortsDecryptedNames is the
// regression test for the bug this decorator specifically exists to
// avoid: the underlying repository's SQL `ORDER BY name` sorts
// ciphertext (each encryption uses a random nonce, unrelated to the
// plaintext's alphabetical order) — the decorator must re-sort on the
// real, decrypted names instead.
func TestEncryptingAccountRepositoryListByUserSortsDecryptedNames(t *testing.T) {
	db := openTestDB(t)
	repo := NewEncryptingAccountRepository(sqlite.NewAccountRepository(db), NewFieldCryptor(testKey(), newFakeUserDataKeyRepo()))
	ctx := context.Background()

	for _, name := range []string{"wallet", "brokerage", "savings"} {
		if _, err := repo.Create(ctx, &dto.AccountDTO{
			UserID: "user-1", Name: name, Type: string(entities.AccountTypeBank),
			Currency: "usd", CreatedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatal(err)
		}
	}

	got, err := repo.ListByUser(ctx, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 accounts, got %d", len(got))
	}
	if got[0].Name != "brokerage" || got[1].Name != "savings" || got[2].Name != "wallet" {
		t.Errorf("ListByUser not alphabetical by decrypted name: %s, %s, %s", got[0].Name, got[1].Name, got[2].Name)
	}
}
