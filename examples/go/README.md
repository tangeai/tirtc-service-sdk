# Go Examples

The SDK packages do not provide an HTTP server. Application servers should own
their own endpoint, Location, and DELETE behavior.

Use `go/pkg/tirtcx` for TiRTC connection operations and TiRTC-X authenticated
WHIP accept. Use `go/pkg/tirtcxauth` directly when issuing tokens.

See `../../go/examples/whip-service` for a minimal application service example.
