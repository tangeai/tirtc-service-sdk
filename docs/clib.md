# clib

`clib/` is the shared TiRTC C API layer used by language SDKs.

Language SDKs should depend on the public `tiRTC.h` API only. Do not expose or
require any lower-level C headers from this repository.

Current headers:

- `clib/include/tirtc/tiRTC.h`
- `clib/include/tirtc/basedef.h`

The Go SDK's `pkg/tirtcclib` package is a direct wrapper for this layer. It
keeps exported Go identifiers aligned with the C API names, such as
`TiRtcWhipAccept` and `TiRtcWhipConnect`.

## Native Libraries

Native library binaries are not stored in this Git repository. Download the
required desktop SDK from:

```text
https://repo-sdk.tange-ai.com/repository/tirtc-sdks/ci/linux-x86_64-desktop/tirtc__linux-x86_64-desktop__webrtc-linux-clang__e84eff7__20260714-112702__standard/tirtc__linux-x86_64-desktop__webrtc-linux-clang__e84eff7__20260714-112702__standard.tgz
```

See `clib/README.md` for installation commands. The Go SDK expects:

- `libTiRTC.a`
- `libwebrtc.so` (runtime dependency of the desktop TiRTC SDK)

When using the desktop TiRTC SDK, deploy `libwebrtc.so` next to the service
binary or add its directory to `LD_LIBRARY_PATH`. Static Linux builds also
link `libdl`, which the Go wrapper configures automatically.

Build Go code against the real clib wrapper with:

```bash
cd go
go build -tags tirtc_clib ./...
```

Without the `tirtc_clib` build tag, the Go wrapper compiles as stubs so pure Go
packages can be tested without native libraries.
