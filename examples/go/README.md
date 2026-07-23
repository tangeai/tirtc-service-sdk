# Go Examples

## Echo service

`echo-service` is a minimal customer-owned WHIP service. It verifies TiRTC-X
tokens, implements the WHIP HTTP resource lifecycle, and echoes incoming audio
and video frames.

Install the native dependency described in `../../clib/README.md`, then run:

```bash
cd examples/go/echo-service
go run -tags tirtc_clib . \
  -listen :8080 \
  -service echo \
  -access-key "$TIRTC_ACCESS_KEY" \
  -public-key "$TIRTC_PUBLIC_KEY" \
  -candidate 203.0.113.10
```

Omit `-candidate` when the service can advertise its local interface directly.
The application, rather than the SDK, owns the HTTP endpoint, `Location`
resource, and `DELETE` behavior.
