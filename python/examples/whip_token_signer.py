#!/usr/bin/env python3
import argparse
import json
import os

from tirtcxauth import Ed25519TokenIssuer


def main() -> None:
    parser = argparse.ArgumentParser(description="生成 WHIP peer_id 和连接 Token")
    parser.add_argument("--service", required=True)
    parser.add_argument("--access-key", required=True)
    parser.add_argument("--device-id", required=True)
    parser.add_argument("--query", default="_tg_mode=echo")
    parser.add_argument("--ttl-seconds", type=int, default=86400)
    parser.add_argument(
        "--private-key-env",
        default="TIRTC_PRIVATE_KEY",
        help="保存 Ed25519 私钥的环境变量名",
    )
    args = parser.parse_args()
    private_key = os.getenv(args.private_key_env)
    if not private_key:
        parser.error(f"环境变量 {args.private_key_env} 未设置")
    token = Ed25519TokenIssuer({args.access_key: private_key}).issue(
        args.service,
        args.access_key,
        args.device_id,
        args.query,
        args.ttl_seconds,
    )
    peer_id = f"whips://{args.service}"
    if args.query:
        peer_id += f"?{args.query.lstrip('?')}"
    print(json.dumps({"peer_id": peer_id, "token": token}, ensure_ascii=False))


if __name__ == "__main__":
    main()
