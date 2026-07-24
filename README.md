# whip-sdk

`whip-sdk` 是用于开发 TiRTC WHIP Service 的多语言 SDK 仓库。

## 项目结构

- [`clib/`](./clib/)：共用的 TiRTC C 头文件、原生库安装说明和本地库目录。
- [`go/`](./go/)：Go SDK。
- [`python/`](./python/)：Python SDK 目录。
- [`docs/`](./docs/)：认证、C API、Go SDK 和 Echo 接入说明。

## 示例

Go 示例位于 [`go/examples/`](./go/examples/)：

- [`quick-start`](./go/examples/quick-start/)：完整的 WHIP Service 快速开始示例。
- [`whip-token-signer`](./go/examples/whip-token-signer/)：生成 `peer_id` 和连接 Token。
- [`gin-service`](./go/examples/gin-service/)：Gin 框架接入示例。
- [`observable-service`](./go/examples/observable-service/)：日志、统计、Observer 和 Metrics 示例。

运行方法和参数说明见 [Go 示例文档](./go/examples/README.md)。
