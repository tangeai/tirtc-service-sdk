# whip-sdk Go

Go module for TiRTC WHIP service implementations.

## Packages

- `pkg/tirtcxauth`: Ed25519 TiRTC-X token issuing and verification.
- `pkg/tirtcclib`: direct Go wrappers for `clib/include/tirtc/tiRTC.h`.
- `pkg/tirtcx`: TiRTC `Conn` object and TiRTC-X authenticated WHIP helpers.
- `pkg/whipecho`: embeddable diagnostic audio/video/command echo sessions.

`pkg/tirtcclib` is intentionally a thin layer. Its exported names mirror the C
API, for example `TiRtcWhipAccept`, `TiRtcSetOption`, and `TiRtcWhipConnect`.
HTTP, Bearer token validation, session IDs, and timeout behavior belong to
the application server. The SDK does not design HTTP endpoints, manage WHIP
Location resources, or implement DELETE termination semantics.

`pkg/whipecho` is an optional higher-level capability that registers its
related `net/http` handlers under a caller-selected base path. Authentication
is applied independently as standard middleware. See `../docs/echo.md`.
The returned service exposes a concurrency-safe `Stats()` snapshot and accepts
an optional `Observer` for application-owned monitoring integrations.

## Build Tags

By default, `pkg/tirtcclib` compiles to stubs returning `tirtcclib.ErrUnavailable`.
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
client, err := tirtcx.New(tirtcx.Options{
    Service:   "wxvoip",
    Verifier:  verifier,
})
_ = err

_ = tirtcx.Init()
defer tirtcx.Uninit()
_ = tirtcx.Start()
defer tirtcx.Stop()

deviceID, result := client.WhipAccept(offerSDP, "a=2&b=1", token)
if result.Err != nil {
    // handle accept error
}

// The application owns HTTP behavior, including 201, Location, and DELETE.
answerSDP := result.AnswerSDP

conn, err := result.WaitConn(30 * time.Second)
conn.OnAudio(func(frame tirtcx.AudioFrame) {
    // handle audio
})

_ = answerSDP
_ = deviceID
```

`pkg/tirtcx` wraps TiRTC connection operations and verifies TiRTC-X query/token
for WHIP accept. It does not provide an HTTP handler.

## Example Service

`examples/quick-start` demonstrates the minimum Echo HTTP and authentication
integration. `examples/observable-service` adds Stats, observers, metrics, and
structured logging.

```bash
go run -tags tirtc_clib ./examples/quick-start \
  -access-key ak1 \
  -public-key "$PUBLIC_KEY" \
  -service echo
```
