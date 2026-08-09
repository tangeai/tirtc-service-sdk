package tirtcxauth

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
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

func TestCanonicalQueryCompatibilityVectors(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{"empty&x=", "empty=&x="},
		{"space=a+b&encoded=a%20b", "encoded=a+b&space=a+b"},
		{"name=%E6%B5%8B%E8%AF%95", "name=%E6%B5%8B%E8%AF%95"},
		{"raw=%FF&path=%2Ffoo%2Fbar", "path=%2Ffoo%2Fbar&raw=%FF"},
		{"a=1&a=0", "a=1&a=0"},
	}
	for _, test := range tests {
		got, err := CanonicalQuery(test.raw)
		if err != nil {
			t.Fatalf("CanonicalQuery(%q): %v", test.raw, err)
		}
		if got != test.want {
			t.Errorf("CanonicalQuery(%q) = %q, want %q", test.raw, got, test.want)
		}
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

func TestEd25519TokenPayloadOmitsNonce(t *testing.T) {
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
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token parts = %d, want 3", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(payload, []byte(`"nonce"`)) {
		t.Fatalf("payload contains nonce: %s", payload)
	}
	for range 2 {
		if _, err := verifier.Verify("wxvoip", "dev1", "a=2", token); err != nil {
			t.Fatalf("Verify reusable token: %v", err)
		}
	}
}

func TestEd25519TokenVerifyAcceptsLegacyNonceClaim(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := scopeFor("wxvoip", "a=2")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	payload, err := json.Marshal(map[string]any{
		"sub": "dev1", "scope": scope, "iss": "ak1",
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
		"nonce": "legacy-nonce",
	})
	if err != nil {
		t.Fatal(err)
	}
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	token := "v1." + payloadB64 + "." +
		base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, []byte(payloadB64)))
	verifier, err := NewEd25519TokenVerifier(map[string]string{
		"ak1": base64.RawURLEncoding.EncodeToString(publicKey),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.Verify("wxvoip", "dev1", "a=2", token); err != nil {
		t.Fatalf("Verify legacy token: %v", err)
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
