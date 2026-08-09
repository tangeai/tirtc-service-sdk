# Python SDK

Python SDK 位于 `python/`，公开包与 Go SDK 对齐：

- `tirtcxauth`：TiRTC-X Token 与 HTTP Bearer 认证。
- `tirtcx`：WHIP Acceptor、Session、Conn 和媒体 API。
- `whipecho`：Echo Session Manager 与框架无关的 HTTP Adapter。

## 认证

```python
from tirtcxauth import Ed25519TokenVerifier, verify_bearer

verifier = Ed25519TokenVerifier({"ak1": public_key})
result = verify_bearer(
    request.headers.get("Authorization", ""),
    "your-service",
    request.query_string.decode(),
    verifier,
)
if not result.authenticated:
    return result.error, result.status
claims = result.claims
```

认证必须使用完整 Raw Query。`TokenClaims.subject` 是签发时绑定的设备 ID。

## 接受 WHIP 连接

```python
import tirtcx

tirtcx.init()
tirtcx.start()
acceptor = tirtcx.WhipAcceptor("203.0.113.10")

session = acceptor.whip_accept(
    offer_sdp,
    tirtcx.ConnEventOptions(
        audio_buffer=16,
        video_buffer=16,
        command_buffer=16,
        error_buffer=4,
        disconnected_buffer=4,
    ),
)

# HTTP 201 Body
answer_sdp = session.answer_sdp

conn = session.wait_conn(timeout=30)
command = conn.events.command.get()
conn.send_command(0x10000, b"hello")
conn.done.wait()
if conn.error:
    raise conn.error
```

`whip_accept()` 的超时只控制同步生成 SDP Answer 的阶段。`wait_conn()` 的调用方
可以分别设置等待超时；等待超时不会自动关闭 Session。

应用负责普通业务会话的资源 ID、`Location`、Registry 和 DELETE 策略。
DELETE 应删除应用资源并调用 `session.close()`。

## Echo HTTP Adapter

```python
from whipecho import HTTPAdapter

echo = HTTPAdapter(acceptor, "/whip/echo/resource")
response = echo.try_handle_post(raw_query, content_type, offer_sdp)
if response is not None:
    # 将 status、headers 和 body 写入 Web 框架响应。
    return response
```

普通业务请求返回 `None`，且 Body 不会被 Adapter 消费。Echo 请求由平台选择。
路由从 `Location` 取出 `session_id` 后调用 `echo.serve_delete(session_id)`。

`echo.stats()` 返回活跃会话、接入失败、建连失败、发送失败和事件丢弃等统计。
需要实时接入监控时，实现 `whipecho.Observer` 并通过 `HTTPAdapter(...,
observer=observer)` 传入；Observer 回调可能来自多个会话线程，必须快速返回且保证
线程安全。

## 生命周期

进程启动时依次调用 `tirtcx.init()`、`tirtcx.start()`。退出时先停止 HTTP
接入，调用 `echo.shutdown()` 并关闭普通业务 Session，最后调用
`tirtcx.stop()`、`tirtcx.uninit()`。

完整服务实现见 [`python/examples/quick_start.py`](../python/examples/quick_start.py)。
