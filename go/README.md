# whip-sdk Go

用于开发 TiRTC WHIP Service 的 Go Module。

## 包结构

- `pkg/tirtcxauth`：基于 Ed25519 的 TiRTC-X Token 签发与验证。
- `internal/tirtcclib`：`clib/include/tirtc/tiRTC.h` 的内部 Go 封装。
- `pkg/tirtcx`：TiRTC `Conn` 对象和不包含认证策略的 WHIP 辅助能力。
- `pkg/whipecho`：可嵌入的音频、视频和命令诊断 Echo Session。

`internal/tirtcclib` 是精简的内部实现层。应用应使用 `pkg/tirtcx`；Go 的 `internal` 边界会阻止应用直接使用 C API 封装。HTTP、Bearer Token 验证、Session ID 和超时策略由应用服务端负责。SDK 不设计 HTTP 入口、不管理 WHIP `Location` 资源，也不定义 DELETE 终止语义。

`pkg/whipecho` 是可选的高层能力，提供可组合的 `net/http` 操作，并允许调用方指定 Location 前缀。应用负责注册路由，并以标准中间件方式添加认证，详见 [`../docs/echo.md`](../docs/echo.md)。

返回的 HTTP Adapter 提供并发安全的 `Stats()` 快照，也可以通过可选的 `Observer` 接入应用自己的监控系统。

## Build Tag

默认情况下，`internal/tirtcclib` 会编译为返回不可用错误的 Stub，使认证和连接层测试可以在没有 TiRTC 原生库的开发环境中运行。

构建真实 clib 封装：

```bash
go build -tags tirtc_clib ./...
```

`tirtc_clib` 构建只使用 `../clib/include/tirtc/tiRTC.h` 作为 C API 头文件。本仓库不提交原生库二进制文件；构建前请按照 [`../clib/README.md`](../clib/README.md) 下载并安装 SDK。Go 封装需要：

- `libTiRTC.a`
- `libwebrtc.so`（TiRTC SDK 的运行时依赖）

应将 `libwebrtc.so` 与服务程序部署在同一目录，或将其目录加入 `LD_LIBRARY_PATH`。cgo 封装还会链接 SDK 加载所需的 `libdl`。

Go SDK 不会暴露更底层的 C 头文件。

## Token 签发

```go
issuer, err := tirtcxauth.NewEd25519TokenIssuer(map[string]string{
    "ak1": privateKeyPEMOrBase64,
})
token, err := issuer.Issue("wxvoip", "ak1", "device-1", "b=1&a=2", time.Minute)
```

## Token 验证

```go
verifier, err := tirtcxauth.NewEd25519TokenVerifier(map[string]string{
    "ak1": publicKeyPEMOrBase64,
})
claims, err := verifier.Verify("wxvoip", "device-1", "a=2&b=1", token)
```

## TiRTC-X Conn

```go
acceptor := tirtcx.NewWhipAcceptor("203.0.113.10") // candidate 可选

_ = tirtcx.Init()
defer tirtcx.Uninit()
_ = tirtcx.Start()
defer tirtcx.Stop()

// 外层 HTTP Handler 已通过 BearerInterceptor。
acceptContext, cancelAccept := context.WithTimeout(r.Context(), 10*time.Second)
defer cancelAccept()
session, err := acceptor.WhipAccept(acceptContext, offerSDP, tirtcx.ConnEventOptions{
    AudioBuffer:        16,
    DisconnectedBuffer: 4,
})
if err != nil {
    // 处理 Accept 错误
}

// 应用负责包括 201、Location 和 DELETE 在内的 HTTP 行为。
answerSDP := session.AnswerSDP()

connectContext, cancelConnect := context.WithTimeout(context.Background(), 30*time.Second)
defer cancelConnect()
conn, err := session.WaitConn(connectContext)
if err != nil {
    // 处理建连错误
}
defer session.Close()

audioFrame := <-conn.Events().Audio
// 处理 audioFrame.Info 和 audioFrame.Data。

_ = audioFrame
_ = answerSDP
```

`WaitConn` 支持并发或重复调用。每个调用方传入自己的等待 Context，并接收相同的最终建连结果。取消一次等待不会关闭 Session。传给 `WhipAccept` 的 Context 只控制同步生成 SDP Answer 的阶段。`WaitConn` 成功后，应在应用资源结束时显式关闭 Session。

`pkg/tirtcx` 封装 TiRTC 连接操作，不选择认证策略，也不提供 HTTP Handler。HTTP 服务应先应用 `tirtcxauth.BearerInterceptor`，再调用 `WhipAcceptor.WhipAccept`。

## 示例服务

- `examples/quick-start`：发送生成的音视频，同时提供 Echo 模式的普通 WHIP 业务 Handler。
- `examples/observable-service`：Stats、Observer、Metrics、结构化日志和 HTTP 优雅退出。
- `examples/gin-service`：框架中间件接入。
- `examples/whip-token-signer`：生成 `peer_id` 和连接 Token。

```bash
cd examples/quick-start
go run -tags tirtc_clib . \
  -access-key ak1 \
  -public-key "$PUBLIC_KEY" \
  -service quick-start
```

`examples` 使用独立的 `go.mod`，示例专用依赖不会成为 SDK Module 的依赖。
