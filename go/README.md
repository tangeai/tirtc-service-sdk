# whip-sdk Go

Go module for TiRTC WHIP service implementations.

## Packages

- `pkg/tirtcxauth`: Ed25519 TiRTC-X token issuing and verification.
- `pkg/tirtcclib`: direct Go wrappers for `clib/include/tirtc/tiRTC.h`.
- `pkg/tirtcx`: TiRTC `Conn` object and TiRTC-X authenticated WHIP helpers.

`pkg/tirtcclib` is intentionally a thin layer. Its exported names mirror the C
API, for example `TiRtcWhipAccept`, `TiRtcSetOption`, and `TiRtcWhipConnect`.
HTTP, Bearer token validation, session IDs, and timeout behavior belong to
the application server. The SDK does not design HTTP endpoints, manage WHIP
Location resources, or implement DELETE termination semantics.

## Build Tags

By default, `pkg/tirtcclib` compiles to stubs returning `tirtcclib.ErrUnavailable`.
This lets auth and connection-layer tests run on development machines without native
TiRTC libraries.

Build the real clib wrapper with:

```bash
go build -tags tirtc_clib ./...
```

The `tirtc_clib` build uses `../clib/include/tirtc/tiRTC.h` as the only C API
header and links against native TiRTC libraries under `../clib/lib`:

- `libTiRTC.so` or `libTiRTC.a`

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

An example application-owned WHIP service is available at
`examples/whip-service`. It demonstrates one possible `POST /whip` and
`DELETE /resource/{id}` integration while keeping that HTTP behavior outside SDK
packages.

```bash
go run -tags tirtc_clib ./examples/whip-service \
  -ak ak1 \
  -sk "$PUBLIC_KEY" \
  -service wxvoip
```
