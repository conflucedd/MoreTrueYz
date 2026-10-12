# internal/fastyz

上游 [FastYZ](https://github.com/aboood40091/FastYZ) 的源码副本，MIT，commit `18bbb52`。

- `fastyz.c` / `fastyz.h`：原样复制，未做修改
- `LICENSE`：上游许可证

`fastyz.go` 用 cgo 封装了其中的 `yaz0_decompress()`，编译宏
`-DFASTYZ_HTAB=FASTYZ_HTAB_SCRATCH` 只是让压缩 API 变成可重入的（解压本来线程安全）。

要更新上游代码时，直接覆盖 `fastyz.c` / `fastyz.h` 即可。
