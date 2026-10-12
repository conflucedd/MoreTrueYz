# 逆向报告：让 BotW 的全部 Yaz0 逐字节复现

- 数据：`botw/`（`game/` + `dlc/`，51509 个文件，其中 **38614 个 Yaz0**）
- 压缩总量 8.08 GiB，解压后 14.62 GiB
- 上游：[FastYZ](https://github.com/aboood40091/FastYZ) `18bbb52`、[TrueYZ](https://github.com/aboood40091/TrueYZ) `3e2a1b7`
- 结果：**38614 / 38614 = 100.0000%** 逐字节复现，32 线程 37 秒
- 逐文件结果：[`reports/yaz0final-report.tsv`](../reports/yaz0final-report.tsv)

## 0. 起点

只用 FastYZ 解包 + 原版 TrueYZ 重压（`cmd/yaz0verify`）：

| 状态 | 数量 |
|---|---|
| `OK`（逐字节一致） | 37074（96.01%） |
| `SIZE_MISMATCH` | 1350 |
| `BYTE_MISMATCH` | 190 |
| 解压失败 | 0 |

失败分两类，泾渭分明：

- **`.sblwp` 1123/1123、`.smubin` 412/413 全部失败** —— 地图数据；
- 其余格式里只有 **5 个孤例**（`shknm2` ×1、`shktmrb` ×2、`sbactorpack` ×2），
  每个只差 4 个字节左右。

下面分别解决。

---

## 1. 编码器 A：BotW 地图数据

### 1.1 先看诊断

用 `botw_scan`（yaz0tool）的 `subopt / skipped / tieDiffer / maxDist` 诊断：

- `subopt == 0`（1349/1351）—— 一旦选择匹配，长度必是 4096 窗口内的**最长**匹配；
- `tieDiffer == 0`（1346/1351）—— 匹配起点与 wiimm 的"等长取最远"完全一致；
- `skipped > 0`（1345/1351）—— 却大量"本可匹配却输出了字面字节"。

即：**搜索是穷举的（不弱），但惰性/跳过比谁都激进。**

### 1.2 手工反推最小样本

取最小的失败文件 `A-1.01_Clustering.sblwp`（40 字节）手工解码 token 流，发现：

- token 只消费了 23 字节负载，文件却有 24 字节负载 —— **尾部多一个未使用的 `0x00`**；
- 该文件在 `p=28` 处有一个 len=7 的匹配可用，编码器却输出字面字节，
  然后在 `p=29` 用 len=11 的匹配（`11 > 7`，符合惰性直觉）。

顺着第 2 点去全量验证第 1 点：

> 38614 个文件里恰好 **531 个**尾部多 1 个 `0x00`，且这 531 个**全部**满足 `tokenCount % 8 == 0`。

这说明原编码器的 flag 分组是**提前开组**的：当前组一写满 8 个 token 就立刻开辟下一个
flag 字节；token 数正好是 8 的倍数时，就留下一个空的 flag 字节。

### 1.3 规则搜索

写了 `cmd/yaz0rev`：参数化编码器模拟器 + 全窗口 oracle（穷举链，远/近 tie 可选），
外加 `-probe` 打印每个位置的 oracle 与原始决策。

`probe` 给出的关键对比（`A-2_Static.smubin`，raw 47012）：

```
p=1753  oracle len=7   d=1045   原: 字面          <- 下一个 len=16, 差 9 → 推迟
p=1754  oracle len=16  d=33     原: 匹配 len=16
p=1803  oracle len=6   d=1045   原: 匹配 len=6    <- 下一个 len=7, 差 1 → 不推迟
p=1804  oracle len=7   d=66
p=9464  oracle len=4   d=8      原: 字面          <- 下一个 len=6, 差 2 → 推迟
p=9465  oracle len=6   d=1356   原: 匹配 len=6    <- 推迟后立即提交, 不再第二次推迟
```

于是把"惰性"拆成两个正交的问题：**推迟的判据**和**推迟后是否继续推迟**。
在失败集上扫描：

| 规则 | `A-2_Static.smubin`（原 20449 B） |
|---|---|
| 远 tie + 标准惰性（`L2 > L1`） | 20263 ✗ |
| 远 tie + lookahead margin=0（`L2 > L1`） | 20268 ✗ |
| **远 tie + lookahead margin=1（`L2 > L1 + 1`）** | **20449 ✅** |
| 远 tie + lookahead margin=2 | 20549 ✗ |

其中 margin=1 的规则压到 1351 个最终失败文件上：**1346/1351 命中**
（就是全部 `.sblwp` / `.smubin`），同时把 531 个"尾部多一个 flag 字节"的文件一并解决。

### 1.4 编码器 A 的完整规则

在位置 `p`：

1. **匹配**：在 4096 字节窗口内穷举最长匹配（链不设上限、无 good/nice 提前退出），
   上限 273 字节；**等长时取距离最远**（窗口内最早出现的位置）。
2. **惰性（lookahead-and-commit）**：
   - `L1 = 最长(p)`，`L2 = 最长(p+1)`；
   - 若 `L2 > L1 + 1`：在 `p` 输出字面字节，**立即**在 `p+1` 输出 `L2` 的匹配，
     位置跳到 `p+1+L2`（**不会连续推迟第二次**）；
   - 否则在 `p` 输出 `L1` 的匹配；`L1 < 3` 时输出字面字节。
3. **flag 分组提前开组**（见 1.2）。
4. 文件头 0x08..0x0B 原样写回原文件的 alignment；0x0C..0x0F 为 0。

实现见 `internal/yaz0enc/encode.go` 的 `encodeLookahead` 与 `DefaultConfigA()`。

---

## 2. 编码器 B：其余所有格式

`.sstera` / `.sbfres` / `.sbactorpack` / `.shktmrb` / `.shknm2` / `.sbmaptex` /
`.sbitemico` … 共 37079 个文件属于 zlib 派生的 EAD/EPD 编码器，也就是 TrueYZ 建模的那一支。

### 2.1 先把 `MAX_DIST` 排除掉

5 个孤例的失败点长这样（`4-20.shknm2`，raw 12208）：

```
token #2874 @ dst=12011
  原文件 : 字面
  TrueYZ : 匹配 len=3 dist=3844
```

`3844` 刚好超过 `4096 - MIN_LOOKAHEAD(276) = 3820`，很容易怀疑是 TrueYZ README 里那条
`MAX_DIST` 偏差。实测**不是**：

- 把 TrueYZ 的 `MAX_DIST` 改成 3820 → 差异点反而提前到 `dst=5044`，
  因为原文件在那里用了 `dist=3860`（> 3820）；
- 也就是说原文件接受 3860，却在 12011 处"看不见" 3844 —— 不是距离阈值问题。

### 2.2 单变量扫描

直接改 TrueYZ 的常数，编译多个变体，对 5 个文件逐一测试：

| 改动 | 命中 |
|---|---|
| `MAX_CHAIN` = 64 / 128 / 256 / 512 / 1024 / 2048 | 0/5 |
| `GOOD_MATCH` = 8 / 16 / 64 / 128 | 0/5 |
| `MAX_LAZY` = 8 / 16 / 64 / 128 / 258 | 0/5 |
| `NICE_MATCH` = 128 / 258 | 0/5 |
| `MAX_DIST` = 3820 | 0/5 |
| **`MIN_LOOKAHEAD` → `MAX_MATCH + MIN_MATCH + 1`** | **5/5 ✅** |

TrueYZ 的 README 明确把 `MIN_LOOKAHEAD = MAX_MATCH + MIN_MATCH`（比 stock zlib 少 1）
列为"**刻意偏离、必须照抄**"。但对 BotW 的这 5 个文件，原编码器用的恰恰是
**stock zlib 的 `+1`**。

### 2.3 不能全局替换

直接把这一改动应用到所有文件（只用 B2）时，38614 个里只有 **31519** 个能复现。
说明 BotW 里这一支也存在两个 build：绝大多数文件用 B1 的规则，这 5 个用 B2 的规则。

因此正确用法是 **B1 → B2 → A 三级回退**。并集统计：

| | 数量 |
|---|---|
| B1 命中 | 37074 |
| B2 命中且 B1 未命中 | **5** |
| A 命中（与 B1∪B2 无交集） | 1535 |
| **并集** | **38614 / 38614** |

实现见 `internal/trueyzstock/`（`MIN_LOOKAHEAD` 改成 `+1`，导出符号改名为
`tystock_*`，以便和原版 TrueYZ 链接进同一个二进制）。

---

## 3. 复现方式

```sh
# 全量校验（B1 -> B2 -> A 三级回退）
make
bin/moretrueyz -root /path/to/botw -j 32 -report report.tsv

# 只用 FastYZ + 原版 TrueYZ 的基线（96.01%）
bin/yaz0verify -root /path/to/botw -j 32 -report trueyz-only-report.tsv

# 规则搜索 / 单文件反推
bin/yaz0rev -tie-far -lazy 3 -margin 1 -eager file.szs   # 用规则 A 编码并比对
bin/yaz0rev -probe -from 1750 -to 1780 file.szs          # 打印 oracle 与原始决策
bin/yaz0rev -list fails.txt -sweep -j 32                 # 批量扫一族配置

# token 级 diff
bin/yaz0tok diff original.szs rebuilt.szs
```

`reports/yaz0final-report.tsv` 的 `which` 列标明每个文件由哪支编码器复现
（本数据集里只有 `B1` / `B2` / `A`，没有 `FAIL`）。

---

## 4. 结论

- BotW 的 Yaz0 不是一支编码器，而是**至少三支**：
  - 地图数据（`.sblwp` / `.smubin`）用一支独立编码器：穷举最长匹配 + 远 tie +
    lookahead-and-commit + 提前开 flag 组；
  - 其余资产用 zlib 派生的 EAD/EPD 编码器，绝大多数与 TrueYZ 原版一致，
    其中 5 个文件需要 stock zlib 的 `MIN_LOOKAHEAD`。
- 三者并集覆盖全部 38614 个文件。
- TrueYZ README 中"必须照抄"的 `MIN_LOOKAHEAD` 在 BotW 上有反例；
  上游若要同时兼容这两类数据，建议把它做成编译期可选或运行时参数。

## 5. 说明

- 编码器 A 是**行为等价模型**，不保证与任天堂内部实现逐行一致；
  它的价值在于对全部 1535 个相关文件都能逐字节复现。
- 单变量扫描时 `MAX_LAZY` 的取值 0/1/4/16/32/64/258 也都试过，均不能复现那 1346 个
  地图文件——它们不是"TrueYZ 换惰性参数"，而是另一支编码器。
