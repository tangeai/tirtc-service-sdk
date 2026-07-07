package tirtcxauth

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
)

func parseEd25519PrivateKey(raw string) (ed25519.PrivateKey, error) {
	der, ok := parsePEM(raw)
	if ok {
		key, err := x509.ParsePKCS8PrivateKey(der)
		if err != nil {
			return nil, fmt.Errorf("parse pkcs8 private key: %w", err)
		}
		privateKey, ok := key.(ed25519.PrivateKey)
		if !ok {
			return nil, errors.New("private key is not Ed25519")
		}
		return privateKey, nil
	}

	buf, err := decodeKeyBytes(raw)
	if err != nil {
		return nil, err
	}
	switch len(buf) {
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(buf), nil
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(buf), nil
	default:
		return nil, fmt.Errorf("invalid Ed25519 private key length %d", len(buf))
	}
}

func parseEd25519PublicKey(raw string) (ed25519.PublicKey, error) {
	der, ok := parsePEM(raw)
	if ok {
		key, err := x509.ParsePKIXPublicKey(der)
		if err != nil {
			return nil, fmt.Errorf("parse pkix public key: %w", err)
		}
		publicKey, ok := key.(ed25519.PublicKey)
		if !ok {
			return nil, errors.New("public key is not Ed25519")
		}
		return publicKey, nil
	}

	buf, err := decodeKeyBytes(raw)
	if err != nil {
		return nil, err
	}
	if len(buf) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid Ed25519 public key length %d", len(buf))
	}
	return ed25519.PublicKey(buf), nil
}

func parsePEM(raw string) ([]byte, bool) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(raw)))
	if block == nil {
		return nil, false
	}
	return block.Bytes, true
}

func decodeKeyBytes(raw string) ([]byte, error) {
	s := strings.TrimSpace(raw)
	if len(s) == 64 || len(s) == 128 {
		if buf, err := hex.DecodeString(s); err == nil {
			return buf, nil
		}
	}
	for _, enc := range []*base64.Encoding{
		base64.RawURLEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.StdEncoding,
	} {
		if buf, err := enc.DecodeString(s); err == nil {
			return buf, nil
		}
	}
	return nil, errors.New("key must be PEM, base64url, base64, or hex encoded")
}
