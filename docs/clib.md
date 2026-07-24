# clib

`clib/` 是各语言 SDK 共用的 TiRTC C API 层。

各语言 SDK 只应依赖公开的 `tiRTC.h` API，不应暴露或要求使用本仓库中更底层的 C 头文件。

当前头文件：

- `clib/include/tirtc/tiRTC.h`
- `clib/include/tirtc/basedef.h`

Go SDK 的 `internal/tirtcclib` 包直接封装这一层。它是 `pkg/tirtcx` 的内部实现，应用无法直接导入。
Python SDK 的 `tirtcx` 包通过 `ctypes` 加载 TiRTC 动态库，不向应用暴露 C 结构体。

## 原生库

本 Git 仓库不保存原生库二进制文件。请从以下地址下载 SDK：

```text
https://repo-sdk.tange-ai.com/repository/tirtc-sdks/ci/linux-x86_64-desktop/tirtc__linux-x86_64-desktop__webrtc-linux-clang__e84eff7__20260714-112702__standard/tirtc__linux-x86_64-desktop__webrtc-linux-clang__e84eff7__20260714-112702__standard.tgz
```

安装命令见 `clib/README.md`。Go SDK 需要：

- `libTiRTC.a`
- `libwebrtc.so`（TiRTC SDK 的运行时依赖）

应将 `libwebrtc.so` 与服务程序部署在同一目录，或将其目录加入 `LD_LIBRARY_PATH`。Linux 静态构建还会链接 `libdl`，Go 封装会自动配置该依赖。

使用真实 clib 封装构建 Go 代码：

```bash
cd go
go build -tags tirtc_clib ./...
```

不指定 `tirtc_clib` Build Tag 时，Go 封装会编译为 Stub，使纯 Go 包可以在没有原生库的环境中测试。

Python SDK 将 `libTiRTC.so` 或 `libTiRTC.dylib` 放入 `clib/lib/`，也可以通过
`TIRTC_PYTHON_LIBRARY` 指定绝对路径。Token 和 HTTP 认证测试不依赖原生库。
