# Embeddable diagnostic echo

`pkg/whipecho` provides a framework-independent diagnostic WHIP session. A
request opts in with the signed query parameter `_tg_mode=echo`; the parameter
is part of the canonical query covered by the TiRTC-X token.

Echo mode replaces the normal service behavior for that session. The Echo
component is the sole consumer of `Conn.Events()`.

## In-process HTTP handler

The normal integration is intentionally close to `net/http/pprof`: construct
the ready-to-use handler and mount it. Authentication is a separate
interceptor:

```go
acceptor := tirtcx.NewWhipAcceptor(candidate)
_, err := whipecho.RegisterHTTPHandlers(
    mux,
    "/custom/whip",
    acceptor,
    whipecho.HTTPOptions{
        Enabled:     true,
        RequireMode: true,
    },
    tirtcxauth.BearerInterceptor(serviceName, verifier),
)
if err != nil {
    return err
}

http.ListenAndServe(":8080", mux)
```

For the base path above, the registered routes are exactly
`POST /custom/whip` and
`DELETE /custom/whip/resource/{session_id}`. The POST response Location uses
the latter route. The base path is validated strictly; a trailing slash,
relative path, or non-canonical path returns an error rather than being
silently normalized.

The handlers own HTTP validation, response formatting, WHIP acceptance, and
session lifecycle. The interceptor verifies the bearer token against the
complete RawQuery before an Echo handler is entered.

Production services should default Echo to disabled and enable it only for
approved service/access-key combinations.

## Thin HTTP shell

`go/examples/quick-start` demonstrates the minimum mounting code:

```bash
cd go
go run -tags tirtc_clib ./examples/quick-start \
  -service echo \
  -access-key "$TIRTC_ACCESS_KEY" \
  -public-key "$TIRTC_PUBLIC_KEY"
```

When adapting this shell as a sidecar, bind it to the host's private interface
or Unix-socket HTTP listener and enable `-require-mode`. The host must proxy the
request without reconstructing its query:

- `POST /whip` including the original SDP body, `Authorization`, and RawQuery.
- `DELETE /whip/resource/{session_id}`.

The Echo process terminates WebRTC itself; no media IPC is required.

`go/examples/observable-service` builds on the quick start with structured
logging, Stats snapshots, Echo/auth observers, and a sample `/metrics`
endpoint.

## Monitoring

`RegisterHTTPHandlers` returns the stateful Echo service. `Stats` is a
thread-safe snapshot with an active-session gauge and monotonic counters:

```go
echo, err := whipecho.RegisterHTTPHandlers(
    mux,
    "/whip",
    acceptor,
    options,
    auth,
)
if err != nil {
    return err
}

stats := echo.Stats()
activeSessions.Set(float64(stats.ActiveSessions))
```

Use `HTTPOptions.Observer` for duration histograms or real-time metrics without
polling:

```go
options.Observer = whipecho.ObserverFuncs{
    OnSessionConnected: func(duration time.Duration) {
        connectDuration.Observe(duration.Seconds())
    },
    OnSessionClosed: func(reason whipecho.CloseReason, duration time.Duration) {
        closedTotal.WithLabelValues(string(reason)).Inc()
        sessionDuration.Observe(duration.Seconds())
    },
}
```

Observer reason, stage, media, and event values are bounded enums. Do not add
session or device IDs as metric labels. Observer callbacks may run
concurrently and should return quickly.

Bearer-token monitoring is separate from Echo session monitoring:

```go
auth := tirtcxauth.BearerInterceptor(serviceName, verifier,
    tirtcxauth.AuthObserverFuncs{
        OnAuthenticationFailed: func(reason tirtcxauth.AuthFailureReason) {
            authFailed.WithLabelValues(string(reason)).Inc()
        },
    },
)
```
