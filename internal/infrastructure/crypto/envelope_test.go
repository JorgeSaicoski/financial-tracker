package crypto

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func testKey() []byte {
	return bytes.Repeat([]byte{0x42}, 32)
}

func TestSealOpenRoundtrip(t *testing.T) {
	key := testKey()
	ciphertext, err := seal(key, []byte("coffee at the corner store"))
	if err != nil {
		t.Fatal(err)
	}
	if ciphertext == "" {
		t.Fatal("seal returned empty ciphertext")
	}

	plaintext, err := open(key, ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if string(plaintext) != "coffee at the corner store" {
		t.Errorf("roundtrip mismatch: got %q", plaintext)
	}
}

func TestSealIsNonDeterministic(t *testing.T) {
	key := testKey()
	a, err := seal(key, []byte("same plaintext"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := seal(key, []byte("same plaintext"))
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("two seals of the same plaintext must differ (random nonce per call)")
	}
}

func TestOpenFailsWithWrongKey(t *testing.T) {
	ciphertext, err := seal(testKey(), []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	wrongKey := bytes.Repeat([]byte{0x99}, 32)
	if _, err := open(wrongKey, ciphertext); err == nil {
		t.Error("expected an error decrypting with the wrong key")
	}
}

func TestOpenFailsOnTamperedCiphertext(t *testing.T) {
	key := testKey()
	ciphertext, err := seal(key, []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-1] ^= 0xFF // flip a bit in the GCM tag/ciphertext
	tampered := base64.StdEncoding.EncodeToString(raw)

	if _, err := open(key, tampered); err == nil {
		t.Error("expected tampered ciphertext to fail authentication")
	}
}

func TestParseMasterKeyRejectsWrongLength(t *testing.T) {
	short := base64.StdEncoding.EncodeToString([]byte("too-short"))
	if _, err := ParseMasterKey(short); err == nil {
		t.Error("expected an error for a non-32-byte master key")
	}

	good := base64.StdEncoding.EncodeToString(testKey())
	key, err := ParseMasterKey(good)
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != 32 {
		t.Errorf("parsed key length = %d, want 32", len(key))
	}
}

func TestParseMasterKeyRejectsInvalidBase64(t *testing.T) {
	if _, err := ParseMasterKey("not valid base64!!"); err == nil {
		t.Error("expected an error for invalid base64")
	}
}

func TestParseHMACKeyRejectsShortKey(t *testing.T) {
	short := base64.StdEncoding.EncodeToString([]byte("short"))
	if _, err := ParseHMACKey(short); err == nil {
		t.Error("expected an error for a key shorter than 16 bytes")
	}

	good := base64.StdEncoding.EncodeToString(testKey())
	if _, err := ParseHMACKey(good); err != nil {
		t.Fatal(err)
	}
}
