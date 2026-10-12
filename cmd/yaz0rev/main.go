// Command yaz0rev 是逆向编码器 A 时用的参数化编码器模拟器:
// 单文件比对、批量比对、配置扫描、以及打印逐位置的决策过程(probe)。
//
// 例:
//
//	yaz0rev -tie-far -lazy 3 -margin 1 -eager x.szs   # 用规则 A 编码并比对
//	yaz0rev -probe -from 1750 -to 1780 x.szs          # 打印 oracle 与原始决策
//	yaz0rev -list fails.txt -sweep -j 32              # 扫一批候选配置
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"os"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"

	"moretrueyz/internal/yaz0enc"
)

var (
	tieFar  = flag.Bool("tie-far", true, "等长匹配取最远")
	lazy    = flag.Int("lazy", 3, "0=贪心 1=lazy> 2=lazy>= 3=lookahead-commit")
	margin  = flag.Int("margin", 1, "lazy 的余量: 仅当下一匹配比当前长超过该值才推迟")
	maxLazy = flag.Int("maxlazy", 0, ">0 时, 上一匹配长度达到该值就不再搜索")
	maxDist = flag.Int("maxdist", 0, "回溯距离上限, 0=4096")
	eager   = flag.Bool("eager", true, "flag 组满后立即开新组")
	list    = flag.String("list", "", "批量模式: 每行一个文件路径")
	jobs    = flag.Int("j", runtime.NumCPU(), "并发")
	sweep   = flag.Bool("sweep", false, "批量模式: 扫一批配置")
	out     = flag.String("o", "", "单文件模式: 把重压结果写到该文件")
	doProbe = flag.Bool("probe", false, "单文件模式: 打印每个位置的决策与 oracle")
	pFrom   = flag.Int("from", 0, "probe 起始位置")
	pTo     = flag.Int("to", 60, "probe 结束位置")
)

func lazyName(l int) string {
	switch l {
	case 0:
		return "greedy"
	case 1:
		return "lazy>"
	case 2:
		return "lazy>="
	case 3:
		return "lookahead"
	}
	return "?"
}

func cfgName(c yaz0enc.Config) string {
	tie := "near"
	if c.TieFar {
		tie = "far"
	}
	ml := "inf"
	if c.MaxLazy > 0 {
		ml = fmt.Sprint(c.MaxLazy)
	}
	md := "4096"
	if c.MaxDist > 0 && c.MaxDist != yaz0enc.MaxDist {
		md = fmt.Sprint(c.MaxDist)
	}
	e := ""
	if c.Eager {
		e = "+eager"
	}
	return fmt.Sprintf("%s/%s/margin%d/ml%s/dist%s%s", tie, lazyName(c.Lazy), c.Margin, ml, md, e)
}

func readFile(p string) []byte {
	b, err := os.ReadFile(p)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return b
}

func compare(orig, enc []byte) (bool, int, int) {
	if bytes.Equal(orig, enc) {
		return true, -1, 0
	}
	n := len(orig)
	if len(enc) < n {
		n = len(enc)
	}
	first, diffs := -1, 0
	for i := 0; i < n; i++ {
		if orig[i] != enc[i] {
			diffs++
			if first < 0 {
				first = i
			}
		}
	}
	if d := len(orig) - len(enc); d > 0 {
		diffs += d
	} else {
		diffs -= d
	}
	if first < 0 {
		first = n
	}
	return false, first, diffs
}

func buildCfg() yaz0enc.Config {
	return yaz0enc.Config{
		TieFar:  *tieFar,
		Lazy:    *lazy,
		Margin:  *margin,
		MaxLazy: *maxLazy,
		MaxDist: *maxDist,
		Eager:   *eager,
		Magic:   [4]byte{'Y', 'a', 'z', '0'},
	}
}

func alignOf(b []byte) uint32 {
	if len(b) < 12 {
		return 0
	}
	return uint32(b[8])<<24 | uint32(b[9])<<16 | uint32(b[10])<<8 | uint32(b[11])
}

