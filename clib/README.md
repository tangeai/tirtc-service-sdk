# TiRTC 原生库

本 Git 仓库不保存 TiRTC 原生库二进制文件。Go SDK 使用以下 SDK 包：

```text
https://repo-sdk.tange-ai.com/repository/tirtc-sdks/ci/linux-x86_64-desktop/tirtc__linux-x86_64-desktop__webrtc-linux-clang__e84eff7__20260714-112702__standard/tirtc__linux-x86_64-desktop__webrtc-linux-clang__e84eff7__20260714-112702__standard.tgz
```

使用 `tirtc_clib` Build Tag 构建前，先下载并安装原生库：

```bash
TIRTC_SDK_URL='https://repo-sdk.tange-ai.com/repository/tirtc-sdks/ci/linux-x86_64-desktop/tirtc__linux-x86_64-desktop__webrtc-linux-clang__e84eff7__20260714-112702__standard/tirtc__linux-x86_64-desktop__webrtc-linux-clang__e84eff7__20260714-112702__standard.tgz'
tmp_dir="$(mktemp -d)"
curl -fsSL --retry 3 "$TIRTC_SDK_URL" | tar -xz -C "$tmp_dir"
sdk_dir="$(find "$tmp_dir" -mindepth 1 -maxdepth 1 -type d | head -n 1)"
cp "$sdk_dir/lib/libTiRTC.a" clib/lib/
cp "$sdk_dir/lib/libwebrtc.so" clib/lib/
```

构建时会链接 `libTiRTC.a`。部署时应将 `libwebrtc.so` 与服务程序放在同一目录，或通过
`LD_LIBRARY_PATH` 加载 `clib/lib`。`clib/lib` 下的原生库文件不会提交到 Git。
