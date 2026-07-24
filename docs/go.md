# Go SDK

The Go SDK lives under `go/` and is split into four layers:

- `internal/tirtcclib`: internal `tiRTC.h` wrappers used by `pkg/tirtcx`.
- `pkg/tirtcxauth`: Ed25519 TiRTC-X token issuing and verification.
- `pkg/tirtcx`: TiRTC `Conn` object and authentication-independent WHIP helpers.
- `pkg/whipecho`: reusable Echo sessions and HTTP-handler registration.

The core connection packages do not own:

- HTTP endpoint design.
- WHIP `Location` resource management.
- `DELETE` termination semantics.

Those remain application-server responsibilities unless an optional
higher-level capability such as `pkg/whipecho` explicitly provides them.

The repository includes `go/examples/quick-start` for minimal integration and
`go/examples/observable-service` for Stats, Observer, metrics, and structured
logging.

## clib Mapping

The lower layer intentionally keeps C-style names:

- `TiRtcInit`
- `TiRtcUninit`
- `TiRtcStart`
- `TiRtcStop`
- `TiRtcSetOption`
- `TiRtcWhipAccept`
- `TiRtcWhipConnect`
- `TiRtcSendAudioStream`
- `TiRtcSendVideoStream`
- `TiRtcSendCommand`

`tirtcx.WhipAcceptor` starts the lower-level WHIP acceptance flow without
imposing an authentication policy. HTTP services should authenticate the
request with `tirtcxauth.BearerInterceptor` before invoking it. The acceptance
path is built on top of:

```c
int TiRtcWhipAccept(const char *offer, int offer_len,
                    const char *candidate,
                    TIRTCWHIPSERVERONSDPREADYCB whip_sdp_cb,
                    TIRTCCONNECTCALLBACK cb,
                    void *user);
```

## Conn Usage

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
    CommandBuffer:      16,
    DisconnectedBuffer: 4,
    SubscribeVideoHandler: func(streamID uint8) int {
        return 0 // Return the TiRTC subscription result synchronously.
    },
})
if err != nil {
    // handle acceptance error
}

// Return session.AnswerSDP() according to the application's HTTP design.
answerSDP := session.AnswerSDP()
_ = answerSDP

connectContext, cancelConnect := context.WithTimeout(context.Background(), 30*time.Second)
defer cancelConnect()
conn, err := session.WaitConn(connectContext)
if err != nil {
    // handle connection error
}
defer session.Close()

events := conn.Events()
command := <-events.Command
// Handle command.Cmd and command.Data.

_ = conn.SendCommand(0x10000, []byte("hello"))

<-conn.Done()
if err := conn.Err(); err != nil {
    // The connection ended with an error.
}
```

`WhipSession` owns the pending or connected `Conn`. `WaitConn` is safe for
concurrent and repeated calls. Each waiter supplies its own context; canceling
one waiter does not close the session. The context passed to `WhipAccept`
controls only the synchronous SDP-answer phase, so an HTTP handler can pass its
request context without canceling connection establishment after returning the
answer.

Applications own the HTTP resource ID, Location, registry, and DELETE policy;
DELETE should remove the application resource and call `session.Close()`.
With a cryptographically random resource ID, DELETE does not need a bearer
token; possession of the Location is the deletion capability.
Subscription decisions use `SubscribeVideoHandler` and
`SubscribeAudioHandler`, which return directly to the native callback and
cannot be left waiting on a response channel.

## Example Service

```bash
cd go/examples/quick-start
go run -tags tirtc_clib . \
  -access-key ak1 \
  -public-key "$PUBLIC_KEY" \
  -service quick-start
```

The quick-start example implements a normal WHIP business session that sends
synthetic PCM audio and JPEG video. The same `POST /whip` handler also has the
Echo capability, with Echo selection kept inside the SDK.

See `examples/observable-service` for `slog`, `HTTPAdapter.Stats()`, Echo and
authentication observers, and a sample `/metrics` endpoint.
