from __future__ import annotations

import base64
import hashlib
import json
import re
import time
from dataclasses import dataclass
from enum import Enum
from typing import Callable, Mapping, Optional
from urllib.parse import unquote_to_bytes

from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric.ed25519 import (
    Ed25519PrivateKey,
    Ed25519PublicKey,
)


class TokenError(ValueError):
    """Token 格式、签名或声明无效。"""


class AuthFailureReason(str, Enum):
    MISSING_TOKEN = "missing_token"
    INVALID_TOKEN = "invalid_token"
    MISCONFIGURED = "misconfigured"


@dataclass(frozen=True)
class TokenClaims:
    subject: str
    scope: str
    issuer: str
    issued_at: int
    expires_at: int

    @classmethod
    def from_dict(cls, value: Mapping[str, object]) -> "TokenClaims":
        try:
            if not all(
                isinstance(value[name], str) for name in ("sub", "scope", "iss")
            ):
                raise TypeError
            if not all(
                isinstance(value[name], int) and not isinstance(value[name], bool)
                for name in ("iat", "exp")
            ):
                raise TypeError
            return cls(
                subject=value["sub"],
                scope=value["scope"],
                issuer=value["iss"],
                issued_at=value["iat"],
                expires_at=value["exp"],
            )
        except (KeyError, TypeError, ValueError) as exc:
            raise TokenError("malformed token claims") from exc

    def as_dict(self) -> dict[str, object]:
        return {
            "sub": self.subject,
            "scope": self.scope,
            "iss": self.issuer,
            "iat": self.issued_at,
            "exp": self.expires_at,
        }


@dataclass(frozen=True)
class AuthResult:
    claims: Optional[TokenClaims]
    status: int
    error: Optional[str] = None
    reason: Optional[AuthFailureReason] = None

    @property
    def authenticated(self) -> bool:
        return self.claims is not None


def _b64url_encode(value: bytes) -> str:
    return base64.urlsafe_b64encode(value).rstrip(b"=").decode("ascii")


def _b64url_decode(value: str) -> bytes:
    padding = "=" * (-len(value) % 4)
    try:
        return base64.urlsafe_b64decode(value + padding)
    except Exception as exc:
        raise TokenError("malformed base64url value") from exc


def canonical_query(query_string: str) -> str:
    """返回与 Go SDK `CanonicalQuery` 相同的 Query 规范形式。"""
    raw = query_string[1:] if query_string.startswith("?") else query_string
    if re.search(r"%(?![0-9A-Fa-f]{2})", raw) or ";" in raw:
        raise TokenError("invalid query string")
    values: dict[bytes, list[bytes]] = {}
    try:
        for field in raw.split("&"):
            if not field:
                continue
            key, separator, value = field.partition("=")
            decoded_key = unquote_to_bytes(key.replace("+", " "))
            decoded_value = unquote_to_bytes(value.replace("+", " ")) if separator else b""
            values.setdefault(decoded_key, []).append(decoded_value)
    except (UnicodeEncodeError, ValueError) as exc:
        raise TokenError("invalid query string") from exc
    return "&".join(
        f"{_query_escape(key)}={_query_escape(value)}"
        for key in sorted(values)
        for value in values[key]
    )


def _query_escape(value: bytes) -> str:
    """按 Go net/url.QueryEscape 的规则编码任意字节。"""
    encoded: list[str] = []
    for byte in value:
        if (
            ord("a") <= byte <= ord("z")
            or ord("A") <= byte <= ord("Z")
            or ord("0") <= byte <= ord("9")
            or byte in b"-_.~"
        ):
            encoded.append(chr(byte))
        elif byte == ord(" "):
            encoded.append("+")
        else:
            encoded.append(f"%{byte:02X}")
    return "".join(encoded)


def _scope_for(service: str, query_string: str) -> str:
    digest = hashlib.sha256(canonical_query(query_string).encode()).hexdigest()
    return f"connect:{service}:{digest}"


def _decode_text_key(raw: str) -> bytes:
    value = raw.strip()
    if len(value) in (64, 128):
        try:
            return bytes.fromhex(value)
        except ValueError:
            pass
    return _b64url_decode(value)


