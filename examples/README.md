# Go 示例

运行需要 TiRTC 的示例前，请先按照 [`../clib/README.md`](../clib/README.md) 安装原生库。

所有示例共用 `examples/go.mod`，示例应用的依赖不会成为 SDK 模块的依赖。

## WHIP 令牌签发工具

`whip-token-signer` 为已授权设备生成 `peer_id` 和签名后的长期委托令牌。它只使用公开的 `tirtcxauth` 包，不依赖 TiRTC 原生库。

```bash
cd examples
go run ./whip-token-signer \
  -service quick-start \
  -custom-rtc-key-id "$TIRTC_CUSTOM_RTC_KEY_ID" \
  -private-key "$TIRTC_PRIVATE_KEY" \
  -device-id your_device_id \
  -query '_tg_mode=echo'
```

命令以 JSON 格式输出 `peer_id` 和 `token`，用于开发和联调。生产环境中，业务服务端必须先验证设备是否有权访问目标服务，再签发相同的连接参数。

## 快速开始

`quick-start` 是一个精简但完整的 WHIP 应用。普通设备连接会收到生成的 440 Hz PCM 音频和动态颜色 JPEG 视频。同一个 WHIP 入口也提供 Echo 能力，由平台内部选择。示例负责普通会话的 `Location` 和 DELETE 生命周期；退出时会先关闭普通会话和 Echo 会话，再停止 TiRTC-X。

```bash
cd examples/quick-start
go run -tags tirtc_clib . \
  -listen :8080 \
  -service quick-start \
  -custom-rtc-key-id "$TIRTC_CUSTOM_RTC_KEY_ID" \
  -custom-rtc-key "$TIRTC_CUSTOM_RTC_KEY" \
  -candidate 203.0.113.10
```

## 可观测服务

`observable-service` 展示结构化 `slog` 输出、`HTTPAdapter.Stats()` 快照、Echo 和认证观察器、时长聚合，以及 `GET /metrics` 的 Prometheus 文本格式示例。

```bash
cd examples/observable-service
go run -tags tirtc_clib . \
  -listen :8080 \
  -service echo \
  -custom-rtc-key-id "$TIRTC_CUSTOM_RTC_KEY_ID" \
  -custom-rtc-key "$TIRTC_CUSTOM_RTC_KEY" \
  -candidate 203.0.113.10
```

监控指标入口只使用 Go 标准库，因此不会给 SDK 模块增加监控库依赖。生产应用可以将相同的统计和观察器值接入已有监控系统。

## Gin 服务

`gin-service` 展示如何在普通 Gin WHIP 处理器前通过 Gin 中间件完成 POST 认证和 Echo 选择。DELETE 不做令牌认证，Echo DELETE 会显式接收 Gin 路径参数：

```bash
cd examples/gin-service
go run -tags tirtc_clib . \
  -listen :8080 \
  -service gin-service \
  -custom-rtc-key-id "$TIRTC_CUSTOM_RTC_KEY_ID" \
  -custom-rtc-key "$TIRTC_CUSTOM_RTC_KEY" \
  -candidate 203.0.113.10
```

Gin 只声明在 `examples/go.mod` 中，不是 SDK 模块的依赖。
