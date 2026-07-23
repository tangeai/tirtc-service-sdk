# Go Examples

Install the native dependency described in `../../clib/README.md` before
running either example.

## Quick start

`quick-start` shows the minimum integration: create a verifier, register the
Echo HTTP handlers with the authentication interceptor, initialize TiRTC, and
start the server.

```bash
cd go
go run -tags tirtc_clib ./examples/quick-start \
  -listen :8080 \
  -service echo \
  -access-key "$TIRTC_ACCESS_KEY" \
  -public-key "$TIRTC_PUBLIC_KEY" \
  -candidate 203.0.113.10
```

## Observable service

`observable-service` adds structured `slog` output, `Service.Stats()` snapshots,
Echo and authentication observers, duration aggregation, and a sample
Prometheus-text endpoint at `GET /metrics`.

```bash
cd go
go run -tags tirtc_clib ./examples/observable-service \
  -listen :8080 \
  -service echo \
  -access-key "$TIRTC_ACCESS_KEY" \
  -public-key "$TIRTC_PUBLIC_KEY" \
  -candidate 203.0.113.10
```

The metrics endpoint uses only the Go standard library so the SDK module does
not acquire a monitoring-library dependency. Production applications can map
the same Stats and Observer values into their existing metrics framework.
