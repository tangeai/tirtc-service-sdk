# whip-sdk Go

Go module for TiRTC WHIP service implementations.

## Packages

- `pkg/tirtcxauth`: Ed25519 TiRTC-X token issuing and verification.
- `internal/tirtcclib`: internal Go wrappers for `clib/include/tirtc/tiRTC.h`.
- `pkg/tirtcx`: TiRTC `Conn` object and authentication-independent WHIP helpers.
- `pkg/whipecho`: embeddable diagnostic audio/video/command echo sessions.

`internal/tirtcclib` is intentionally a thin implementation layer.
Applications should use `pkg/tirtcx`; the Go `internal` boundary prevents
direct use of the C wrapper. HTTP, Bearer token validation, session IDs, and
timeout behavior belong to the application server. The SDK does not design
HTTP endpoints, manage WHIP Location resources, or implement DELETE
termination semantics.

`pkg/whipecho` is an optional higher-level capability that provides composable
`net/http` operations with a caller-selected Location prefix. The application owns
route registration and applies authentication as standard middleware. See
`../docs/echo.md`.
The returned HTTP adapter exposes a concurrency-safe `Stats()` snapshot and accepts
an optional `Observer` for application-owned monitoring integrations.

## Build Tags

By default, `internal/tirtcclib` compiles to stubs returning an unavailable
error.
This lets auth and connection-layer tests run on development machines without native
TiRTC libraries.

Build the real clib wrapper with:

```bash
go build -tags tirtc_clib ./...
```

The `tirtc_clib` build uses `../clib/include/tirtc/tiRTC.h` as the only C API
header. Native binaries are not committed to this repository. Download and
install the required SDK as described in `../clib/README.md` before building.
The wrapper expects:

- `libTiRTC.a`
- `libwebrtc.so` (runtime dependency of the desktop TiRTC SDK)

Deploy `libwebrtc.so` next to the service binary or make its directory
available through `LD_LIBRARY_PATH`. The cgo wrapper also links `libdl`, which
is required by the desktop SDK loader.

No lower-level C headers are exposed through the Go SDK.

## Token Issuer

```go
issuer, err := tirtcxauth.NewEd25519TokenIssuer(map[string]string{
    "ak1": privateKeyPEMOrBase64,
})
token, err := issuer.Issue("wxvoip", "ak1", "device-1", "b=1&a=2", time.Minute)
```

## Token Verifier

```go
verifier, err := tirtcxauth.NewEd25519TokenVerifier(map[string]string{
    "ak1": publicKeyPEMOrBase64,
})
claims, err := verifier.Verify("wxvoip", "device-1", "a=2&b=1", token)
```

## TiRTC-X Conn

```go
acceptor := tirtcx.NewWhipAcceptor("203.0.113.10") // candidate is optional

_ = tirtcx.Init()
defer tirtcx.Uninit()
_ = tirtcx.Start()
defer tirtcx.Stop()

// The surrounding HTTP handler has already passed BearerInterceptor.
acceptContext, cancelAccept := context.WithTimeout(r.Context(), 10*time.Second)
defer cancelAccept()
session, err := acceptor.WhipAccept(acceptContext, offerSDP, tirtcx.ConnEventOptions{
    AudioBuffer:        16,
    DisconnectedBuffer: 4,
})
if err != nil {
    // handle acceptance error
}

// The application owns HTTP behavior, including 201, Location, and DELETE.
answerSDP := session.AnswerSDP()

connectContext, cancelConnect := context.WithTimeout(context.Background(), 30*time.Second)
defer cancelConnect()
conn, err := session.WaitConn(connectContext)
if err != nil {
	// handle connection error
}
defer session.Close()

audioFrame := <-conn.Events().Audio
// Handle audioFrame.Info and audioFrame.Data.

_ = audioFrame
_ = answerSDP
```

`WaitConn` may be called concurrently or repeatedly; every caller supplies its
own wait context and receives the same eventual connection result. Canceling a
wait does not close the session. The context passed to `WhipAccept` controls
only the synchronous SDP-answer phase. After `WaitConn` succeeds, close the
session explicitly when the application resource ends.

`pkg/tirtcx` wraps TiRTC connection operations without choosing an
authentication policy or providing an HTTP handler. HTTP services should apply
`tirtcxauth.BearerInterceptor` before calling `WhipAcceptor.WhipAccept`.

## Example Service

`examples/quick-start` demonstrates a normal WHIP business handler that sends
synthetic audio/video while also exposing Echo mode. `examples/observable-service`
adds Stats, observers, metrics, structured logging, and graceful HTTP shutdown.
`examples/gin-service` demonstrates framework middleware integration.

```bash
cd examples/quick-start
go run -tags tirtc_clib . \
  -access-key ak1 \
  -public-key "$PUBLIC_KEY" \
  -service quick-start
```

The `examples` tree has its own `go.mod`, so example-only dependencies do not
become dependencies of the SDK module.
