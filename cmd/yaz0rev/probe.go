package main

import (
	"fmt"
	"os"

	"moretrueyz/internal/yaz0enc"
)

// probe 打印 [from,to) 区间内每个位置: 原文件的选择 + 全窗口 oracle。
// 这是逆推出"lookahead-and-commit / margin=1"那条规则的主要工具。
func probe(path string, from, to int) {
	orig := readFile(path)
	raw := yaz0enc.Decode(orig)
	if raw == nil {
		fmt.Fprintln(os.Stderr, "解码失败")
		return
	}
	toks, err := yaz0enc.ParseTokens(orig)
	if err != nil {
		fmt.Fprintln(os.Stderr, "token 解析:", err)
	}
	byPos := map[int]yaz0enc.Token{}
	for _, t := range toks {
		byPos[t.Pos] = t
	}
	mF := yaz0enc.NewMatcher(raw, true, 0)  // 远 tie
	mN := yaz0enc.NewMatcher(raw, false, 0) // 近 tie
	if to > len(raw) {
		to = len(raw)
	}
	for p := from; p < to; p++ {
		mF.InsertUpTo(p)
		mN.InsertUpTo(p)
		lf, pf := mF.Longest(p)
		ln, pn := mN.Longest(p)
		desc := "(匹配内部)"
		if t, ok := byPos[p]; ok {
			if t.Literal {
				desc = "字面"
			} else {
				desc = fmt.Sprintf("匹配 len=%d dist=%d", t.Len, t.Dist)
			}
		}
		dF, dN := -1, -1
		if pf >= 0 {
			dF = p - pf
		}
		if pn >= 0 {
			dN = p - pn
		}
		fmt.Printf("p=%-8d oracleFar len=%-3d d=%-5d | oracleNear len=%-3d d=%-5d | 原: %s\n",
			p, lf, dF, ln, dN, desc)
		mF.InsertUpTo(p + 1)
		mN.InsertUpTo(p + 1)
	}
}
