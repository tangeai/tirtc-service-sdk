# TiRTC-X Ed25519 Token

业务服务端完成授权后，同时生成：

```text
peer_id = whips://{service_name}?{raw_query}
```

没有 Query 时省略 `?`。`service_name` 和 Query 必须与签发 Token 时使用的值一致。

WHIP Token 使用 Ed25519 签名：

```text
v1.<payload_b64url>.<signature_b64url>
```

- `payload_b64url` 是对 JSON Claims 进行不带填充的 Base64URL 编码。
- `signature_b64url` 是对 `ed25519.Sign(private_key, []byte(payload_b64url))` 的结果进行不带填充的 Base64URL 编码。

Claims：

```json
{
  "sub": "<device-id>",
  "scope": "connect:<service>:<sha256hex(canonical_query)>",
  "iss": "<access-key>",
  "iat": 1710000000,
  "exp": 1712592000
}
```

`canonical_query` 的输入可以包含或省略开头的 `?`。字段使用 `&` 分隔；无效的百分号转义和未转义的分号会被拒绝。按照 `application/x-www-form-urlencoded` 规则解析后：

1. 按参数名的字节序升序排列。
2. 同名参数保留原始值顺序。
3. 使用标准 URL Query 规则重新编码：空格使用 `+`，其他转义使用大写 `%HH`。
4. 无值参数编码为 `name=`。

例如：

```text
b=2&a=1&a=0  ->  a=1&a=0&b=2
```

Scope 的完整计算方式：

```text
query_hash = lowercase_hex(SHA-256(UTF-8(canonical_query)))
scope = "connect:" + service_name + ":" + query_hash
```

JSON Claims 使用 UTF-8 编码。先对实际 JSON Payload 进行无填充 Base64URL 编码，再使用 Ed25519 私钥签署 `payload_b64url` 的 ASCII 字节；签名输入不包含 `v1.`。

验证 API 可以接受空的预期设备 ID。此时仍会验证签名、Issuer、有效期和 Scope，但不会将 `sub` 与外部设备 ID 比较；`sub` 仍会随 Claims 返回。

各语言 SDK 应提供密钥映射，而不是相互平行的密钥切片：

- 签发方私钥：`access_key -> private_key`
- 验证方公钥：`access_key -> public_key`

推荐使用 PEM PKCS8 私钥和 PEM PKIX 公钥。对于不使用 PEM 包装保存密钥的配置系统，各语言 SDK 也可以支持 Base64URL、Base64 或十六进制编码的原始 Ed25519 密钥。

Token 在 `exp` 到期前是可重复使用的长期委托凭证。Token 不包含 Nonce，验证器也不执行防重放检查。单次建连通过 `POST /v1/connect` 的可选 `nonce` 字段防止重放；TiRTC Access 会拒绝重复的请求 Nonce。当前 TiRTC 设备 SDK 会自动为每次建连生成新的 Nonce，不改变公开的 `TiRtcWhipConnect` API。

## Go HTTP 中间件

`BearerInterceptor` 使用请求的完整原始 Query 验证 Bearer Token。验证成功后，会将 Claims 的副本写入请求 Context：

```go
handler := tirtcxauth.BearerInterceptor(serviceName, verifier)(
    http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        claims, ok := tirtcxauth.ClaimsFromContext(r.Context())
        if !ok {
            http.Error(w, "missing verified claims", http.StatusInternalServerError)
            return
        }
        slog.InfoContext(r.Context(), "accepted request",
            "device_id", claims.Subject,
            "access_key", claims.Issuer,
        )
    }),
)
```

`ClaimsFromContext` 返回值副本，避免一个 Handler 修改另一个 Handler 观察到的 Claims。

只对 WHIP POST Handler 使用该拦截器。当 `Location` 包含密码学安全的随机 Session ID 时，资源 DELETE 请求不携带 Bearer Token；持有该 `Location` 即具有删除权限。

## Python HTTP 认证

`verify_bearer` 返回与 Web 框架无关的认证结果：

```python
result = verify_bearer(authorization, service_name, raw_query, verifier)
if not result.authenticated:
    return result.error, result.status
claims = result.claims
```

必须传入完整 Raw Query。验证成功时 `claims.subject` 是 Token 绑定的设备 ID。
