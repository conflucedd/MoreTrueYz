// Command moretrueyz 对 BotW(或任意目录)里的全部 Yaz0 做逐字节复现校验。
//
// 三级回退:
//
//	B1 = 上游 TrueYZ 原版
//	B2 = TrueYZ + stock MIN_LOOKAHEAD(补掉 B1 漏掉的极少数文件)
//	A  = 本仓库逆向出的 far-tie + lookahead-and-commit 编码器(.sblwp/.smubin)
//
// 用法:
//
//	moretrueyz -root /path/to/botw -j 32 -report report.tsv
//	moretrueyz -list yaz0-files.txt -j 32
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"moretrueyz/internal/fastyz"
	"moretrueyz/internal/trueyz"
	"moretrueyz/internal/trueyzstock"
	"moretrueyz/internal/yaz0enc"
)

type result struct {
	path  string
	which string // B1 / B2 / A / FAIL / ...
	comp  int
	raw   int
	reB1  int
	reB2  int
	reA   int
	note  string
}

func alignOf(data []byte) uint32 {
	if len(data) < 12 {
		return 0
	}
	return uint32(data[8])<<24 | uint32(data[9])<<16 | uint32(data[10])<<8 | uint32(data[11])
}

func checkOne(path string, cfgA yaz0enc.Config, scB1, scB2 unsafe.Pointer) result {
	res := result{path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		res.which = "READ_FAIL"
		res.note = err.Error()
		return res
	}
	res.comp = len(data)
	if !fastyz.IsYaz0(data) {
		res.which = "NOT_YAZ0"
		return res
	}
	rawSize := int(fastyz.RawSize(data))
	res.raw = rawSize

	raw := make([]byte, rawSize)
	if rawSize > 0 {
		if n := fastyz.Decompress(data, raw, rawSize); n != rawSize {
			res.which = "DECOMPRESS_FAIL"
			res.note = fmt.Sprintf("FastYZ 返回 %d, 期望 %d", n, rawSize)
			return res
		}
	}
	// C API 不接受空指针: 空输入用一个 1 字节哑元, 长度仍传 0。
	in := raw
	if len(in) == 0 {
		in = []byte{0}
	}
	align := int(alignOf(data))
	bound := rawSize + yaz0enc.HeaderSize + rawSize/8 + 1
	out := make([]byte, bound)
	eq := func(m int) bool { return m > 0 && m == len(data) && bytes.Equal(out[:m], data) }

	res.reB1 = trueyz.CompressAlign(in, rawSize, out, align, scB1)
	if eq(res.reB1) {
		res.which = "B1"
		return res
	}
	res.reB2 = trueyzstock.CompressAlign(in, rawSize, out, align, scB2)
	if eq(res.reB2) {
		res.which = "B2"
		return res
	}
	cfgA.Align = uint32(align)
	encA := yaz0enc.Encode(raw, cfgA)
	res.reA = len(encA)
	if len(encA) == len(data) && bytes.Equal(encA, data) {
		res.which = "A"
		return res
	}
	res.which = "FAIL"
	res.note = fmt.Sprintf("原 %d 字节; B1=%d B2=%d A=%d", len(data), res.reB1, res.reB2, res.reA)
	return res
}

