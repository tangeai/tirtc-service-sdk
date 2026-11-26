# TiRTC 原生库

本 Git 仓库不保存 TiRTC 原生库二进制文件。请从正式分发渠道获取与语言和平台匹配的 TiRTC SDK。

Go SDK 使用 TiRTC C SDK **2.6.3** 的 Linux x86_64 desktop 发行包。请从正式分发渠道取得
该版本对应的包；不要混用旧的 CI 构建包或其他版本的头文件、静态库和运行时库。

使用 `tirtc_clib` 构建标签构建前，先下载并安装原生库：

```bash
TIRTC_SDK_URL='<TiRTC C SDK 2.6.3 Linux x86_64 desktop 发行包 URL>'
tmp_dir="$(mktemp -d)"
curl -fsSL --retry 3 "$TIRTC_SDK_URL" | tar -xz -C "$tmp_dir"
sdk_dir="$(find "$tmp_dir" -mindepth 1 -maxdepth 1 -type d | head -n 1)"
cp "$sdk_dir/lib/libTiRTC.a" clib/lib/
cp "$sdk_dir/lib/libwebrtc.so" clib/lib/
```

构建时会链接 `libTiRTC.a`。部署时应将 `libwebrtc.so` 与服务程序放在同一目录，或通过
`LD_LIBRARY_PATH` 加载 `clib/lib`。`clib/lib` 下的原生库文件不会提交到 Git。
