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

The Go SDK links against the native TiRTC library under `clib/lib`:

- `libTiRTC.so` or `libTiRTC.a`

Build Go code against the real clib wrapper with:

```bash
cd go
go build -tags tirtc_clib ./...
```

Without the `tirtc_clib` build tag, the Go wrapper compiles as stubs so pure Go
packages can be tested without native libraries.
