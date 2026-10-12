# reports/

## `yaz0final-report.tsv`

对 BotW `game/` + `dlc/` 的全部 38614 个 Yaz0 跑
`bin/moretrueyz -root /path/to/botw -j 32` 的结果，路径已改成相对 `botw/` 的路径。

列：

| 列 | 含义 |
|---|---|
| `path` | 相对 `botw/` 的文件路径 |
| `which` | 由哪支编码器复现：`B1` / `B2` / `A`（无 `FAIL`） |
| `compSize` | 原文件字节数 |
| `rawSize` | 解压后字节数 |
| `rebuiltB1` | TrueYZ 原版的输出长度；为 0 表示未尝试（已被更前面的编码器命中） |
| `rebuiltB2` | TrueYZ + stock `MIN_LOOKAHEAD` 的输出长度；0 表示未尝试 |
| `rebuiltA` | 编码器 A 的输出长度；0 表示未尝试 |

统计：

| `which` | 数量 |
|---|---|
| `B1` | 37074 |
| `B2` | 5 |
| `A` | 1535 |
| 合计 | 38614 |

`A` 的 1535 个全部是 `.sblwp`（1123）与 `.smubin`（412）。
