# internal/trueyz

上游 [TrueYZ](https://github.com/aboood40091/TrueYZ) 的源码副本，MIT，commit `3e2a1b7`。

- `trueyz.c` / `trueyz.h`：原样复制，未做修改
- `LICENSE`：上游许可证

`trueyz.go` 用 cgo 封装了 `trueyz_compress_align_scratch()`（编译宏
`-DTRUEYZ_STATE=TRUEYZ_STATE_SCRATCH`，可重入，每 goroutine 一块 scratch）。

在 `cmd/moretrueyz` 里这被称作**编码器 B1**。
