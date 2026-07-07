package tirtcxauth

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrUnknownAccessKey = errors.New("tirtcxauth: unknown access key")
	ErrMalformedToken   = errors.New("tirtcxauth: malformed token")
	ErrTokenExpired     = errors.New("tirtcxauth: token expired")
	ErrTokenNotYetValid = errors.New("tirtcxauth: token not yet valid")
	ErrTokenScope       = errors.New("tirtcxauth: token scope mismatch")
	ErrTokenSubject     = errors.New("tirtcxauth: token subject mismatch")
	ErrTokenIssuer      = errors.New("tirtcxauth: token issuer mismatch")
	ErrTokenSignature   = errors.New("tirtcxauth: token signature mismatch")
	ErrTokenNonce       = errors.New("tirtcxauth: token nonce rejected")
)

// TokenIssuer signs TiRTC-X WHIP tokens.
type TokenIssuer interface {
	Issue(service, accessKey, deviceID, queryString string, ttl time.Duration) (string, error)
}

// TokenVerifier verifies TiRTC-X WHIP tokens.
type TokenVerifier interface {
	Verify(service, deviceID, queryString, token string) (*TokenClaims, error)
}

type ed25519TokenIssuer struct {
	privateKeys map[string]ed25519.PrivateKey
}

type ed25519TokenVerifier struct {
	publicKeys  map[string]ed25519.PublicKey
	now         func() time.Time
	verifyNonce NonceVerifier
}

// NonceVerifier validates a verified token nonce. It can be backed by Redis or
// another shared store to reject replayed tokens.
type NonceVerifier func(*TokenClaims) error

type TokenVerifierOption func(*ed25519TokenVerifier)

func WithNonceVerifier(fn NonceVerifier) TokenVerifierOption {
	return func(v *ed25519TokenVerifier) {
		v.verifyNonce = fn
	}
}

// NewEd25519TokenIssuer creates a token issuer from access-key to private-key mappings.
//
// Key values may be PEM PKCS8 Ed25519 private keys, raw base64/base64url/hex
// Ed25519 seeds, or raw base64/base64url/hex Ed25519 private keys.
func NewEd25519TokenIssuer(privateKeys map[string]string) (TokenIssuer, error) {
	parsed := make(map[string]ed25519.PrivateKey, len(privateKeys))
	for accessKey, raw := range privateKeys {
		key, err := parseEd25519PrivateKey(raw)
		if err != nil {
			return nil, fmt.Errorf("private key %q: %w", accessKey, err)
		}
		parsed[accessKey] = key
	}
	return &ed25519TokenIssuer{privateKeys: parsed}, nil
}

// NewEd25519TokenVerifier creates a token verifier from access-key to public-key mappings.
//
// Key values may be PEM PKIX Ed25519 public keys or raw base64/base64url/hex
// Ed25519 public keys.
func NewEd25519TokenVerifier(publicKeys map[string]string, opts ...TokenVerifierOption) (TokenVerifier, error) {
	parsed := make(map[string]ed25519.PublicKey, len(publicKeys))
	for accessKey, raw := range publicKeys {
		key, err := parseEd25519PublicKey(raw)
		if err != nil {
			return nil, fmt.Errorf("public key %q: %w", accessKey, err)
		}
		parsed[accessKey] = key
	}
	verifier := &ed25519TokenVerifier{
		publicKeys: parsed,
		now:        time.Now,
	}
	for _, opt := range opts {
		opt(verifier)
	}
	return verifier, nil
}

func (i *ed25519TokenIssuer) Issue(service, accessKey, deviceID, queryString string, ttl time.Duration) (string, error) {
	privateKey, ok := i.privateKeys[accessKey]
	if !ok {
		return "", ErrUnknownAccessKey
	}
	scope, err := scopeFor(service, queryString)
	if err != nil {
		return "", err
	}

	now := time.Now().UTC()
	claims := TokenClaims{
		Subject:   deviceID,
		Scope:     scope,
		Issuer:    accessKey,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(ttl).Unix(),
		Nonce:     randomNonce(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	signature := ed25519.Sign(privateKey, []byte(payloadB64))
	return "v1." + payloadB64 + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (v *ed25519TokenVerifier) Verify(service, deviceID, queryString, token string) (*TokenClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != "v1" || parts[1] == "" || parts[2] == "" {
		return nil, ErrMalformedToken
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("%w: payload: %v", ErrMalformedToken, err)
	}
	var claims TokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("%w: claims: %v", ErrMalformedToken, err)
	}
	publicKey, ok := v.publicKeys[claims.Issuer]
	if !ok || claims.Issuer == "" {
		return nil, ErrUnknownAccessKey
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("%w: signature: %v", ErrMalformedToken, err)
	}
	if !ed25519.Verify(publicKey, []byte(parts[1]), signature) {
		return nil, ErrTokenSignature
	}
	if deviceID != "" && claims.Subject != deviceID {
		return nil, ErrTokenSubject
	}
	expectedScope, err := scopeFor(service, queryString)
	if err != nil {
		return nil, err
	}
	if claims.Scope != expectedScope {
		return nil, ErrTokenScope
	}
	now := v.now().UTC().Unix()
	if claims.IssuedAt > now {
		return nil, ErrTokenNotYetValid
	}
	if claims.ExpiresAt <= now {
		return nil, ErrTokenExpired
	}
	if v.verifyNonce != nil {
		if err := v.verifyNonce(&claims); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrTokenNonce, err)
		}
	}
	return &claims, nil
}

func randomNonce() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
