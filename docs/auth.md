# TiRTC-X Ed25519 Token

WHIP tokens use Ed25519 signatures:

```text
v1.<payload_b64url>.<signature_b64url>
```

- `payload_b64url` is base64url without padding over the JSON claims.
- `signature_b64url` is base64url without padding over
  `ed25519.Sign(private_key, []byte(payload_b64url))`.

Claims:

```json
{
  "sub": "<device-id>",
  "scope": "connect:<service>:<sha256hex(canonical_query)>",
  "iss": "<access-key>",
  "iat": 1710000000,
  "exp": 1712592000
}
```

`canonical_query` is produced by parsing the WHIP request query and re-encoding
it with keys sorted according to standard URL query encoding.

Verifier APIs may accept an empty expected device ID. In that case the signature,
issuer, expiry, and scope are checked, while `sub` is returned in the claims
without being compared to an external value.

Language SDKs should expose key maps instead of parallel key slices:

- issuer private keys: `access_key -> private_key`
- verifier public keys: `access_key -> public_key`

Preferred key formats are PEM PKCS8 private keys and PEM PKIX public keys.
Language SDKs may also support raw base64url/base64/hex Ed25519 key bytes for
configuration systems that store keys without PEM wrapping.

The token is a reusable delegation credential until `exp`. It has no nonce and
the verifier performs no replay check. Protect individual connection attempts
with the optional `nonce` field in `POST /v1/connect`; TiRTC Access rejects a
reused request nonce. Current TiRTC device SDKs generate a fresh connect nonce
without changing the public `TiRtcWhipConnect` API.

## Go HTTP middleware

`BearerInterceptor` verifies the bearer token against the request's complete
raw query. A successful verification attaches a copy of the claims to the
request context:

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

`ClaimsFromContext` returns a value copy so one handler cannot mutate the
claims observed by another handler.

Apply the interceptor to WHIP POST handlers. Resource DELETE requests do not
carry a bearer token when Location contains a cryptographically random session
ID; possession of that Location authorizes deletion.
