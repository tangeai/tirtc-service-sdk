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
  "exp": 1710000300,
  "nonce": "<random>"
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

Verifiers may also inject a nonce verifier for replay protection. For example,
the Go SDK supports `WithNonceVerifier(func(*TokenClaims) error)`, which can be
backed by Redis `SETNX` with token expiry.
