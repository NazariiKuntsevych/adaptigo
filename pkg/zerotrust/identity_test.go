package zerotrust_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"adaptigo/pkg/zerotrust"
)

func TestTokenManager_IssueAndVerifySuccess(t *testing.T) {
	secret := []byte("crypto-test-secret-key")
	tm := zerotrust.NewTokenManager(secret, 5*time.Minute)

	token, err := tm.Issue("service-client", "service-backend")
	if err != nil {
		t.Fatalf("Issue() = %v, want nil", err)
	}

	claims, err := tm.Verify(token, "service-backend")
	if err != nil {
		t.Fatalf("Verify() = %v, want nil", err)
	}

	if claims.Issuer != "service-client" {
		t.Errorf("Issuer = %q, want %q", claims.Issuer, "service-client")
	}
	if claims.Audience != "service-backend" {
		t.Errorf("Audience = %q, want %q", claims.Audience, "service-backend")
	}
	if claims.Nonce == "" {
		t.Errorf("Nonce = %q, want non-empty string", claims.Nonce)
	}
}

func TestTokenManager_VerifyTamperedSignature(t *testing.T) {
	secret := []byte("crypto-test-secret-key")
	tm := zerotrust.NewTokenManager(secret, 5*time.Minute)

	token, err := tm.Issue("service-client", "service-backend")
	if err != nil {
		t.Fatalf("Issue() = %v, want nil", err)
	}

	parts := strings.Split(token, ".")
	tamperedToken := parts[0] + ".tampered_signature_payload"

	_, err = tm.Verify(tamperedToken, "service-backend")
	if !errors.Is(err, zerotrust.ErrSignatureMismatch) {
		t.Errorf("Verify() = %v, want %v", err, zerotrust.ErrSignatureMismatch)
	}
}

func TestTokenManager_VerifyExpired(t *testing.T) {
	secret := []byte("crypto-test-secret-key")
	// Issue with negative TTL to immediately expire
	tm := zerotrust.NewTokenManager(secret, -2*time.Second)

	token, err := tm.Issue("service-client", "service-backend")
	if err != nil {
		t.Fatalf("Issue() = %v, want nil", err)
	}

	_, err = tm.Verify(token, "service-backend")
	if !errors.Is(err, zerotrust.ErrTokenExpired) {
		t.Errorf("Verify() = %v, want %v", err, zerotrust.ErrTokenExpired)
	}
}

func TestTokenManager_VerifyAudienceMismatch(t *testing.T) {
	secret := []byte("crypto-test-secret-key")
	tm := zerotrust.NewTokenManager(secret, 5*time.Minute)

	token, err := tm.Issue("service-client", "service-backend")
	if err != nil {
		t.Fatalf("Issue() = %v, want nil", err)
	}

	_, err = tm.Verify(token, "service-inventory")
	if !errors.Is(err, zerotrust.ErrAudienceMismatch) {
		t.Errorf("Verify() = %v, want %v", err, zerotrust.ErrAudienceMismatch)
	}
}
