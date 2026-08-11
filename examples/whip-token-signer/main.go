package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/tangeai/tirtc-service-sdk/v2/pkg/tirtcxauth"
)

type options struct {
	service        string
	customRtcKeyID string
	privateKey     string
	deviceID       string
	rawQuery       string
	ttl            time.Duration
}

type connectionParams struct {
	PeerID string `json:"peer_id"`
	Token  string `json:"token"`
}

func main() {
	var opts options
	flag.StringVar(&opts.service, "service", "", "registered WHIP service name")
	flag.StringVar(&opts.customRtcKeyID, "custom-rtc-key-id", "", "registered Custom RTC Key ID")
	flag.StringVar(&opts.privateKey, "private-key", "", "Ed25519 private key used to issue the token")
	flag.StringVar(&opts.deviceID, "device-id", "", "authorized TiRTC device ID")
	flag.StringVar(&opts.rawQuery, "query", "", "optional raw peer_id query, without '?'")
	flag.DurationVar(&opts.ttl, "ttl", 30*24*time.Hour, "token lifetime")
	flag.Parse()

	params, err := signWHIPToken(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sign WHIP token:", err)
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(params); err != nil {
		fmt.Fprintln(os.Stderr, "write connection parameters:", err)
		os.Exit(1)
	}
}

func signWHIPToken(opts options) (connectionParams, error) {
	opts.service = strings.TrimSpace(opts.service)
	opts.customRtcKeyID = strings.TrimSpace(opts.customRtcKeyID)
	opts.deviceID = strings.TrimSpace(opts.deviceID)
	opts.rawQuery = strings.TrimPrefix(opts.rawQuery, "?")
	switch {
	case opts.service == "":
		return connectionParams{}, errors.New("-service is required")
	case opts.customRtcKeyID == "":
		return connectionParams{}, errors.New("-custom-rtc-key-id is required")
	case strings.TrimSpace(opts.privateKey) == "":
		return connectionParams{}, errors.New("-private-key is required")
	case opts.deviceID == "":
		return connectionParams{}, errors.New("-device-id is required")
	case opts.ttl <= 0:
		return connectionParams{}, errors.New("-ttl must be greater than zero")
	}

	peerURL := &url.URL{Scheme: "whips", Host: opts.service, RawQuery: opts.rawQuery}
	if peerURL.Hostname() != opts.service {
		return connectionParams{}, errors.New("-service must be a valid URI host")
	}
	if _, err := tirtcxauth.CanonicalQuery(opts.rawQuery); err != nil {
		return connectionParams{}, fmt.Errorf("invalid -query: %w", err)
	}

	issuer, err := tirtcxauth.NewEd25519TokenIssuer(map[string]string{
		opts.customRtcKeyID: opts.privateKey,
	})
	if err != nil {
		return connectionParams{}, fmt.Errorf("create token issuer: %w", err)
	}
	token, err := issuer.Issue(
		opts.service,
		opts.customRtcKeyID,
		opts.deviceID,
		opts.rawQuery,
		opts.ttl,
	)
	if err != nil {
		return connectionParams{}, fmt.Errorf("issue token: %w", err)
	}
	return connectionParams{
		PeerID: peerURL.String(),
		Token:  token,
	}, nil
}
