package tirtcxauth

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
)

// CanonicalQuery returns the query string form used by TiRTC-X scopes.
//
// The input may be a raw query string with or without a leading question mark.
// Parameters are parsed according to net/url and re-encoded with keys sorted.
func CanonicalQuery(queryString string) (string, error) {
	queryString = strings.TrimPrefix(queryString, "?")
	values, err := url.ParseQuery(queryString)
	if err != nil {
		return "", fmt.Errorf("canonical query: %w", err)
	}
	return values.Encode(), nil
}

func scopeFor(service, queryString string) (string, error) {
	canonical, err := CanonicalQuery(queryString)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(canonical))
	return "connect:" + service + ":" + hex.EncodeToString(sum[:]), nil
}
