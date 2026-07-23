# Go SDK

The Go SDK lives under `go/` and is split into four layers:

- `pkg/tirtcclib`: direct `tiRTC.h` wrappers. No HTTP, token, or session logic.
- `pkg/tirtcxauth`: Ed25519 TiRTC-X token issuing and verification.
- `pkg/tirtcx`: TiRTC `Conn` object and TiRTC-X authenticated WHIP helpers.
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

`tirtcx.WhipServer` keeps token verification and WHIP acceptance separate:
call `Auth` with the raw query and bearer token, then call `WhipAccept` with
the offer SDP. The acceptance path is built on top of:

```c
int TiRtcWhipAccept(const char *offer, int offer_len,
                    const char *candidate,
                    TIRTCWHIPSERVERONSDPREADYCB whip_sdp_cb,
                    TIRTCCONNECTCALLBACK cb,
                    void *user);
```

## Conn Usage

```go
server, err := tirtcx.NewWhipServer(tirtcx.Options{
    Service:   "wxvoip",
    Verifier:  verifier,
    Candidate: "203.0.113.10", // optional
})
if err != nil {
    // handle config error
}

_ = tirtcx.Init()
defer tirtcx.Uninit()
_ = tirtcx.Start()
defer tirtcx.Stop()

deviceID, err := server.Auth(rawQuery, bearerToken)
if err != nil {
    // return an authentication error
}

result, err := server.WhipAccept(offerSDP, tirtcx.ConnEventOptions{
    CommandBuffer:      16,
    DisconnectedBuffer: 4,
})
if err != nil {
    // handle acceptance error
}

// Return result.AnswerSDP according to the application's HTTP design.
answerSDP := result.AnswerSDP
_ = answerSDP
_ = deviceID

conn, err := result.WaitConn(30 * time.Second)
if err != nil {
    // handle connection error
}

events := conn.Events()
command := <-events.Command
// Handle command.Cmd and command.Data.

_ = conn.SendCommand(0x10000, []byte("hello"))
```

## Example Service

```bash
cd go
go run -tags tirtc_clib ./examples/quick-start \
  -access-key ak1 \
  -public-key "$PUBLIC_KEY" \
  -service echo
```

The quick-start example registers `POST /whip` and
`DELETE /whip/resource/{session_id}`, including matching `Location` responses
and the bearer-token authentication interceptor.

See `examples/observable-service` for `slog`, `Service.Stats()`, Echo and
authentication observers, and a sample `/metrics` endpoint.
