package auth

import (
	"testing"
	"time"
)

func TestAccessTokenRoundTrip(t *testing.T) {
	token, err := NewAccessToken("secret", "u_1", "t_1", "admin", 60*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	id, err := ParseAccessToken("secret", token)
	if err != nil {
		t.Fatal(err)
	}
	if id.UserID != "u_1" || id.TenantID != "t_1" || id.Role != "admin" {
		t.Fatalf("unexpected identity: %+v", id)
	}
	if _, err := ParseAccessToken("other", token); err == nil {
		t.Fatal("expected invalid signature")
	}
}

func TestRefreshTokenHash(t *testing.T) {
	raw, hash, err := NewRefreshToken()
	if err != nil {
		t.Fatal(err)
	}
	if raw == "" || hash == "" || raw == hash {
		t.Fatal("invalid refresh token pair")
	}
	if HashToken(raw) != hash {
		t.Fatal("hash mismatch")
	}
}
