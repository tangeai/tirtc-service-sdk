package tirtcxauth

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestCanonicalQuerySortsParameters(t *testing.T) {
	got, err := CanonicalQuery("b=1&a=2")
	if err != nil {
		t.Fatal(err)
	}
	if got != "a=2&b=1" {
		t.Fatalf("CanonicalQuery = %q, want %q", got, "a=2&b=1")
	}
}

func TestEd25519TokenIssueVerify(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := NewEd25519TokenIssuer(map[string]string{
		"ak1": base64.RawURLEncoding.EncodeToString(privateKey),
	})
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewEd25519TokenVerifier(map[string]string{
		"ak1": base64.RawURLEncoding.EncodeToString(publicKey),
	})
	if err != nil {
		t.Fatal(err)
	}

	token, err := issuer.Issue("wxvoip", "ak1", "dev1", "b=1&a=2", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := verifier.Verify("wxvoip", "dev1", "a=2&b=1", token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.Subject != "dev1" || claims.Issuer != "ak1" {
		t.Fatalf("claims = %+v", claims)
	}
}

func TestEd25519TokenVerifyAllowsEmptyDeviceID(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := NewEd25519TokenIssuer(map[string]string{
		"ak1": base64.RawURLEncoding.EncodeToString(privateKey),
	})
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewEd25519TokenVerifier(map[string]string{
		"ak1": base64.RawURLEncoding.EncodeToString(publicKey),
	})
	if err != nil {
		t.Fatal(err)
	}

	token, err := issuer.Issue("wxvoip", "ak1", "dev1", "a=2", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := verifier.Verify("wxvoip", "", "a=2", token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.Subject != "dev1" {
		t.Fatalf("Subject = %q, want dev1", claims.Subject)
	}
}

func TestEd25519TokenVerifySelectsKeyFromIssuer(t *testing.T) {
	publicKey1, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	publicKey2, privateKey2, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := NewEd25519TokenIssuer(map[string]string{
		"ak2": base64.RawURLEncoding.EncodeToString(privateKey2),
	})
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewEd25519TokenVerifier(map[string]string{
		"ak1": base64.RawURLEncoding.EncodeToString(publicKey1),
		"ak2": base64.RawURLEncoding.EncodeToString(publicKey2),
	})
	if err != nil {
		t.Fatal(err)
	}

	token, err := issuer.Issue("wxvoip", "ak2", "dev1", "a=2", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := verifier.Verify("wxvoip", "dev1", "a=2", token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.Issuer != "ak2" {
		t.Fatalf("Issuer = %q, want ak2", claims.Issuer)
	}
}

func TestEd25519TokenVerifyCallsNonceVerifier(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := NewEd25519TokenIssuer(map[string]string{
		"ak1": base64.RawURLEncoding.EncodeToString(privateKey),
	})
	if err != nil {
		t.Fatal(err)
	}
	var got *TokenClaims
	verifier, err := NewEd25519TokenVerifier(map[string]string{
		"ak1": base64.RawURLEncoding.EncodeToString(publicKey),
	}, WithNonceVerifier(func(claims *TokenClaims) error {
		got = claims
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}

	token, err := issuer.Issue("wxvoip", "ak1", "dev1", "a=2", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := verifier.Verify("wxvoip", "dev1", "a=2", token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got == nil {
		t.Fatal("nonce verifier was not called")
	}
	if got.Nonce == "" || got.Nonce != claims.Nonce {
		t.Fatalf("nonce verifier claims = %+v, returned claims = %+v", got, claims)
	}
}

func TestEd25519TokenVerifyRejectsNonce(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := NewEd25519TokenIssuer(map[string]string{
		"ak1": base64.RawURLEncoding.EncodeToString(privateKey),
	})
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewEd25519TokenVerifier(map[string]string{
		"ak1": base64.RawURLEncoding.EncodeToString(publicKey),
	}, WithNonceVerifier(func(*TokenClaims) error {
		return fmt.Errorf("replay")
	}))
	if err != nil {
		t.Fatal(err)
	}

	token, err := issuer.Issue("wxvoip", "ak1", "dev1", "a=2", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.Verify("wxvoip", "dev1", "a=2", token); !errors.Is(err, ErrTokenNonce) {
		t.Fatalf("nonce err = %v, want %v", err, ErrTokenNonce)
	}
}

func TestEd25519TokenVerifyRejectsMismatches(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := NewEd25519TokenIssuer(map[string]string{
		"ak1": base64.RawURLEncoding.EncodeToString(privateKey),
	})
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewEd25519TokenVerifier(map[string]string{
		"ak1": base64.RawURLEncoding.EncodeToString(publicKey),
	})
	if err != nil {
		t.Fatal(err)
	}

	token, err := issuer.Issue("wxvoip", "ak1", "dev1", "a=2", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.Verify("wxvoip", "other", "a=2", token); !errors.Is(err, ErrTokenSubject) {
		t.Fatalf("subject mismatch err = %v, want %v", err, ErrTokenSubject)
	}
	if _, err := verifier.Verify("aichat", "dev1", "a=2", token); !errors.Is(err, ErrTokenScope) {
		t.Fatalf("scope mismatch err = %v, want %v", err, ErrTokenScope)
	}
	if _, err := verifier.Verify("wxvoip", "dev1", "a=3", token); !errors.Is(err, ErrTokenScope) {
		t.Fatalf("query mismatch err = %v, want %v", err, ErrTokenScope)
	}
}

func TestEd25519TokenVerifyRejectsExpiredToken(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := NewEd25519TokenIssuer(map[string]string{
		"ak1": base64.RawURLEncoding.EncodeToString(privateKey),
	})
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewEd25519TokenVerifier(map[string]string{
		"ak1": base64.RawURLEncoding.EncodeToString(publicKey),
	})
	if err != nil {
		t.Fatal(err)
	}

	token, err := issuer.Issue("wxvoip", "ak1", "dev1", "a=2", -time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.Verify("wxvoip", "dev1", "a=2", token); !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("expired err = %v, want %v", err, ErrTokenExpired)
	}
}
