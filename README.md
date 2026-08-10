# TiRTC 自定义 RTC 服务 SDK

`tirtc-service-sdk` 是用于开发 TiRTC 自定义 RTC 服务的 Go SDK。它提供连接认证、WHIP 会话管理、媒体与命令处理，以及 Echo 链路诊断能力。

## 项目结构

- [`pkg/tirtcx`](./pkg/tirtcx/)：TiRTC 引擎、连接和 WHIP 会话 API。
- [`pkg/tirtcxauth`](./pkg/tirtcxauth/)：Ed25519 Token 签发、验证和 HTTP Bearer 认证。
- [`pkg/whipecho`](./pkg/whipecho/)：可嵌入的 Echo 诊断能力。
- [`internal/tirtcclib`](./internal/tirtcclib/)：TiRTC 原生库的内部 Go 封装。
- [`examples/`](./examples/)：快速开始、Gin、可观测服务和 Token 签发示例。
- [`clib/`](./clib/)：TiRTC C 头文件和原生库安装说明。
- [`docs/`](./docs/)：认证算法、Go API 分层和 Echo 接入说明。

## 示例

示例位于 [`examples/`](./examples/)：

- [`quick-start`](./examples/quick-start/)：完整的自定义 RTC 服务快速开始示例。
- [`whip-token-signer`](./examples/whip-token-signer/)：生成 `peer_id` 和连接 Token。
- [`gin-service`](./examples/gin-service/)：Gin 框架接入示例。
- [`observable-service`](./examples/observable-service/)：日志、统计、观察器和监控指标示例。

运行方法和参数说明见[示例文档](./examples/README.md)。安装 TiRTC 原生库后，使用 `tirtc_clib` 构建标签编译真实服务；不使用该标签时可以运行纯 Go 单元测试。

```bash
make test
```
