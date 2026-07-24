# Embeddable diagnostic echo

`pkg/whipecho` provides a diagnostic capability that can be layered onto an
existing WHIP HTTP handler. Platform-internal routing selects Echo requests;
ordinary requests continue to the application's business handler.

Echo mode replaces the normal service behavior for that session. The Echo
component is the sole consumer of `Conn.Events()`.

## In-process HTTP handler

The normal integration follows standard `net/http` composition: construct the
Echo capability, wrap the business handler, and let the application own route
and middleware registration:

```go
acceptor := tirtcx.NewWhipAcceptor(candidate)
echo, err := whipecho.NewHTTPAdapter(
    acceptor,
    "/custom/whip/echo/resource",
    whipecho.HTTPOptions{},
)
if err != nil {
    return err
}

auth := tirtcxauth.BearerInterceptor(serviceName, verifier)
mux.HandleFunc("POST /custom/whip", auth(echo.Wrap(businessHandler)))
mux.HandleFunc(
    "DELETE /custom/whip/echo/resource/{session_id}",
    auth(echo.DeleteHandler()),
)

server := &http.Server{Addr: ":8080", Handler: mux}
// Start and gracefully stop server according to the application lifecycle.
_ = server.Shutdown(shutdownContext)
_ = echo.Shutdown(shutdownContext)
```

For the Location prefix above, the application registers
`POST /custom/whip` and
`DELETE /custom/whip/echo/resource/{session_id}`. The POST response Location uses
the latter route. The Location prefix is validated strictly; a trailing slash,
relative path, or non-canonical path returns an error rather than being silently
normalized.

The adapter owns diagnostic selection, response formatting, and Echo
session lifecycle. Normal requests are delegated without consuming their body.
The application applies the same bearer-token interceptor to both handlers. A
service omits Echo by mounting its business handler directly.

`HTTPAdapter` is stateful. During process shutdown, stop accepting HTTP
requests and call `echo.Shutdown(ctx)` before `tirtcx.Stop()`. Shutdown rejects
new Echo sessions, closes all active sessions, and waits for their goroutines.

## Other HTTP frameworks

`TryHandlePOST` and `ServeDelete` are the framework-neutral integration points.
`TryHandlePOST` returns false for an ordinary business request without consuming
its body. For example, a Gin service can adapt the standard HTTP authentication
interceptor once and keep its normal handler chain:

```go
whip := router.Group("/custom/whip")
whip.Use(ginAuth(tirtcxauth.BearerInterceptor(serviceName, verifier)))
whip.POST("", echoMiddleware(echo), business.PostWHIP)
whip.DELETE("/echo/resource/:session_id", echoDeleteHandler(echo))
```

The same pattern works with any framework that exposes an
`http.ResponseWriter` and `*http.Request`. Framework adapters remain in the
host service, so `whipecho` does not depend on Gin or another router.

## Example application

`go/examples/quick-start` demonstrates composition with a normal WHIP
business handler. Normal sessions receive generated PCM audio and JPEG video;
the same endpoint also provides Echo:

```bash
cd go/examples/quick-start
go run -tags tirtc_clib . \
  -service quick-start \
  -access-key "$TIRTC_ACCESS_KEY" \
  -public-key "$TIRTC_PUBLIC_KEY"
```

`go/examples/observable-service` focuses on structured logging, Stats
snapshots, Echo/auth observers, and a sample `/metrics` endpoint.

## Monitoring

`NewHTTPAdapter` returns the stateful Echo HTTP adapter. `Stats` is a
thread-safe snapshot with an active-session gauge and monotonic counters:

```go
echo, err := whipecho.NewHTTPAdapter(
    acceptor,
    "/whip/echo/resource",
    options,
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
    OnConnectFailed: func(reason whipecho.ConnectFailureReason) {
        connectFailed.WithLabelValues(string(reason)).Inc()
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
