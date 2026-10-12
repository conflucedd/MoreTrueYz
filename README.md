# MoreTrueYz

> 让《塞尔达传说：旷野之息》里的**全部 38614 个 Yaz0 文件**都能被逐字节复现。

[TrueYZ](https://github.com/aboood40091/TrueYZ) 能复现任天堂 EAD/EPD 的 Yaz0 编码器输出，
但用它跑一遍 BotW 的 `game/` + `dlc/`，只能复现 **37074 / 38614（96.01%）**。
本仓库补上了剩下的 1540 个：

| 编码器 | 覆盖文件数 | 说明 |
|---|---|---|
| **B1** TrueYZ 原版 | 37074 | 上游原样 |
| **B2** TrueYZ + stock `MIN_LOOKAHEAD` | **5** | 只改一个常数 |
| **A** far-tie + lookahead-commit（本仓库逆向） | **1535** | 全部 `.sblwp` / `.smubin` |
| **合计** | **38614 / 38614 = 100.0000%** | 32 线程 38 秒 |

详细推导过程见 [`docs/reverse-engineering.md`](docs/reverse-engineering.md)。

## 两个关键发现

### 1. BotW 的地图数据用了另一支编码器（编码器 A）

`.sblwp`（1123 个）和 `.smubin`（413 个）里除了 1 个之外，都不是 zlib 派生编码器压的。
它的规则是：

1. **匹配**：4096 窗口内**穷举最长匹配**（链不设上限、无 good/nice 提前退出），
   **等长时取距离最远**（窗口内最早出现的位置）；
2. **惰性**：看一眼下一位置 —— 只有 `L(p+1) > L(p) + 1` 才在 `p` 输出字面字节，
   然后**立即提交** `p+1` 的匹配，**不会连续推迟第二次**（lookahead-and-commit）；
3. **flag 分组提前开组**：当前组一写满 8 个 token 就立刻开辟下一个 flag 字节，
   因此 token 数正好是 8 的倍数时，文件尾部会多一个未被使用的 `0x00`。
   全 38614 个文件里恰好 531 个有这种尾巴，且全部满足 `tokenCount % 8 == 0`。

### 2. `MIN_LOOKAHEAD` 在 BotW 上有反例（编码器 B2）

TrueYZ 的 README 把 `MIN_LOOKAHEAD = MAX_MATCH + MIN_MATCH` 列为"刻意偏离 stock zlib、
必须照抄"的常数。但 BotW 里有 5 个文件（`shknm2` / `shktmrb` / `sbactorpack`）恰恰要用
stock zlib 的 `MAX_MATCH + MIN_MATCH + 1` 才能压回原样。

注意这个改动**不能全局替换**（全局替换时只有 31519 个文件能复现），
所以正确做法是 **B1 → B2 → A 三级回退**。

## 目录结构

```
MoreTrueYz/
├── cmd/
│   ├── moretrueyz/     # 三级回退的 100% 复现校验器
│   ├── yaz0verify/     # 基线：只用 FastYZ + 原版 TrueYZ（96.01%）
│   ├── yaz0rev/        # 规则搜索：参数化编码器模拟 + probe + 配置扫描
│   └── yaz0tok/        # token 级 diff
├── internal/
│   ├── yaz0enc/        # 编码器 A + Yaz0 解码 / token 解析
│   ├── fastyz/         # cgo 封装 FastYZ（解压）
│   ├── trueyz/         # cgo 封装 TrueYZ（B1）
│   └── trueyzstock/    # cgo 封装 TrueYZ + stock MIN_LOOKAHEAD（B2）
├── docs/
│   ├── reverse-engineering.md      # 完整逆向报告
│   └── verification-fastyz-trueyz.md  # 第一阶段的 FastYZ/TrueYZ 校验 + 与 botw_scan 的交叉比对
└── reports/
    └── yaz0final-report.tsv        # 38614 个文件的最终结果（which = B1/B2/A）
```

## 构建

需要 Go（1.18+）和一个 C 编译器（cgo 用于链接 FastYZ / TrueYZ）。

```sh
git clone <this repo> MoreTrueYz
cd MoreTrueYz
go build ./...
# 或者
make            # 输出到 bin/
```

> 模块路径目前是 `module moretrueyz`（纯本地路径）。若想用
> `go install github.com/<你>/MoreTrueYz/cmd/moretrueyz@latest`，
> 把 `go.mod` 的 module 改成你的仓库地址，并同步替换 `cmd/` 下的 import 前缀即可。

## 使用

### 全量校验（推荐）

```sh
# 直接扫目录
bin/moretrueyz -root /path/to/botw -j 32 -report report.tsv

# 或者用文件清单
bin/moretrueyz -list yaz0-files.txt -j 32 -report report.tsv
```

输出示例：

```
总文件                                    : 38614
编码器 B1 (TrueYZ 原版)                   : 37074
编码器 B2 (TrueYZ + stock MIN_LOOKAHEAD)  : 5
编码器 A  (far-tie + lookahead-commit)    : 1535
失败                                      : 0
覆盖率                                    : 100.0000%
```

报告 TSV 的 `which` 列标明每个文件由哪支编码器复现。

### 单文件实验

```sh
# 用规则 A 编码并比对（firstDiff=-1 表示逐字节一致）
bin/yaz0rev -tie-far -lazy 3 -margin 1 -eager file.szs

# 打印每个位置的 oracle 与原始文件的决策 —— 逆推规则的主要工具
bin/yaz0rev -probe -from 1750 -to 1780 file.szs

# 批量扫描一族配置
bin/yaz0rev -list fails.txt -sweep -j 32

# token 级 diff
bin/yaz0tok diff original.szs rebuilt.szs

# 基线：只用 FastYZ + 原版 TrueYZ
bin/yaz0verify -root /path/to/botw -j 32
```

## 结果数据

`reports/yaz0final-report.tsv` 是对 BotW `game/` + `dlc/`（51509 个文件，其中 38614 个 Yaz0，
压缩总量 8.08 GiB，解压后 14.62 GiB）跑出来的逐文件结果：

```
path	which	compSize	rawSize	rebuiltB1	rebuiltB2	rebuiltA
```

`which` 取值：`B1` / `B2` / `A`（本数据集里没有 `FAIL`）。

## 说明

- 本仓库的代码以 MIT 发布；`internal/fastyz`、`internal/trueyz` 是上游 MIT 项目的源码副本
  （FastYZ `18bbb52`、TrueYZ `3e2a1b7`，各自的 `LICENSE` 已随目录保留），
  `internal/trueyzstock` 是 TrueYZ 的修改版（`MIN_LOOKAHEAD + 1`，导出符号改名以便共存）。
- 逆向出的编码器 A 是**行为等价模型**，不保证与任天堂内部实现逐行一致；
  它的价值在于对全部 1535 个相关文件都能逐字节复现。
- 数据集中不含任何游戏数据，报告里的路径只是文件位置。

---

## English

TrueYZ reproduces Nintendo EAD/EPD's Yaz0 encoder byte-for-byte, but on Breath of the
Wild it only covers **37074 of 38614** Yaz0 files (96.01%). This repo closes the gap:

- **Encoder A** (reverse-engineered here) covers all 1535 `.sblwp` / `.smubin` files.
  Rules: exhaustive longest match in a 4096-byte window, farthest tie, a
  *lookahead-and-commit* lazy rule (defer only when `L(p+1) > L(p) + 1`, and commit
  immediately instead of deferring again), and eager flag-group allocation (which
  leaves a trailing `0x00` flag byte when the token count is a multiple of 8).
- **Encoder B2** is TrueYZ with `MIN_LOOKAHEAD = MAX_MATCH + MIN_MATCH + 1` (the stock
  zlib value), which fixes exactly the 5 files TrueYZ misses. It must **not** be applied
  globally — hence the B1 → B2 → A fallback.

Result: **38614 / 38614 = 100.0000%** byte-exact, verified over the full corpus.

See [`docs/reverse-engineering.md`](docs/reverse-engineering.md) for the full write-up.

## License

MIT. Vendored FastYZ / TrueYZ sources are MIT, © their respective authors.