def _private_key(raw: str) -> Ed25519PrivateKey:
    value = raw.strip().encode()
    if value.startswith(b"-----BEGIN"):
        key = serialization.load_pem_private_key(value, password=None)
        if not isinstance(key, Ed25519PrivateKey):
            raise TokenError("private key is not Ed25519")
        return key
    data = _decode_text_key(raw)
    if len(data) == 64:
        data = data[:32]
    if len(data) != 32:
        raise TokenError(f"invalid Ed25519 private key length {len(data)}")
    return Ed25519PrivateKey.from_private_bytes(data)


def _public_key(raw: str) -> Ed25519PublicKey:
    value = raw.strip().encode()
    if value.startswith(b"-----BEGIN"):
        key = serialization.load_pem_public_key(value)
        if not isinstance(key, Ed25519PublicKey):
            raise TokenError("public key is not Ed25519")
        return key
    data = _decode_text_key(raw)
    if len(data) != 32:
        raise TokenError(f"invalid Ed25519 public key length {len(data)}")
    return Ed25519PublicKey.from_public_bytes(data)


class Ed25519TokenIssuer:
    def __init__(self, private_keys: Mapping[str, str]) -> None:
        self._keys = {name: _private_key(value) for name, value in private_keys.items()}

    def issue(
        self,
        service: str,
        access_key: str,
        device_id: str,
        query_string: str,
        ttl_seconds: float,
    ) -> str:
        key = self._keys.get(access_key)
        if key is None:
            raise TokenError("unknown access key")
        now = int(time.time())
        claims = TokenClaims(
            subject=device_id,
            scope=_scope_for(service, query_string),
            issuer=access_key,
            issued_at=now,
            expires_at=int(now + ttl_seconds),
        )
        payload = json.dumps(
            claims.as_dict(), separators=(",", ":"), ensure_ascii=False
        ).encode()
        encoded = _b64url_encode(payload)
        return f"v1.{encoded}.{_b64url_encode(key.sign(encoded.encode()))}"


class Ed25519TokenVerifier:
    def __init__(
        self,
        public_keys: Mapping[str, str],
        now: Callable[[], float] = time.time,
    ) -> None:
        self._keys = {name: _public_key(value) for name, value in public_keys.items()}
        self._now = now

    def verify(
        self, service: str, device_id: str, query_string: str, token: str
    ) -> TokenClaims:
        parts = token.split(".")
        if len(parts) != 3 or parts[0] != "v1" or not parts[1] or not parts[2]:
            raise TokenError("malformed token")
        try:
            raw_claims = json.loads(_b64url_decode(parts[1]))
        except (json.JSONDecodeError, UnicodeDecodeError) as exc:
            raise TokenError("malformed token claims") from exc
        claims = TokenClaims.from_dict(raw_claims)
        key = self._keys.get(claims.issuer)
        if key is None or not claims.issuer:
            raise TokenError("unknown access key")
        try:
            key.verify(_b64url_decode(parts[2]), parts[1].encode())
        except Exception as exc:
            raise TokenError("token signature mismatch") from exc
        if device_id and claims.subject != device_id:
            raise TokenError("token subject mismatch")
        if claims.scope != _scope_for(service, query_string):
            raise TokenError("token scope mismatch")
        now = int(self._now())
        if claims.issued_at > now:
            raise TokenError("token not yet valid")
        if claims.expires_at <= now:
            raise TokenError("token expired")
        return claims


def verify_bearer(
    authorization: str,
    service: str,
    raw_query: str,
    verifier: Optional[Ed25519TokenVerifier],
    observer: Optional[Callable[[Optional[AuthFailureReason]], None]] = None,
) -> AuthResult:
    """验证 HTTP Authorization，并返回可由任意框架使用的结果。"""
    parts = authorization.split()
    if len(parts) != 2 or parts[0].lower() != "bearer" or not parts[1]:
        if observer:
            observer(AuthFailureReason.MISSING_TOKEN)
        return AuthResult(None, 401, "missing bearer token", AuthFailureReason.MISSING_TOKEN)
    if verifier is None or not service.strip():
        if observer:
            observer(AuthFailureReason.MISCONFIGURED)
        return AuthResult(
            None, 500, "token verifier is not configured", AuthFailureReason.MISCONFIGURED
        )
    try:
        claims = verifier.verify(service.strip(), "", raw_query, parts[1])
    except TokenError:
        if observer:
            observer(AuthFailureReason.INVALID_TOKEN)
        return AuthResult(None, 403, "invalid bearer token", AuthFailureReason.INVALID_TOKEN)
    if observer:
        observer(None)
    return AuthResult(claims, 200)
