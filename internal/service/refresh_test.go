package service

import (
	"testing"
	"time"
)

func TestNewRefreshTokenUniqueAndHashed(t *testing.T) {
	raw1, hash1, err := NewRefreshToken()
	if err != nil {
		t.Fatal(err)
	}
	raw2, hash2, err := NewRefreshToken()
	if err != nil {
		t.Fatal(err)
	}
	if raw1 == raw2 {
		t.Fatal("two refresh tokens must not collide")
	}
	if hash1 == hash2 {
		t.Fatal("two token hashes must not collide")
	}
	if raw1 == hash1 {
		t.Fatal("hash must differ from the raw token")
	}
	// Hash must be a stable function of the raw token.
	if HashToken(raw1) != hash1 {
		t.Fatal("HashToken must reproduce the stored hash")
	}
	if len(raw1) != 64 { // 32 random bytes, hex-encoded
		t.Fatalf("unexpected raw token length %d", len(raw1))
	}
}

func TestAccessTTLConfigurable(t *testing.T) {
	orig := AccessTTL()
	defer SetAccessTTL(orig)

	SetAccessTTL(0) // ignored
	if AccessTTL() != orig {
		t.Fatal("non-positive TTL must be ignored")
	}
	SetAccessTTL(7 * time.Minute)
	if AccessTTL() != 7*time.Minute {
		t.Fatalf("got %v, want 7m", AccessTTL())
	}
}
