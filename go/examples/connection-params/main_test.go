package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"testing"
	"time"

	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/pkg/tirtcxauth"
)

func TestGenerateConnectionParams(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	params, err := generateConnectionParams(options{
		service:    "echo",
		accessKey:  "ak1",
		privateKey: base64.RawURLEncoding.EncodeToString(privateKey),
		deviceID:   "device-1",
		rawQuery:   "?room=test&mode=echo",
		ttl:        time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if params.PeerID != "whips://echo?room=test&mode=echo" {
		t.Fatalf("peer_id = %q", params.PeerID)
	}
	verifier, err := tirtcxauth.NewEd25519TokenVerifier(map[string]string{
		"ak1": base64.RawURLEncoding.EncodeToString(publicKey),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.Verify("echo", "device-1", "mode=echo&room=test", params.Token); err != nil {
		t.Fatalf("verify token: %v", err)
	}
}

func TestGenerateConnectionParamsRequiresInputs(t *testing.T) {
	_, err := generateConnectionParams(options{})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
