# whip-sdk

`whip-sdk` 是用于开发 TiRTC WHIP 服务的多语言 SDK 仓库。

## 项目结构

- [`clib/`](./clib/)：共用的 TiRTC C 头文件、原生库安装说明和本地库目录。
- [`go/`](./go/)：Go SDK。
- [`python/`](./python/)：Python SDK。
- [`docs/`](./docs/)：认证、C API、Go/Python SDK 和 Echo 接入说明。

## 示例

Go 示例位于 [`go/examples/`](./go/examples/)：

- [`quick-start`](./go/examples/quick-start/)：完整的 WHIP 服务快速开始示例。
- [`whip-token-signer`](./go/examples/whip-token-signer/)：生成 `peer_id` 和连接令牌。
- [`gin-service`](./go/examples/gin-service/)：Gin 框架接入示例。
- [`observable-service`](./go/examples/observable-service/)：日志、统计、观察器和监控指标示例。

运行方法和参数说明见 [Go 示例文档](./go/examples/README.md)。

Python 示例位于 [`python/examples/`](./python/examples/)：

- [`quick_start.py`](./python/examples/quick_start.py)：完整的 WHIP 服务快速开始示例。
- [`whip_token_signer.py`](./python/examples/whip_token_signer.py)：生成 `peer_id` 和连接令牌。

运行方法和参数说明见 [Python 示例文档](./python/examples/README.md)。
