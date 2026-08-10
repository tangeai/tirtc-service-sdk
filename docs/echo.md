# 可嵌入的诊断 Echo

`pkg/whipecho` 提供可以叠加到现有 WHIP HTTP Handler 上的诊断能力。平台内部路由负责选择 Echo 请求，普通请求继续交给应用的业务 Handler。

对于被选中的会话，Echo 模式会替代正常业务行为，Echo 组件是 `Conn.Events()` 的唯一消费者。

## 进程内 HTTP Handler

标准 `net/http` 接入方式是：创建 Echo 能力、包装业务 Handler，并由应用管理路由和中间件注册：

```go
acceptor := tirtcx.NewWhipAcceptor(candidate)
echo, err := whipecho.NewHTTPAdapter(
    acceptor,
    "/custom/whip/echo/resource",
    whipecho.HTTPOptions{},
)
if err != nil {
    return err
}

auth := tirtcxauth.BearerInterceptor(serviceName, verifier)
mux.HandleFunc("POST /custom/whip", auth(echo.Wrap(businessHandler)))
mux.HandleFunc(
    "DELETE /custom/whip/echo/resource/{session_id}",
    echo.DeleteHandler(),
)

server := &http.Server{Addr: ":8080", Handler: mux}
go server.ListenAndServe()
// 应用退出时：
_ = server.Shutdown(shutdownContext)
_ = echo.Shutdown(shutdownContext)
```

对于上述 Location 前缀，应用需要注册：

- `POST /custom/whip`
- `DELETE /custom/whip/echo/resource/{session_id}`

POST 响应中的 `Location` 使用后一个路由。Location 前缀会被严格验证；尾部斜杠、相对路径或非规范路径都会返回错误，不会被静默规范化。

Adapter 负责诊断请求选择、响应格式和 Echo Session 生命周期。普通请求会在不读取 Body 的情况下交给业务 Handler。只有 POST 需要 Bearer Token 认证；DELETE 使用 `Location` 中不可预测的随机 Session ID，不传输也不验证 Token。不需要 Echo 的服务可以直接挂载业务 Handler。

`HTTPAdapter` 是有状态对象。进程退出时，应先停止接收 HTTP 请求，再调用 `echo.Shutdown(ctx)`，最后调用 `tirtcx.Stop()`。Shutdown 会拒绝新的 Echo Session、关闭全部活动 Session，并等待其 Goroutine 退出。

## 其他 HTTP 框架

`TryHandlePOST` 和 `ServeDelete` 是与框架无关的接入点。对于普通业务请求，`TryHandlePOST` 返回 `false`，且不会读取 Body。例如，Gin 服务可以复用标准 HTTP 认证拦截器，并保留正常的 Handler 链：

```go
whip := router.Group("/custom/whip")
whip.POST("",
    ginAuth(tirtcxauth.BearerInterceptor(serviceName, verifier)),
    echoMiddleware(echo),
    business.PostWHIP,
)
whip.DELETE("/echo/resource/:session_id", echoDeleteHandler(echo))
```

任何能够提供 `http.ResponseWriter` 和 `*http.Request` 的框架都可以使用相同模式。框架 Adapter 保留在宿主服务中，因此 `whipecho` 不依赖 Gin 或其他路由库。

## 示例应用

`examples/quick-start` 展示了 Echo 与普通 WHIP 业务 Handler 的组合方式。普通会话接收生成的 PCM 音频和 JPEG 视频，同一入口也提供 Echo 能力：

```bash
cd examples/quick-start
go run -tags tirtc_clib . \
  -service quick-start \
  -access-key "$TIRTC_ACCESS_KEY" \
  -public-key "$TIRTC_PUBLIC_KEY"
```

`examples/observable-service` 重点展示结构化日志、Stats 快照、Echo/认证 Observer 以及示例 `/metrics` 入口。

## 监控

`NewHTTPAdapter` 返回有状态的 Echo HTTP Adapter。`Stats` 是线程安全的快照，包含活动 Session Gauge 和单调递增 Counter：

```go
echo, err := whipecho.NewHTTPAdapter(
    acceptor,
    "/whip/echo/resource",
    options,
)
if err != nil {
    return err
}

stats := echo.Stats()
activeSessions.Set(float64(stats.ActiveSessions))
```

需要 Duration Histogram 或实时指标时，可以使用 `HTTPOptions.Observer`，无需轮询：

```go
options.Observer = whipecho.ObserverFuncs{
    OnSessionConnected: func(duration time.Duration) {
        connectDuration.Observe(duration.Seconds())
    },
    OnSessionClosed: func(reason whipecho.CloseReason, duration time.Duration) {
        closedTotal.WithLabelValues(string(reason)).Inc()
        sessionDuration.Observe(duration.Seconds())
    },
    OnConnectFailed: func(reason whipecho.ConnectFailureReason) {
        connectFailed.WithLabelValues(string(reason)).Inc()
    },
}
```

Observer 的 Reason、Stage、Media 和 Event 值均为范围有限的枚举。不要将 Session ID 或设备 ID 作为指标 Label。Observer 回调可能并发执行，应尽快返回。

Bearer Token 监控与 Echo Session 监控相互独立：

```go
auth := tirtcxauth.BearerInterceptor(serviceName, verifier,
    tirtcxauth.AuthObserverFuncs{
        OnAuthenticationFailed: func(reason tirtcxauth.AuthFailureReason) {
            authFailed.WithLabelValues(string(reason)).Inc()
        },
    },
)
```
