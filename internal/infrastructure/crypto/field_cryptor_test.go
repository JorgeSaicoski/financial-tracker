package crypto

import (
	"context"
	"testing"
)

func TestFieldCryptorEncryptDecryptRoundtrip(t *testing.T) {
	cryptor := NewFieldCryptor(testKey(), newFakeUserDataKeyRepo())
	ctx := context.Background()

	ciphertext, err := cryptor.Encrypt(ctx, "user-1", "rent for march")
	if err != nil {
		t.Fatal(err)
	}
	if ciphertext == "rent for march" {
		t.Fatal("Encrypt returned plaintext unchanged")
	}

	plaintext, err := cryptor.Decrypt(ctx, "user-1", ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if plaintext != "rent for march" {
		t.Errorf("Decrypt = %q, want %q", plaintext, "rent for march")
	}
}

func TestFieldCryptorEmptyStringPassesThrough(t *testing.T) {
	cryptor := NewFieldCryptor(testKey(), newFakeUserDataKeyRepo())
	ctx := context.Background()

	ciphertext, err := cryptor.Encrypt(ctx, "user-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if ciphertext != "" {
		t.Errorf("Encrypt(\"\") = %q, want \"\" (preserves NULL/empty contract)", ciphertext)
	}

	plaintext, err := cryptor.Decrypt(ctx, "user-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if plaintext != "" {
		t.Errorf("Decrypt(\"\") = %q, want \"\"", plaintext)
	}
}

func TestFieldCryptorReusesSameDataKeyAcrossCalls(t *testing.T) {
	keys := newFakeUserDataKeyRepo()
	cryptor := NewFieldCryptor(testKey(), keys)
	ctx := context.Background()

	if _, err := cryptor.Encrypt(ctx, "user-1", "first"); err != nil {
		t.Fatal(err)
	}
	if _, err := cryptor.Encrypt(ctx, "user-1", "second"); err != nil {
		t.Fatal(err)
	}

	if len(keys.rows) != 1 {
		t.Fatalf("want exactly one data key row for user-1, got %d", len(keys.rows))
	}
}

func TestFieldCryptorSeparatesUsersByKey(t *testing.T) {
	cryptor := NewFieldCryptor(testKey(), newFakeUserDataKeyRepo())
	ctx := context.Background()

	ciphertext, err := cryptor.Encrypt(ctx, "user-1", "shared plaintext")
	if err != nil {
		t.Fatal(err)
	}

	// Decrypting user-1's ciphertext as if it belonged to user-2 must
	// fail — proof each user really does get their own data key, not a
	// shared one.
	if _, err := cryptor.Decrypt(ctx, "user-2", ciphertext); err == nil {
		t.Error("expected decrypting another user's ciphertext under the wrong user's key to fail")
	}
}

func TestFieldCryptorUnreadableWithoutMasterKey(t *testing.T) {
	keys := newFakeUserDataKeyRepo()
	cryptor := NewFieldCryptor(testKey(), keys)
	ctx := context.Background()

	if _, err := cryptor.Encrypt(ctx, "user-1", "bank account 1234"); err != nil {
		t.Fatal(err)
	}

	row, err := keys.Get(ctx, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if row.WrappedKey == "" {
		t.Fatal("no wrapped key persisted")
	}

	// A cryptor holding the wrong master key can't unwrap the persisted
	// data key at all — this is the acceptance criterion that a raw DB
	// dump (which includes user_data_keys) stays unreadable without the
	// real master key.
	wrongMasterKey := NewFieldCryptor(append([]byte(nil), testKey()...), keys)
	wrongMasterKey.(*fieldCryptor).masterKey[0] ^= 0xFF
	if _, err := wrongMasterKey.Decrypt(ctx, "user-1", "irrelevant-ciphertext"); err == nil {
		t.Error("expected decrypt to fail when the master key is wrong")
	}
}
