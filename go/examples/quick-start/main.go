package main

import (
	"flag"
	"log"
	"net/http"

	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/pkg/tirtcx"
	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/pkg/tirtcxauth"
	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/pkg/whipecho"
)

func main() {
	listen := flag.String("listen", ":8080", "HTTP listen address")
	serviceName := flag.String("service", "echo", "registered WHIP service name")
	accessKey := flag.String("access-key", "", "TiRTC-X access key")
	publicKey := flag.String("public-key", "", "Ed25519 public key used to verify TiRTC-X tokens")
	candidate := flag.String("candidate", "", "optional public candidate address")
	flag.Parse()

	verifier, err := tirtcxauth.NewEd25519TokenVerifier(map[string]string{*accessKey: *publicKey})
	if err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()
	_, err = whipecho.RegisterHTTPHandlers(mux, "/whip", whipecho.HTTPOptions{
		Candidate: *candidate,
		Enabled:   true,
	}, tirtcxauth.BearerInterceptor(*serviceName, verifier))
	if err != nil {
		log.Fatal(err)
	}

	if err := tirtcx.Init(); err != nil {
		log.Fatal(err)
	}
	defer tirtcx.Uninit()
	if err := tirtcx.Start(); err != nil {
		log.Fatal(err)
	}
	defer tirtcx.Stop()

	log.Printf("WHIP Echo quick start listening on %s", *listen)
	log.Fatal(http.ListenAndServe(*listen, mux))
}
