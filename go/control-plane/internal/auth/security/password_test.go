package security

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestTokenHasherUsesStableSHA256Contract(t *testing.T) {
	hasher := NewTokenHasher()
	token := "api_example_token_for_contract"

	got, err := hasher.HashToken(token)
	if err != nil {
		t.Fatalf("HashToken returned error: %v", err)
	}

	sum := sha256.Sum256([]byte(token))
	want := hex.EncodeToString(sum[:])
	if got != want {
		t.Fatalf("HashToken = %q, want %q", got, want)
	}

	second, err := hasher.HashToken(token)
	if err != nil {
		t.Fatalf("HashToken second call returned error: %v", err)
	}
	if second != got {
		t.Fatalf("HashToken is not deterministic: first=%q second=%q", got, second)
	}

	if err := hasher.VerifyToken(got, token); err != nil {
		t.Fatalf("VerifyToken rejected matching token: %v", err)
	}
	if err := hasher.VerifyToken(got, token+"x"); err == nil {
		t.Fatalf("VerifyToken accepted mismatched token")
	}
}

func TestTokenPrefixTruncatesWithoutExposingFullToken(t *testing.T) {
	token := "api_1234567890abcdef_sensitive_tail"
	if got := TokenPrefix(token); got != "api_1234567890abcd" {
		t.Fatalf("TokenPrefix = %q", got)
	}

	short := "api_short"
	if got := TokenPrefix(short); got != short {
		t.Fatalf("TokenPrefix(short) = %q", got)
	}
}
