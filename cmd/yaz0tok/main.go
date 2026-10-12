// Command yaz0tok 解析/对比 Yaz0 的 token 流, 用于定位两个压缩结果的
// "决策层"差异。
//
//	yaz0tok dump file.yaz0
//	yaz0tok diff orig.yaz0 rebuilt.szs
package main

import (
	"fmt"
	"os"

	"moretrueyz/internal/yaz0enc"
)

func describe(t yaz0enc.Token) string {
	if t.Literal {
		return "字面"
	}
	return fmt.Sprintf("匹配 len=%d dist=%d", t.Len, t.Dist)
}

func readFile(p string) []byte {
	b, err := os.ReadFile(p)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return b
}

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "用法: yaz0tok dump|diff <a> [b]")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "dump":
		data := readFile(os.Args[2])
		toks, err := yaz0enc.ParseTokens(data)
		fmt.Printf("%s\n  压缩=%d  token 数=%d\n", os.Args[2], len(data), len(toks))
		if err != nil {
			fmt.Println("  解析警告:", err)
		}
		for i, t := range toks {
			if i >= 40 {
				fmt.Printf("  ... 其余 %d 个 token\n", len(toks)-40)
				break
			}
			fmt.Printf("  #%-4d dst=%-8d %-24s src=0x%06X\n", i, t.Pos, describe(t), t.Off)
		}
	case "diff":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "diff 需要两个文件")
			os.Exit(2)
		}
		a := readFile(os.Args[2])
		b := readFile(os.Args[3])
		ta, ea := yaz0enc.ParseTokens(a)
		tb, eb := yaz0enc.ParseTokens(b)
		fmt.Printf("A=%s  comp=%d tokens=%d err=%v\n", os.Args[2], len(a), len(ta), ea)
		fmt.Printf("B=%s  comp=%d tokens=%d err=%v\n", os.Args[3], len(b), len(tb), eb)
		n := len(ta)
		if len(tb) < n {
			n = len(tb)
		}
		first := -1
		for i := 0; i < n; i++ {
			x, y := ta[i], tb[i]
			if x.Pos != y.Pos || x.Literal != y.Literal || x.Len != y.Len || x.Dist != y.Dist {
				first = i
				break
			}
		}
		if first < 0 {
			if len(ta) == len(tb) {
				fmt.Println("token 流完全一致")
			} else {
				fmt.Printf("前 %d 个 token 一致, token 数不同 (A=%d B=%d)\n", n, len(ta), len(tb))
			}
			return
		}
		fmt.Printf("\n第一处 token 差异: #%d (A@0x%06X / B@0x%06X)\n", first, ta[first].Off, tb[first].Off)
		lo := first - 3
		if lo < 0 {
			lo = 0
		}
		hi := first + 4
		if hi > n {
			hi = n
		}
		for i := lo; i < hi; i++ {
			mark := "  "
			if i == first {
				mark = "->"
			}
			fmt.Printf(" %s #%-4d A: dst=%-8d %-22s | B: dst=%-8d %-22s\n",
				mark, i, ta[i].Pos, describe(ta[i]), tb[i].Pos, describe(tb[i]))
		}
		showBytes(a, ta[first], "A")
		showBytes(b, tb[first], "B")
	default:
		fmt.Fprintln(os.Stderr, "未知命令")
		os.Exit(2)
	}
}

func showBytes(data []byte, t yaz0enc.Token, name string) {
	lo := t.Off - 4
	if lo < 0 {
		lo = 0
	}
	hi := t.Off + 8
	if hi > len(data) {
		hi = len(data)
	}
	fmt.Printf("   %s 压缩流 @0x%06X: % X\n", name, lo, data[lo:hi])
}
