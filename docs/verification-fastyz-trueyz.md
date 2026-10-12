# 第一阶段：FastYZ 解包 + TrueYZ 复现校验（基线 96.01%）

本文记录在引入编码器 A/B2 之前，只用 **FastYZ 解包 + 原版 TrueYZ 重压** 的结果，
也就是报告里的 "37074 / 38614" 这个中间结论，以及和 `botw_scan`（yaz0tool）报告的交叉比对。

复现命令：

```sh
bin/yaz0verify -root /path/to/botw -j 32 -report trueyz-only-report.tsv
```

## 1. 方法

1. 并发遍历目录，只读每个文件前 16 字节，按 `Yaz0` 魔数挑出候选；
2. 每个候选整块读入内存，用 FastYZ 的 `yaz0_decompress()` 解包；
3. 立刻用 TrueYZ 的 `trueyz_compress_align_scratch()` 重压，
   并把原头部 0x08..0x0B 的 alignment 提示原样传回；
4. 与原文件逐字节比较。

全程在内存中完成，解压结果不落盘；每个 goroutine 一块私有 scratch
（`FASTYZ_HTAB_SCRATCH` / `TRUEYZ_STATE_SCRATCH`）。

## 2. 总体结果

| 项目 | 数值 |
|---|---|
| 扫描文件总数 | 51509 |
| Yaz0 文件 | 38614 |
| 压缩总量 | 8.08 GiB |
| 解压后总量 | 14.62 GiB |
| **FastYZ 解压失败** | **0** |
| **TrueYZ 逐字节复现** | **37074 / 38614 = 96.01%** |
| 复现成功占压缩字节 | 99.58% |

失败明细：

| 状态 | 数量 | 说明 |
|---|---|---|
| `OK` | 37074 | 逐字节完全一致 |
| `SIZE_MISMATCH` | 1350 | 重压长度不同（1344 个 TrueYZ 更小，6 个恰好大 1 字节） |
| `BYTE_MISMATCH` | 190 | 长度相同、字节不同 |

失败的分布：

| 扩展名 | 失败 / 总数 |
|---|---|
| `.sblwp` | 1123 / 1123 |
| `.smubin` | 412 / 413 |
| `.shktmrb` | 2 / 2560 |
| `.sbactorpack` | 2 / 7203 |
| `.shknm2` | 1 / 2560 |

其余扩展名（`.sstera` 9174、`.sbfres` 8067、`.sbmaptex` 2344、`.sbeventpack` 997 …）全部复现成功。

## 3. 一个容易踩的坑：header 里的 alignment

38614 个文件里有 **2244 个**头部 0x08..0x0B 不是 0（例如 `00 00 00 04`）。
TrueYZ 的 CLI 默认 `-a 0`，直接 `trueyz raw.bin` 会让这 2244 个文件全部对不上；
必须把原文件头那 4 字节按大端取出来，非 0 时用 `-a N` 传回去（CLI 拒绝 `-a 0`）。

本工具是从原始头部读取并原样回传的，所以这 2244 个全部复现成功。
头部 0x0C..0x0F 全部为 0，解压结果也没有再以 `Yaz0` 开头的嵌套文件。

## 4. 与 `botw_scan`（yaz0tool）报告的交叉比对

数据源：

- 本工具的 TrueYZ 结果：`trueyz-only-report.tsv`
- yaz0tool 的报告：`yaz0-match.tsv`（候选 = wiimm-C9 / wiimm-C10(lazy) / tie-near / tie-near+lazy）
- 用上传版 yaz0tool 对失败集重跑 `-wide`（候选扩到 12 个）：`bothfail-wide.tsv`

混淆矩阵：

| | yaz0tool 复现成功 | yaz0tool 未复现 | 合计 |
|---|---|---|---|
| **TrueYZ 成功** | 5375 | 31699 | 37074 |
| **TrueYZ 失败** | **189** | **1351** | 1540 |
| 合计 | 5564 | 33050 | 38614 |

两个工具的能力高度互补。

### TrueYZ 失败但 yaz0tool 能复现的 189 个

| 扩展名 | 数量 |
|---|---|
| `.smubin` | 155 |
| `.sblwp` | 34 |

命中的候选只有 `wiimm-C10(lazy)` 164 个、`wiimm-C9` 25 个。
这 189 个全是小文件（压缩后中位 127 B、最大 4392 B）—— 小文件里各家编码器
选择本来就少，wiimm 的"等长取最远"恰好和原文件重合。

### 两边都复现不了的 1351 个

把 yaz0tool 的 12 个候选全部压上去：**0 / 1351 复现成功**。
诊断画像：

- `subopt == 0`：1349 / 1351 —— 搜索不弱；
- `skipped > 0`：1345 / 1351 —— 但大量主动放弃匹配；
- 其中 1346 个是 `.sblwp` / `.smubin`。

这份诊断正是后来逆推出编码器 A 的起点（见
[`reverse-engineering.md`](reverse-engineering.md)）。

## 5. 独立交叉验证（上游 CLI）

用单独编出来的 `fastyz` / `trueyz` 命令行工具，对 268 个分层抽样文件做往返：

- 正确传入 alignment：238 OK、30 DIFF，30 个 DIFF **全部**是 `.smubin` / `.sblwp`；
- 忘记 `-a`：77 个原本可复现的文件被误判为 DIFF —— 佐证第 3 节。

结论与 Go 版程序一致。

## 6. 结论

- **FastYZ 可以可靠解包 botw/ 中全部 38614 个 Yaz0，零失败。**
- **TrueYZ 对 BotW 并非总是逐字节复现**：文件数 96.01%、压缩字节数 99.58%。
- 剩余部分由本仓库的编码器 A 与 B2 补齐，达到 100%
  （见 [`reverse-engineering.md`](reverse-engineering.md)）。
