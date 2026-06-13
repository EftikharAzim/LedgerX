package service

import (
	"testing"
)

func TestInitAuthRejectsEmptySecret(t *testing.T) {
	if err := InitAuth(""); err == nil {
		t.Fatal("expected error for empty secret")
	}
}

func TestJWTRoundTrip(t *testing.T) {
	if err := InitAuth("test-secret"); err != nil {
		t.Fatal(err)
	}
	tok, err := GenerateJWT(42)
	if err != nil {
		t.Fatal(err)
	}
	uid, err := ParseJWT(tok)
	if err != nil {
		t.Fatal(err)
	}
	if uid != 42 {
		t.Fatalf("got uid %d, want 42", uid)
	}
}

func TestJWTWrongSecretRejected(t *testing.T) {
	if err := InitAuth("secret-a"); err != nil {
		t.Fatal(err)
	}
	tok, err := GenerateJWT(7)
	if err != nil {
		t.Fatal(err)
	}
	if err := InitAuth("secret-b"); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseJWT(tok); err == nil {
		t.Fatal("token signed with a different secret must be rejected")
	}
}

func TestJWTGarbageRejected(t *testing.T) {
	if err := InitAuth("test-secret"); err != nil {
		t.Fatal(err)
	}
	for _, tok := range []string{"", "abc", "a.b.c"} {
		if _, err := ParseJWT(tok); err == nil {
			t.Fatalf("token %q must be rejected", tok)
		}
	}
}

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := HashPassword("hunter22")
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckPassword(hash, "hunter22"); err != nil {
		t.Fatal("correct password rejected")
	}
	if err := CheckPassword(hash, "wrong"); err == nil {
		t.Fatal("wrong password accepted")
	}
}
