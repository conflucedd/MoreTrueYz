// Package yaz0enc 的自检: 编码结果必须能被自己的解码器还原, 且 UTF-8/随机
// 数据在编码器 A 的各个模式下都能无损往返。
package yaz0enc

import (
	"bytes"
	"math/rand"
	"testing"
)

func roundtrip(t *testing.T, raw []byte, cfg Config) {
	t.Helper()
	enc := Encode(raw, cfg)
	if len(enc) < HeaderSize || string(enc[:4]) != "Yaz0" {
		t.Fatalf("不是合法 Yaz0 输出")
	}
	got := Decode(enc)
	if !bytes.Equal(got, raw) {
		t.Fatalf("往返失败: 原始 %d 字节, 解出 %d 字节, 首个差异在 %d",
			len(raw), len(got), firstDiff(raw, got))
	}
}

func firstDiff(a, b []byte) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

func TestRoundTripConfigs(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	var random []byte
	for i := 0; i < 4096; i++ {
		random = append(random, byte(rng.Intn(256)))
	}
	repetitive := bytes.Repeat([]byte{0, 0, 0, 0, 1, 2, 3}, 2048)

	cases := []struct {
		name string
		raw  []byte
	}{
		{"empty", nil},
		{"one", []byte{7}},
		{"text", []byte("The quick brown fox jumps over the lazy dog. " +
			"The quick brown fox jumps over the lazy dog.")},
		{"random", random},
		{"repetitive", repetitive},
	}
	configs := []Config{
		DefaultConfigA(),
		{TieFar: true, Lazy: 0, Eager: true, Magic: [4]byte{'Y', 'a', 'z', '0'}},
		{TieFar: false, Lazy: 1, Margin: 0, Eager: true, Magic: [4]byte{'Y', 'a', 'z', '0'}},
		{TieFar: false, Lazy: 2, Eager: true, Magic: [4]byte{'Y', 'a', 'z', '0'}},
		{TieFar: true, Lazy: 3, Margin: 2, Eager: false, Magic: [4]byte{'Y', 'a', 'z', '0'}},
		{TieFar: true, Lazy: 1, Margin: 0, MaxLazy: 32, MaxDist: 3820, Eager: true,
			Magic: [4]byte{'Y', 'a', 'z', '0'}},
	}
	for _, c := range configs {
		for _, tc := range cases {
			roundtrip(t, tc.raw, c)
		}
	}
}

// 编码器 A 的 flag 分组是"提前开组": token 数正好是 8 的倍数时,
// 输出尾部会多一个未使用的 0x00 字节。
func TestEagerFlagGroupLeavesTrailingByte(t *testing.T) {
	// 构造 8 个 token: 7 个字面 + 1 个匹配 -> 8 个 token。
	// 数据必须让位置 7 处出现一个 >=3 的匹配。
	raw := []byte{1, 2, 3, 4, 5, 6, 7, 1, 2, 3}
	cfg := Config{TieFar: true, Lazy: 0, Eager: true, Magic: [4]byte{'Y', 'a', 'z', '0'}}
	enc := Encode(raw, cfg)
	toks, consumed, ok := TokenCount(enc)
	if !ok {
		t.Fatal("TokenCount 解析失败")
	}
	if toks%8 != 0 {
		t.Skipf("该样本 token 数 %d 不是 8 的倍数, 跳过", toks)
	}
	if consumed == len(enc) {
		t.Fatalf("Eager 模式应在 token 数 %% 8 == 0 时留下 1 个尾部字节: consumed=%d len=%d",
			consumed, len(enc))
	}
	if len(enc)-consumed != 1 || enc[consumed] != 0 {
		t.Fatalf("尾部应恰好是 1 个 0x00: 剩余 %d 字节, 首字节 0x%02X",
			len(enc)-consumed, enc[consumed])
	}
	// 非 eager 时不应该有尾部字节。
	cfg.Eager = false
	enc2 := Encode(raw, cfg)
	if _, consumed2, ok := TokenCount(enc2); ok && consumed2 != len(enc2) {
		t.Fatalf("非 Eager 模式不应留下尾部字节: consumed=%d len=%d", consumed2, len(enc2))
	}
}
