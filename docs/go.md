# TiRTC 自定义 RTC 服务 SDK

Go Module 位于仓库根目录，分为四层：

- `internal/tirtcclib`：供 `pkg/tirtcx` 使用的内部 `tiRTC.h` 封装。
- `pkg/tirtcxauth`：基于 Ed25519 的 TiRTC-X Token 签发与验证。
- `pkg/tirtcx`：TiRTC `Conn` 对象和不包含认证策略的 WHIP 辅助能力。
- `pkg/whipecho`：可复用的 Echo Session 和 HTTP Handler 注册能力。

核心连接包不负责：

- HTTP 入口设计；
- WHIP `Location` 资源管理；
- `DELETE` 终止语义。

这些职责由应用服务端承担，除非使用 `pkg/whipecho` 等明确提供相应能力的可选高层组件。

仓库包含用于最小接入的 `examples/quick-start`，以及展示 Stats、Observer、Metrics 和结构化日志的 `examples/observable-service`。

## 日志

Go SDK 和 `whipecho` 使用标准库 `log/slog`。应用可在启动时调用 `slog.SetDefault`，统一配置日志格式、等级和输出位置；`whipecho.Options.Logger` 仅用于需要独立日志配置时覆盖默认 Logger。

`tirtcx.SetNativeLogLevel` 只控制 TiRTC 原生库的日志等级，应在 `tirtcx.Init` 前调用，不影响 Go `slog` 日志。

若服务会发送实时音频，可在 `tirtcx.Init` 前调用 `tirtcx.SetAudioRedundancyLossThreshold` 配置 TGTRP 音频冗余的丢包阈值。默认值为 10；`0` 始终发送冗余音频包，`100` 则关闭。该配置作用于全部 TGTRP 连接，返回 `-1` 表示当前原生库不可用或不支持该能力。

当 TiRTC 回调携带的连接句柄无法匹配到 `Conn` 时，`tirtcx` 会通过 `slog.Default()` 输出包含 `component`、`event` 和 `handle` 的警告日志。日志不包含媒体内容、Token 或业务数据。

## clib 映射

底层封装有意保留 C 风格名称：

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

`tirtcx.WhipAcceptor` 启动底层 WHIP Accept 流程，不附加认证策略。HTTP 服务应先使用 `tirtcxauth.BearerInterceptor` 验证请求，再调用该接口。Accept 流程基于：

```c
int TiRtcWhipAccept(const char *offer, int offer_len,
                    const char *candidate,
                    TIRTCWHIPSERVERONSDPREADYCB whip_sdp_cb,
                    TIRTCCONNECTCALLBACK cb,
                    void *user);
```

## Conn 使用方式

```go
if err := tirtcx.Init(); err != nil {
    return err
}
defer tirtcx.Uninit()
if err := tirtcx.Start(context.Background()); err != nil {
    return err
}
defer tirtcx.Stop()

acceptor := tirtcx.NewWhipAcceptor("203.0.113.10") // candidate 可选

// 外层 HTTP Handler 已通过 BearerInterceptor。
acceptContext, cancelAccept := context.WithTimeout(r.Context(), 10*time.Second)
defer cancelAccept()
session, err := acceptor.WhipAccept(acceptContext, offerSDP, tirtcx.ConnEventOptions{
    CommandBuffer:      16,
    DisconnectedBuffer: 4,
    SubscribeVideoHandler: func(streamID uint8) int {
        return 0 // 同步返回 TiRTC 订阅结果。
    },
})
if err != nil {
    return err
}

// 按照应用的 HTTP 设计返回 session.AnswerSDP()。
answerSDP := session.AnswerSDP()
_ = answerSDP

connectContext, cancelConnect := context.WithTimeout(context.Background(), 30*time.Second)
defer cancelConnect()
conn, err := session.WaitConn(connectContext)
if err != nil {
    session.Close()
    return err
}
defer session.Close()

events := conn.Events()
command := <-events.Command
// 处理 command.Cmd 和 command.Data。

_ = conn.SendCommand(0x10000, []byte("hello"))

<-conn.Done()
if err := conn.Err(); err != nil {
    // 连接因错误结束。
}
```

`WhipSession` 持有等待建连或已经建连的 `Conn`。`WaitConn` 支持并发和重复调用，每个调用方使用自己的 Context；取消其中一个等待不会关闭 Session。传给 `WhipAccept` 的 Context 只控制同步生成 SDP Answer 的阶段，因此 HTTP Handler 可以传入请求 Context，返回 Answer 后不会因此取消后续建连。

应用负责 HTTP Resource ID、`Location`、Registry 和 DELETE 策略。DELETE 应删除应用资源并调用 `session.Close()`。Resource ID 使用密码学安全的随机值时，DELETE 不需要 Bearer Token；持有 `Location` 即具有删除权限。

订阅决策通过 `SubscribeVideoHandler` 和 `SubscribeAudioHandler` 完成。它们直接从原生回调返回结果，不能等待 Response Channel。

## 示例服务

```bash
cd examples/quick-start
go run -tags tirtc_clib . \
  -custom-rtc-key-id custom-rtc-key-1 \
  -custom-rtc-key "$TIRTC_CUSTOM_RTC_KEY" \
  -service quick-start
```

快速开始示例实现了一个普通 WHIP 业务 Session，向设备发送生成的 PCM 音频和 JPEG 视频。同一个 `POST /whip` Handler 也提供 Echo 能力，Echo 选择逻辑保留在 SDK 内部。

`examples/observable-service` 展示了 `slog`、`HTTPAdapter.Stats()`、Echo 和认证 Observer，以及示例 `/metrics` 入口。