func main() {
	flag.Parse()

	if *list == "" {
		args := flag.Args()
		if len(args) < 1 {
			fmt.Fprintln(os.Stderr, "用法: yaz0rev [-sweep] -list <清单> | yaz0rev <file.yaz0>")
			os.Exit(2)
		}
		cfg := buildCfg()
		for _, p := range args {
			if *doProbe {
				probe(p, *pFrom, *pTo)
				continue
			}
			orig := readFile(p)
			raw := yaz0enc.Decode(orig)
			if raw == nil {
				fmt.Printf("DECODE_FAIL %s\n", p)
				continue
			}
			cfg.Align = alignOf(orig)
			enc := yaz0enc.Encode(raw, cfg)
			ok, first, diffs := compare(orig, enc)
			st := "DIFF"
			if ok {
				st = "OK"
			}
			fmt.Printf("%s %s  原=%d 重压=%d firstDiff=%d diffs=%d\n", st, p, len(orig), len(enc), first, diffs)
			if *out != "" {
				os.WriteFile(*out, enc, 0o644)
			}
		}
		return
	}

	paths := readList(*list)
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "清单为空")
		os.Exit(1)
	}
	if *sweep {
		sweepConfigs(paths)
		return
	}
	cfg := buildCfg()
	ok, fails := runBatch(paths, cfg)
	fmt.Printf("配置 %s : %d/%d 逐字节复现\n", cfgName(cfg), ok, len(paths))
	for i, f := range fails {
		if i >= 10 {
			fmt.Printf("  ... 其余 %d 个失败\n", len(fails)-10)
			break
		}
		fmt.Printf("  %s (原=%d 重压=%d firstDiff=%d)\n", f.path, f.comp, f.rebuilt, f.first)
	}
}

type failInfo struct {
	path          string
	comp, rebuilt int
	first, diffs  int
}

func runBatch(paths []string, cfg yaz0enc.Config) (int, []failInfo) {
	ch := make(chan string, 256)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var okCount int64
	var fails []failInfo
	for i := 0; i < *jobs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := cfg
			for p := range ch {
				orig, err := os.ReadFile(p)
				if err != nil {
					continue
				}
				raw := yaz0enc.Decode(orig)
				if raw == nil {
					mu.Lock()
					fails = append(fails, failInfo{path: p})
					mu.Unlock()
					continue
				}
				c.Align = alignOf(orig)
				enc := yaz0enc.Encode(raw, c)
				ok, first, diffs := compare(orig, enc)
				if ok {
					atomic.AddInt64(&okCount, 1)
					continue
				}
				mu.Lock()
				fails = append(fails, failInfo{path: p, comp: len(orig), rebuilt: len(enc), first: first, diffs: diffs})
				mu.Unlock()
			}
		}()
	}
	for _, p := range paths {
		ch <- p
	}
	close(ch)
	wg.Wait()
	sort.Slice(fails, func(i, j int) bool { return fails[i].path < fails[j].path })
	return int(okCount), fails
}

func sweepConfigs(paths []string) {
	var cfgs []yaz0enc.Config
	for _, tie := range []bool{true, false} {
		for _, lz := range []int{0, 1, 2, 3} {
			for _, mg := range []int{0, 1, 2} {
				if (lz == 1 || lz == 3) && mg > 2 {
					continue
				}
				for _, eg := range []bool{true, false} {
					cfgs = append(cfgs, yaz0enc.Config{
						TieFar: tie, Lazy: lz, Margin: mg, Eager: eg,
						Magic: [4]byte{'Y', 'a', 'z', '0'},
					})
				}
			}
		}
	}
	type res struct {
		name string
		ok   int
	}
	var results []res
	for _, c := range cfgs {
		ok, _ := runBatch(paths, c)
		results = append(results, res{cfgName(c), ok})
		fmt.Printf("%-40s %6d / %d\n", cfgName(c), ok, len(paths))
	}
	sort.Slice(results, func(i, j int) bool { return results[i].ok > results[j].ok })
	fmt.Println("\n排序:")
	for _, r := range results {
		fmt.Printf("  %-40s %6d / %d\n", r.name, r.ok, len(paths))
	}
}

func readList(p string) []string {
	f, err := os.Open(p)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		if l := sc.Text(); l != "" {
			out = append(out, l)
		}
	}
	return out
}
