# TiRTC 原生库

本 Git 仓库不保存 TiRTC 原生库二进制文件。请从正式分发渠道获取与语言和平台匹配的 TiRTC SDK。

## Go

Go SDK 使用以下 SDK 包：

```text
https://repo-sdk.tange-ai.com/repository/tirtc-sdks/ci/linux-x86_64-desktop/tirtc__linux-x86_64-desktop__webrtc-linux-clang__e84eff7__20260714-112702__standard/tirtc__linux-x86_64-desktop__webrtc-linux-clang__e84eff7__20260714-112702__standard.tgz
```

使用 `tirtc_clib` 构建标签构建前，先下载并安装原生库：

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

## Python

Python SDK 通过 `ctypes` 加载 TiRTC 动态库。将 `libTiRTC.so` 或
`libTiRTC.dylib` 放入 `clib/lib/`，也可以设置：

```bash
export TIRTC_PYTHON_LIBRARY=/absolute/path/to/libTiRTC.so
```

动态库及其运行时依赖必须来自同一发布版本。令牌签发和验证不加载原生库。
