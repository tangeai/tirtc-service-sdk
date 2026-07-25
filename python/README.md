# Python SDK

Python SDK 与 Go SDK 使用相同的职责划分：

- `tirtcxauth`：Ed25519 TiRTC-X 令牌签发、验证和 HTTP Bearer 认证。
- `tirtcx`：不包含认证策略的 WHIP 接入、连接生命周期和媒体收发。
- `whipecho`：可嵌入的 Echo 会话、HTTP 适配和统计。

## 安装

```bash
cd python
python3 -m pip install .
```

运行测试：

```bash
python3 -m pip install -e '.[test]'
python3 -m pytest
```

## 原生库

`tirtcx` 通过 `ctypes` 调用 TiRTC 动态库。请从
[TiRTC SDK 下载页](https://docs.tange.ai/products/tirtc/download.html) 获取与部署平台匹配的 SDK，
解压后找到 Linux 的 `libTiRTC.so` 或 macOS 的 `libTiRTC.dylib`。

普通 `pip install .` 安装应通过环境变量指定动态库的绝对路径：

```bash
export TIRTC_PYTHON_LIBRARY='/absolute/path/to/libTiRTC.so'
```

只有直接从源码树运行或使用 editable 安装时，才可改为把动态库放入仓库的
`clib/lib/`。动态库及其运行时依赖必须来自同一 TiRTC SDK 发布包。

令牌和 HTTP 认证功能不依赖原生库。只有调用 `tirtcx.init()`、创建
`WhipAcceptor` 或使用连接 API 时才加载原生库。

## 快速使用

```python
import tirtcx
from tirtcxauth import Ed25519TokenVerifier, verify_bearer

verifier = Ed25519TokenVerifier({"your-access-key": public_key})
auth = verify_bearer(authorization, "your-service", raw_query, verifier)
if not auth.authenticated:
    return auth.status

tirtcx.init()
tirtcx.start()
acceptor = tirtcx.WhipAcceptor("203.0.113.10")
session = acceptor.whip_accept(offer_sdp)
answer_sdp = session.answer_sdp
conn = session.wait_conn(timeout=30)
```

应用负责普通业务会话的 HTTP 路由、`Location`、会话注册表和 `DELETE`。
`DELETE` 不传令牌，收到请求后删除资源并调用 `session.close()`。

完整说明见 [`../docs/python.md`](../docs/python.md)，示例见
[`examples/`](./examples/)。
