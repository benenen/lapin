package auth

import (
	"bytes"
	"strings"
	"testing"
)

func TestPasswordHashAndVerification(t *testing.T) {
	encoded, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(encoded, "correct horse battery staple") {
		t.Fatal("correct password did not verify")
	}
	if VerifyPassword(encoded, "wrong password") {
		t.Fatal("wrong password verified")
	}
	for _, invalid := range []string{"", "$argon2i$v=19$m=1,t=1,p=1$x$x", "$argon2id$v=19$bad$x$x", "$argon2id$v=19$m=1,t=1,p=1$!$eA", "$argon2id$v=19$m=1,t=1,p=1$eA$!"} {
		if VerifyPassword(invalid, "password") {
			t.Fatalf("invalid hash verified: %q", invalid)
		}
	}
}

func TestOpaqueTokenIsPrefixedAndHashed(t *testing.T) {
	raw, hash, err := NewOpaqueToken("lpn_")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(raw, "lpn_") || len(raw) < 40 {
		t.Fatalf("unexpected token: %q", raw)
	}
	if !bytes.Equal(hash, HashToken(raw)) || bytes.Equal(hash, HashToken(raw+"x")) {
		t.Fatal("token hash is not deterministic and input-specific")
	}
}
