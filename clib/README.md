# TiRTC native dependency

Native TiRTC libraries are intentionally not stored in this Git repository.
The Go wrapper requires the following SDK package:

```text
https://repo-sdk.tange-ai.com/repository/tirtc-sdks/ci/linux-x86_64-desktop/tirtc__linux-x86_64-desktop__webrtc-linux-clang__e84eff7__20260714-112702__standard/tirtc__linux-x86_64-desktop__webrtc-linux-clang__e84eff7__20260714-112702__standard.tgz
```

Download and install its libraries locally before building with the
`tirtc_clib` tag:

```bash
TIRTC_SDK_URL='https://repo-sdk.tange-ai.com/repository/tirtc-sdks/ci/linux-x86_64-desktop/tirtc__linux-x86_64-desktop__webrtc-linux-clang__e84eff7__20260714-112702__standard/tirtc__linux-x86_64-desktop__webrtc-linux-clang__e84eff7__20260714-112702__standard.tgz'
tmp_dir="$(mktemp -d)"
curl -fsSL --retry 3 "$TIRTC_SDK_URL" | tar -xz -C "$tmp_dir"
sdk_dir="$(find "$tmp_dir" -mindepth 1 -maxdepth 1 -type d | head -n 1)"
cp "$sdk_dir/lib/libTiRTC.a" clib/lib/
cp "$sdk_dir/lib/libwebrtc.so" clib/lib/
```

`libTiRTC.a` is linked at build time. Deploy `libwebrtc.so` next to the
service executable or make `clib/lib` available through `LD_LIBRARY_PATH`.
Files under `clib/lib` are ignored by Git.