func main() {
	root := flag.String("root", "", "要扫描的目录(会挑出所有 Yaz0)")
	list := flag.String("list", "", "文件清单, 每行一个路径(与 -root 二选一)")
	report := flag.String("report", "", "把逐文件结果写成 TSV")
	failOut := flag.String("failures", "failures.txt", "仍失败的文件清单(为空则不写)")
	jobs := flag.Int("j", runtime.NumCPU(), "并发数")
	flag.Parse()

	if *root == "" && *list == "" {
		fmt.Fprintln(os.Stderr, "需要 -root 或 -list 之一")
		os.Exit(2)
	}

	var paths []string
	var scanned, yaz0 int64
	if *list != "" {
		paths = readList(*list)
	} else {
		paths = scanDir(*root, *jobs, &scanned, &yaz0)
	}
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "没有找到任何 Yaz0 文件")
		os.Exit(1)
	}
	if *root != "" && *list == "" {
		fmt.Printf("扫描目录 : %s\n文件总数 : %d\nYaz0 文件: %d\n", *root, scanned, yaz0)
	}
	fmt.Printf("待校验   : %d 个文件, 并发 %d\n", len(paths), *jobs)

	cfgA := yaz0enc.DefaultConfigA()

	ch := make(chan string, 1024)
	var wg sync.WaitGroup
	var mu sync.Mutex
	results := make([]result, 0, len(paths))
	var done int64

	t0 := time.Now()
	for i := 0; i < *jobs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			scB1 := trueyz.AllocScratch()
			defer trueyz.FreeScratch(scB1)
			scB2 := trueyzstock.AllocScratch()
			defer trueyzstock.FreeScratch(scB2)
			for p := range ch {
				r := checkOne(p, cfgA, scB1, scB2)
				mu.Lock()
				results = append(results, r)
				mu.Unlock()
				atomic.AddInt64(&done, 1)
			}
		}()
	}
	stop := make(chan struct{})
	go progress(stop, &done, len(paths))
	for _, p := range paths {
		ch <- p
	}
	close(ch)
	wg.Wait()
	close(stop)
	elapsed := time.Since(t0)

	sort.Slice(results, func(i, j int) bool { return results[i].path < results[j].path })

	var nB1, nB2, nA int
	var fails []result
	for _, r := range results {
		switch r.which {
		case "B1":
			nB1++
		case "B2":
			nB2++
		case "A":
			nA++
		default:
			fails = append(fails, r)
		}
	}

	fmt.Printf("\n总文件                                    : %d\n", len(results))
	fmt.Printf("编码器 B1 (TrueYZ 原版)                   : %d\n", nB1)
	fmt.Printf("编码器 B2 (TrueYZ + stock MIN_LOOKAHEAD)  : %d\n", nB2)
	fmt.Printf("编码器 A  (far-tie + lookahead-commit)    : %d\n", nA)
	fmt.Printf("失败                                      : %d\n", len(fails))
	if len(results) > 0 {
		fmt.Printf("覆盖率                                    : %.4f%%\n",
			100*float64(nB1+nB2+nA)/float64(len(results)))
	}
	fmt.Printf("耗时                                      : %s\n", elapsed.Round(time.Millisecond))
	for i, f := range fails {
		if i >= 20 {
			fmt.Printf("  ... 其余 %d 个失败\n", len(fails)-20)
			break
		}
		fmt.Printf("  [%s] %s  %s\n", f.which, f.path, f.note)
	}

	if *report != "" {
		if err := writeReport(*report, results); err != nil {
			fmt.Fprintf(os.Stderr, "写报告失败: %v\n", err)
		} else {
			fmt.Printf("报告: %s\n", *report)
		}
	}
	if *failOut != "" {
		if len(fails) == 0 {
			os.Remove(*failOut)
		} else {
			f, _ := os.Create(*failOut)
			for _, r := range fails {
				fmt.Fprintln(f, r.path)
			}
			f.Close()
			fmt.Printf("失败清单: %s\n", *failOut)
		}
	}
}

func progress(stop chan struct{}, done *int64, total int) {
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			fmt.Fprintf(os.Stderr, "\r进度 %d/%d (%.1f%%)", atomic.LoadInt64(done), total,
				100*float64(atomic.LoadInt64(done))/float64(total))
		}
	}
}

func scanDir(root string, jobs int, scanned, yaz0 *int64) []string {
	ch := make(chan string, 1024)
	out := make(chan string, 1024)
	var wg sync.WaitGroup
	for i := 0; i < jobs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 4)
			for p := range ch {
				f, err := os.Open(p)
				if err != nil {
					continue
				}
				n, _ := io.ReadFull(f, buf)
				f.Close()
				if n == 4 && bytes.Equal(buf, []byte("Yaz0")) {
					out <- p
				}
			}
		}()
	}
	var collected []string
	done := make(chan struct{})
	go func() {
		for p := range out {
			collected = append(collected, p)
		}
		close(done)
	}()
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		atomic.AddInt64(scanned, 1)
		ch <- path
		return nil
	})
	close(ch)
	wg.Wait()
	close(out)
	<-done
	sort.Strings(collected)
	*yaz0 = int64(len(collected))
	return collected
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

func writeReport(path string, rs []result) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriterSize(f, 1<<20)
	defer w.Flush()
	fmt.Fprintln(w, "path\twhich\tcompSize\trawSize\trebuiltB1\trebuiltB2\trebuiltA")
	for _, r := range rs {
		fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%d\t%d\t%d\n",
			r.path, r.which, r.comp, r.raw, r.reB1, r.reB2, r.reA)
	}
	return w.Flush()
}
