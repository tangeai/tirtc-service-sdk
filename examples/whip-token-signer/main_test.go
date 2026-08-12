package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"testing"
	"time"

	"github.com/tangeai/tirtc-service-sdk/v2/pkg/tirtcxauth"
)

func TestSignWHIPToken(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	params, err := signWHIPToken(options{
		service:        "echo",
		customRtcKeyID: "custom-rtc-key-1",
		privateKey:     base64.RawURLEncoding.EncodeToString(privateKey),
		deviceID:       "device-1",
		rawQuery:       "?room=test&_tg_mode=echo",
		ttl:            time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if params.PeerID != "whips://echo?room=test&_tg_mode=echo" {
		t.Fatalf("peer_id = %q", params.PeerID)
	}
	verifier, err := tirtcxauth.NewEd25519TokenVerifier(map[string]string{
		"custom-rtc-key-1": base64.RawURLEncoding.EncodeToString(publicKey),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.Verify("echo", "device-1", "_tg_mode=echo&room=test", params.Token); err != nil {
		t.Fatalf("verify token: %v", err)
	}
}

func TestSignWHIPTokenRequiresInputs(t *testing.T) {
	_, err := signWHIPToken(options{})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
