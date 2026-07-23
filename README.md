# whip-sdk

`whip-sdk` is the multilingual SDK repository for TiRTC WHIP services.

The repository is organized around a shared C API contract and language-specific
SDKs:

- `clib/`: shared TiRTC C headers and native library placement.
- `go/`: Go SDK implementation. This is the first supported language.
- `python/`: reserved for a future Python SDK.
- `docs/`: protocol and SDK notes shared by language implementations.
- `examples/`: language-specific examples and entry points.

## Current Status

The Go SDK provides:

- Ed25519 TiRTC-X token issuing and verification.
- A direct `tiRTC.h` wrapper layer with identifiers aligned to the C API.
- A TiRTC `Conn` object layer for WHIP accept/connect and connection operations.

The SDK does not own HTTP endpoint design, WHIP `Location` resource management,
or `DELETE` termination semantics. Application servers should implement those
parts on top of the SDK.

An echo service under `examples/go/echo-service` demonstrates a complete,
minimal HTTP integration and media loopback. A lower-level HTTP integration
example also remains under `go/examples/whip-service`.

The Python SDK is intentionally not implemented yet; its directory is reserved
so the repository layout remains stable when Python support is added.
