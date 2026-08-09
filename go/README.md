# whip-sdk Go

用于开发 TiRTC WHIP 服务的 Go 模块。

## 包结构

- `pkg/tirtcxauth`：基于 Ed25519 的 TiRTC-X 令牌签发与验证。
- `internal/tirtcclib`：`clib/include/tirtc/tiRTC.h` 的内部 Go 封装。
- `pkg/tirtcx`：TiRTC `Conn` 对象和不包含认证策略的 WHIP 辅助能力。
- `pkg/whipecho`：可嵌入的音频、视频和命令诊断 Echo 会话。

`internal/tirtcclib` 是精简的内部实现层。应用应使用 `pkg/tirtcx`；Go 的 `internal` 边界会阻止应用直接使用 C API 封装。HTTP、Bearer 令牌验证、会话 ID 和超时策略由应用服务端负责。SDK 不设计 HTTP 入口、不管理 WHIP `Location` 资源，也不定义 DELETE 终止语义。

`pkg/whipecho` 是可选的高层能力，提供可组合的 `net/http` 操作，并允许调用方指定 `Location` 前缀。应用负责注册路由，并以标准中间件方式添加认证，详见 [`../docs/echo.md`](../docs/echo.md)。

返回的 HTTP 适配器提供并发安全的 `Stats()` 快照，也可以通过可选的观察器接入应用自己的监控系统。

## 构建标签

默认情况下，`internal/tirtcclib` 会编译为返回不可用错误的桩实现，使认证和连接层测试可以在没有 TiRTC 原生库的开发环境中运行。

构建真实 clib 封装：

```bash
go build -tags tirtc_clib ./...
```

`tirtc_clib` 构建只使用 `../clib/include/tirtc/tiRTC.h` 作为 C API 头文件。本仓库不提交原生库二进制文件；构建前请按照 [`../clib/README.md`](../clib/README.md) 下载并安装 SDK。Go 封装需要：

- `libTiRTC.a`
- `libwebrtc.so`（TiRTC SDK 的运行时依赖）

应将 `libwebrtc.so` 与服务程序部署在同一目录，或将其目录加入 `LD_LIBRARY_PATH`。cgo 封装还会链接 SDK 加载所需的 `libdl`。

Go SDK 不会暴露更底层的 C 头文件。

## 使用文档

- [令牌签发、验证与 HTTP 认证](../docs/auth.md)
- [Go SDK 分层、连接生命周期和 API 示例](../docs/go.md)
- [Echo 接入和监控](../docs/echo.md)

## 示例服务

- `examples/quick-start`：发送生成的音视频，同时提供 Echo 模式的普通 WHIP 业务处理器。
- `examples/observable-service`：统计、观察器、监控指标、结构化日志和 HTTP 优雅退出。
- `examples/gin-service`：框架中间件接入。
- `examples/whip-token-signer`：生成 `peer_id` 和连接令牌。

```bash
cd examples/quick-start
go run -tags tirtc_clib . \
  -access-key ak1 \
  -public-key "$PUBLIC_KEY" \
  -service quick-start
```

`examples` 使用独立的 `go.mod`，示例专用依赖不会成为 SDK 模块的依赖。
