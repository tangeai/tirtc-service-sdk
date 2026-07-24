# Go Examples

Install the native dependency described in `../../clib/README.md` before
running the examples.

The examples share `go/examples/go.mod`, so their application dependencies do
not become dependencies of the SDK module.

## Connection parameters

`connection-params` generates a `peer_id` and a signed delegation token for an
authorized device. It uses only the public `tirtcxauth` package and does not
require the native TiRTC libraries.

```bash
cd go/examples
go run ./connection-params \
  -service quick-start \
  -access-key "$TIRTC_ACCESS_KEY" \
  -private-key "$TIRTC_PRIVATE_KEY" \
  -device-id your_device_id \
  -query 'mode=echo'
```

The command writes `peer_id` and `token` as JSON. Use it for development and
integration testing. A production business server must authorize the device
before issuing the same parameters.

## Quick start

`quick-start` is a small but complete WHIP application. A normal device
connection receives a generated 440 Hz PCM stream and moving-color JPEG video.
The same WHIP entry also has the Echo capability, selected internally by the
platform. The example owns its normal session Location and DELETE lifecycle.
On shutdown it closes both normal and Echo sessions before stopping TiRTC-X.

```bash
cd go/examples/quick-start
go run -tags tirtc_clib . \
  -listen :8080 \
  -service quick-start \
  -access-key "$TIRTC_ACCESS_KEY" \
  -public-key "$TIRTC_PUBLIC_KEY" \
  -candidate 203.0.113.10
```

## Observable service

`observable-service` adds structured `slog` output, `HTTPAdapter.Stats()` snapshots,
Echo and authentication observers, duration aggregation, and a sample
Prometheus-text endpoint at `GET /metrics`.

```bash
cd go/examples/observable-service
go run -tags tirtc_clib . \
  -listen :8080 \
  -service echo \
  -access-key "$TIRTC_ACCESS_KEY" \
  -public-key "$TIRTC_PUBLIC_KEY" \
  -candidate 203.0.113.10
```

The metrics endpoint uses only the Go standard library so the SDK module does
not acquire a monitoring-library dependency. Production applications can map
the same Stats and Observer values into their existing metrics framework.

## Gin service

`gin-service` demonstrates authentication and Echo selection as Gin middleware
in front of a normal Gin WHIP handler. Authentication is scoped to the
`/whip` route group, and Echo DELETE receives Gin's path parameter explicitly:

```bash
cd go/examples/gin-service
go run -tags tirtc_clib . \
  -listen :8080 \
  -service gin-service \
  -access-key "$TIRTC_ACCESS_KEY" \
  -public-key "$TIRTC_PUBLIC_KEY" \
  -candidate 203.0.113.10
```

Gin is declared only in `go/examples/go.mod`; it is not a dependency of the SDK
module.
