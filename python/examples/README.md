# Python 示例

在 `python/` 目录安装 SDK：

```bash
python3 -m pip install -e .
```

## whip_token_signer.py

生成用于 Echo 联调的 `peer_id` 和长期委托令牌：

```bash
export TIRTC_PRIVATE_KEY='<Ed25519 私钥>'

python3 examples/whip_token_signer.py \
  --service quick-start \
  --access-key your-access-key \
  --device-id your-device-id \
  --query _tg_mode=echo
```

私钥只应保存在业务服务端或本地联调环境，不得写入设备、日志或代码仓库。

## quick_start.py

先设置 TiRTC 动态库路径，再启动服务：

```bash
export TIRTC_PYTHON_LIBRARY=/absolute/path/to/libTiRTC.so

python3 examples/quick_start.py \
  --listen :8080 \
  --service quick-start \
  --access-key your-access-key \
  --public-key "$TIRTC_PUBLIC_KEY" \
  --candidate 203.0.113.10
```

`--candidate` 仅在服务需要公布不同于本机网卡的公网地址时设置。

示例提供：

- `POST /whip`：普通业务会话和平台选择的 Echo 会话；
- `DELETE /whip/resource/{session_id}`：删除普通业务会话；
- `DELETE /whip/echo/resource/{session_id}`：删除 Echo 会话。

示例已限制 SDP 请求大小并实现优雅退出。生产服务还应按部署环境增加并发和请求
速率限制、访问日志脱敏及 HTTP/HTTPS 入口策略。
