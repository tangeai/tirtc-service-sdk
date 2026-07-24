# whip-sdk

`whip-sdk` 是用于开发 TiRTC WHIP Service 的多语言 SDK 仓库。

仓库以统一的 C API 约定为基础，按语言组织 SDK：

- `clib/`：存放共用的 TiRTC C 头文件和原生库。
- `go/`：Go SDK 实现，是当前首先支持的语言。
- `python/`：预留给后续 Python SDK。
- `docs/`：各语言实现共用的协议和 SDK 说明。
- `examples/`：各语言的示例与入口。

## 当前状态

Go SDK 当前提供：

- 基于 Ed25519 的 TiRTC-X Token 签发与验证。
- 与 `tiRTC.h` 标识符保持一致的 C API 封装。
- 用于 WHIP 接入、建连和连接操作的 TiRTC `Conn` 对象层。

核心连接包不限定 HTTP 入口的设计。可选的 `pkg/whipecho` 提供可组合的
POST/DELETE 适配器，并保证其 `Location` 一致。

`go/examples/` 下的 Go 快速开始应用和可观测 Echo 示例展示了可复用的
HTTP 注册、认证、统计和 Observer 组合方式。

Python SDK 尚未实现。当前保留其目录，以便后续增加 Python 支持时保持仓库结构稳定。
