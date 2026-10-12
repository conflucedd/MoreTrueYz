// Command yaz0verify 是"只用 FastYZ 解包 + 原版 TrueYZ 重压"的基线校验,
// 用来复现报告里 37074/38614(96.01%) 这个中间结论。
//
// 完整的 100% 校验请用 cmd/moretrueyz。
//
// 用法:
//
//	yaz0verify -root /path/to/botw -j 32 -report trueyz-only.tsv
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
)

type record struct {
	path   string
	comp   int
	raw    int
	align  uint32
	status string
	note   string
}

func main() {
	root := flag.String("root", "", "要扫描的目录")
	list := flag.String("list", "", "文件清单(与 -root 二选一)")
	report := flag.String("report", "", "逐文件结果 TSV")
	jobs := flag.Int("j", runtime.NumCPU(), "并发")
	flag.Parse()

	if *root == "" && *list == "" {
		fmt.Fprintln(os.Stderr, "需要 -root 或 -list 之一")
		os.Exit(2)
	}
	var paths []string
	if *list != "" {
		paths = readList(*list)
	} else {
		paths = scanDir(*root, *jobs)
	}
	fmt.Printf("待校验: %d 个 Yaz0, 并发 %d\n", len(paths), *jobs)

	ch := make(chan string, 1024)
	var wg sync.WaitGroup
	var mu sync.Mutex
	recs := make([]record, 0, len(paths))
	var done int64
	t0 := time.Now()
	for i := 0; i < *jobs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sc := trueyz.AllocScratch()
			defer trueyz.FreeScratch(sc)
			for p := range ch {
				mu.Lock()
				recs = append(recs, verify(p, sc))
				mu.Unlock()
				atomic.AddInt64(&done, 1)
			}
		}()
	}
	for _, p := range paths {
		ch <- p
	}
	close(ch)
	wg.Wait()
	elapsed := time.Since(t0)
	sort.Slice(recs, func(i, j int) bool { return recs[i].path < recs[j].path })

	count := map[string]int{}
	for _, r := range recs {
		count[r.status]++
	}
	ok := count["OK"]
	fmt.Printf("\nYaz0 文件   : %d\n", len(recs))
	fmt.Printf("TrueYZ 复现 : %d / %d (%.2f%%)\n", ok, len(recs),
		100*float64(ok)/float64(max(1, len(recs))))
	for _, k := range []string{"OK", "SIZE_MISMATCH", "BYTE_MISMATCH", "DECOMPRESS_FAIL", "READ_FAIL"} {
		if count[k] > 0 {
			fmt.Printf("  %-16s %d\n", k, count[k])
		}
	}
	fmt.Printf("耗时        : %s\n", elapsed.Round(time.Millisecond))
	if *report != "" {
		if err := writeReport(*report, recs); err == nil {
			fmt.Printf("报告: %s\n", *report)
		}
	}
}

func verify(path string, scratch unsafe.Pointer) record {
	rec := record{path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		rec.status = "READ_FAIL"
		return rec
	}
	rec.comp = len(data)
	if !fastyz.IsYaz0(data) {
		rec.status = "NOT_YAZ0"
		return rec
	}
	rec.align = uint32(data[8])<<24 | uint32(data[9])<<16 | uint32(data[10])<<8 | uint32(data[11])
	rawSize := int(fastyz.RawSize(data))
	rec.raw = rawSize
	raw := make([]byte, rawSize)
	if rawSize > 0 {
		if fastyz.Decompress(data, raw, rawSize) != rawSize {
			rec.status = "DECOMPRESS_FAIL"
			return rec
		}
	}
	in := raw
	if len(in) == 0 {
		in = []byte{0}
	}
	out := make([]byte, rawSize+fastyz.HeaderSize+rawSize/8+1)
	m := trueyz.CompressAlign(in, rawSize, out, int(rec.align), scratch)
	switch {
	case m == len(data) && m > 0 && bytes.Equal(out[:m], data):
		rec.status = "OK"
	case m != len(data):
		rec.status = "SIZE_MISMATCH"
	default:
		rec.status = "BYTE_MISMATCH"
	}
	return rec
}

func scanDir(root string, jobs int) []string {
	ch := make(chan string, 1024)
	var wg sync.WaitGroup
	var collected []string
	var mu sync.Mutex
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
					mu.Lock()
					collected = append(collected, p)
					mu.Unlock()
				}
			}
		}()
	}
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		ch <- path
		return nil
	})
	close(ch)
	wg.Wait()
	sort.Strings(collected)
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

func writeReport(path string, rs []record) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriterSize(f, 1<<20)
	defer w.Flush()
	fmt.Fprintln(w, "path\tcompSize\trawSize\talign\tstatus")
	for _, r := range rs {
		fmt.Fprintf(w, "%s\t%d\t%d\t0x%08X\t%s\n", r.path, r.comp, r.raw, r.align, r.status)
	}
	return w.Flush()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
