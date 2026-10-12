# internal/trueyzstock

TrueYZ 的**修改版**，MIT（沿用上游许可证，见 `LICENSE`），基线上游 commit `3e2a1b7`。

改动只有一处，但很关键：

```diff
-#define MIN_LOOKAHEAD (MAX_MATCH + MIN_MATCH)      /* TrueYZ / 上游 */
+#define MIN_LOOKAHEAD (MAX_MATCH + MIN_MATCH + 1)  /* stock zlib */
```

另外把导出符号从 `trueyz_*` 改名为 `tystock_*`，以便和原版 TrueYZ 链接进同一个二进制。

## 为什么

上游 README 把 `MIN_LOOKAHEAD = MAX_MATCH + MIN_MATCH` 列为"刻意偏离 stock zlib、
必须照抄"的常数。但 BotW 里有 5 个文件（`shknm2` ×1、`shktmrb` ×2、`sbactorpack` ×2）
恰恰要用 stock 值才能压回原样。

**注意：不能全局替换。** 只用这个变体跑全部 38614 个文件，只有 31519 个能复现 ——
说明 BotW 里这一支编码器存在两个 build。因此 `cmd/moretrueyz` 采用
**B1（原版）→ B2（本变体）→ A（编码器 A）** 的三级回退。

改动方式（可复现）：

```sh
sed -e 's/^#define MIN_LOOKAHEAD (MAX_MATCH + MIN_MATCH)/#define MIN_LOOKAHEAD (MAX_MATCH + MIN_MATCH + 1)/' \
    -e 's/trueyz_compress_align_scratch/tystock_compress_align_scratch/g' \
    -e 's/trueyz_compress_scratch/tystock_compress_scratch/g' \
    trueyz.c > trueyzstock/trueyz.c
sed -e 's/trueyz_compress_align_scratch/tystock_compress_align_scratch/g' \
    -e 's/trueyz_compress_scratch/tystock_compress_scratch/g' \
    trueyz.h > trueyzstock/trueyz.h
```

详见 [`docs/reverse-engineering.md`](../../docs/reverse-engineering.md) 第 2 节。
