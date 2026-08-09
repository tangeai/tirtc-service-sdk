import base64

import pytest
from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey

from tirtcxauth import (
    AuthFailureReason,
    Ed25519TokenIssuer,
    Ed25519TokenVerifier,
    TokenError,
    canonical_query,
    verify_bearer,
)


def keys():
    private = Ed25519PrivateKey.from_private_bytes(bytes(range(32)))
    public = private.public_key()
    private_raw = base64.urlsafe_b64encode(
        private.private_bytes(
            serialization.Encoding.Raw,
            serialization.PrivateFormat.Raw,
            serialization.NoEncryption(),
        )
    ).rstrip(b"=").decode()
    public_raw = base64.urlsafe_b64encode(
        public.public_bytes(
            serialization.Encoding.Raw,
            serialization.PublicFormat.Raw,
        )
    ).rstrip(b"=").decode()
    return private_raw, public_raw


def test_issue_verify_and_query_canonicalization():
    private, public = keys()
    issuer = Ed25519TokenIssuer({"ak1": private})
    verifier = Ed25519TokenVerifier({"ak1": public})
    token = issuer.issue("echo", "ak1", "device-1", "b=2&a=1&a=0", 60)
    claims = verifier.verify("echo", "device-1", "a=1&a=0&b=2", token)
    assert claims.subject == "device-1"
    assert claims.issuer == "ak1"
    assert canonical_query("?b=2&a=1&a=0") == "a=1&a=0&b=2"


@pytest.mark.parametrize(
    ("raw", "expected"),
    [
        ("empty&x=", "empty=&x="),
        ("space=a+b&encoded=a%20b", "encoded=a+b&space=a+b"),
        ("name=%E6%B5%8B%E8%AF%95", "name=%E6%B5%8B%E8%AF%95"),
        ("raw=%FF&path=%2Ffoo%2Fbar", "path=%2Ffoo%2Fbar&raw=%FF"),
        ("a=1&a=0", "a=1&a=0"),
    ],
)
def test_canonical_query_matches_go_net_url(raw, expected):
    assert canonical_query(raw) == expected


def test_scope_and_subject_are_enforced():
    private, public = keys()
    token = Ed25519TokenIssuer({"ak1": private}).issue(
        "echo", "ak1", "device-1", "_tg_mode=echo", 60
    )
    verifier = Ed25519TokenVerifier({"ak1": public})
    with pytest.raises(TokenError, match="subject"):
        verifier.verify("echo", "device-2", "_tg_mode=echo", token)
    with pytest.raises(TokenError, match="scope"):
        verifier.verify("echo", "device-1", "", token)
    with pytest.raises(TokenError, match="query"):
        canonical_query("bad=%zz")


def test_bearer_result_uses_go_status_semantics():
    private, public = keys()
    issuer = Ed25519TokenIssuer({"ak1": private})
    verifier = Ed25519TokenVerifier({"ak1": public})
    token = issuer.issue("echo", "ak1", "device-1", "", 60)
    assert verify_bearer("", "echo", "", verifier).status == 401
    assert verify_bearer("Bearer bad", "echo", "", verifier).status == 403
    result = verify_bearer(f"Bearer {token}", "echo", "", verifier)
    assert result.status == 200
    assert result.claims.subject == "device-1"
    assert result.reason is None
    assert (
        verify_bearer("Bearer x", "", "", verifier).reason
        == AuthFailureReason.MISCONFIGURED
    )
