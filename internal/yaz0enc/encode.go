package yaz0enc

import "encoding/binary"

// Config 描述一台候选编码器。
type Config struct {
	Name    string
	TieFar  bool   // true: 等长匹配取距离最远(窗口内最早出现); false: 取最近
	Lazy    int    // 0=贪心, 1=下一位置更长才推迟(>), 2=下一位置不短就推迟(>=), 3=lookahead-and-commit
	Margin  int    // Lazy==1/3 时的余量: 仅当下一匹配比当前长超过该值才推迟
	MaxLazy int    // >0: 上一匹配长度达到该值就不再搜索(zlib 语义)
	Eager   bool   // true: flag 组一满就立刻开下一组(可能留下一个空 flag 字节)
	MaxDist int    // 回溯距离上限, 0 表示 4096
	Align   uint32 // 写入文件头 0x08..0x0B 的 alignment 提示
	Magic   [4]byte
}

// DefaultConfigA 是逆向出来的编码器 A: 远 tie + lookahead-and-commit(margin=1)
// + 提前开 flag 组。用于 BotW 的 .sblwp / .smubin。
func DefaultConfigA() Config {
	return Config{
		Name:   "A",
		TieFar: true,
		Lazy:   3,
		Margin: 1,
		Eager:  true,
		Magic:  [4]byte{'Y', 'a', 'z', '0'},
	}
}

const hashLog = 16

// Matcher 是一个全窗口(4096)的匹配查找器, 用 3 字节哈希 + 链。
// 链不设长度上限, 因此找到的是窗口内真正的"最长匹配"。
type Matcher struct {
	raw     []byte
	head    []int32
	prev    []int32
	ins     int
	tieFar  bool
	maxDist int
}

// NewMatcher 建一个匹配器; maxDist<=0 时用 4096。
func NewMatcher(raw []byte, tieFar bool, maxDist int) *Matcher {
	if maxDist <= 0 || maxDist > MaxDist {
		maxDist = MaxDist
	}
	head := make([]int32, 1<<hashLog)
	for i := range head {
		head[i] = -1
	}
	prev := make([]int32, len(raw))
	for i := range prev {
		prev[i] = -1
	}
	return &Matcher{raw: raw, head: head, prev: prev, tieFar: tieFar, maxDist: maxDist}
}

func (m *Matcher) hash(i int) uint32 {
	h := uint32(m.raw[i])<<16 | uint32(m.raw[i+1])<<8 | uint32(m.raw[i+2])
	h *= 2654435761
	return h >> (32 - hashLog)
}

func (m *Matcher) insertAt(i int) {
	if i+2 >= len(m.raw) {
		m.prev[i] = -1
		return
	}
	h := m.hash(i)
	m.prev[i] = m.head[h]
	m.head[h] = int32(i)
}

// InsertUpTo 把所有 < end 的位置插入哈希链(只插一次)。
func (m *Matcher) InsertUpTo(end int) {
	for m.ins < end {
		m.insertAt(m.ins)
		m.ins++
	}
}

// Longest 在 [p-maxDist, p) 内找最长匹配(上限 273), 返回长度与源位置。
// 找不到 >=3 的匹配时返回 (0, -1)。
func (m *Matcher) Longest(p int) (int, int) {
	n := len(m.raw)
	if p+2 >= n {
		return 0, -1
	}
	limit := n - p
	if limit > MaxMatch {
		limit = MaxMatch
	}
	q := m.head[m.hash(p)]
	bestLen, bestPos := 0, -1
	for q >= 0 {
		idx := int(q)
		if p-idx > m.maxDist {
			break
		}
		if m.raw[idx] == m.raw[p] {
			l := 1
			for l < limit && m.raw[idx+l] == m.raw[p+l] {
				l++
			}
			if l >= MinMatch {
				if l > bestLen || (l == bestLen && m.tieFar && idx < bestPos) {
					bestLen = l
					bestPos = idx
					if bestLen == limit && !m.tieFar {
						break
					}
				}
			}
		}
		q = m.prev[idx]
	}
	if bestLen < MinMatch {
		return 0, -1
	}
	return bestLen, bestPos
}

// writer 按 Yaz0 的 flag 分组写出 token。
type writer struct {
	cfg     Config
	out     []byte
	codePos int
	mask    byte
}

func newWriter(rawLen int, cfg Config) *writer {
	return &writer{cfg: cfg, out: make([]byte, 0, rawLen+rawLen/8+64), codePos: -1}
}

func (w *writer) reserve() {
	w.codePos = len(w.out)
	w.out = append(w.out, 0)
	w.mask = 0x80
}
func (w *writer) begin() {
	if w.mask == 0 {
		w.reserve()
	}
}
func (w *writer) endTok() {
	w.mask >>= 1
	if w.cfg.Eager && w.mask == 0 {
		w.reserve()
	}
}
func (w *writer) literal(b byte) {
	w.begin()
	w.out[w.codePos] |= w.mask
	w.out = append(w.out, b)
	w.endTok()
}
func (w *writer) match(length, src, cur int) {
	w.begin()
	delta := cur - src - 1
	if length < 0x12 {
		w.out = append(w.out, byte(delta>>8)|byte(length-2)<<4, byte(delta))
	} else {
		w.out = append(w.out, byte(delta>>8), byte(delta), byte(length-0x12))
	}
	w.endTok()
}
func (w *writer) finish(n int) []byte {
	hdr := make([]byte, HeaderSize+len(w.out))
	magic := w.cfg.Magic
	if magic == [4]byte{} {
		magic = [4]byte{'Y', 'a', 'z', '0'}
	}
	copy(hdr[0:4], magic[:])
	binary.BigEndian.PutUint32(hdr[4:8], uint32(n))
	binary.BigEndian.PutUint32(hdr[8:12], w.cfg.Align)
	copy(hdr[HeaderSize:], w.out)
	return hdr
}

// Encode 按 cfg 把 raw 压成完整 Yaz0 文件。
func Encode(raw []byte, cfg Config) []byte {
	if cfg.Lazy == 3 {
		return encodeLookahead(raw, cfg)
	}
	return encodeZlib(raw, cfg)
}

// encodeLookahead 是编码器 A 的核心: 每个位置找最长匹配 L1; 若下一位置的
// 最长匹配 L2 比 L1 长超过 Margin, 就输出一个字面字节并"立即提交" pos+1
// 处的匹配(只推迟一次, 不链式推迟)。
func encodeLookahead(raw []byte, cfg Config) []byte {
	n := len(raw)
	m := NewMatcher(raw, cfg.TieFar, cfg.MaxDist)
	w := newWriter(n, cfg)
	for pos := 0; pos < n; {
		m.InsertUpTo(pos)
		L1, P1 := m.Longest(pos)
		m.InsertUpTo(pos + 1)
		L2, P2 := m.Longest(pos + 1)
		if L1 >= MinMatch && L2 > L1+cfg.Margin {
			w.literal(raw[pos])
			if L2 >= MinMatch {
				w.match(L2, P2, pos+1)
				pos += 1 + L2
			} else {
				pos++
			}
			continue
		}
		if L1 >= MinMatch {
			w.match(L1, P1, pos)
			pos += L1
			continue
		}
		w.literal(raw[pos])
		pos++
	}
	return w.finish(n)
}

// encodeZlib 是 zlib deflate_slow 式的惰性匹配(TrueYZ 那一支的结构),
// 用于规则搜索时做对照。
func encodeZlib(raw []byte, cfg Config) []byte {
	n := len(raw)
	m := NewMatcher(raw, cfg.TieFar, cfg.MaxDist)
	w := newWriter(n, cfg)

	pos := 0
	prevLen, prevPos := 0, -1
	pending := false
	for pos < n {
		m.InsertUpTo(pos)
		L, P := 0, -1
		if !(cfg.Lazy != 0 && cfg.MaxLazy > 0 && pending && prevLen >= cfg.MaxLazy) {
			L, P = m.Longest(pos)
		}
		if pending && prevLen >= MinMatch {
			take := false
			switch cfg.Lazy {
			case 0:
				take = true
			case 1:
				take = L <= prevLen+cfg.Margin
			case 2:
				take = L < prevLen
			}
			if take {
				w.match(prevLen, prevPos, pos-1)
				pos = pos - 1 + prevLen
				pending = false
				continue
			}
		}
		if pending {
			w.literal(raw[pos-1])
		}
		prevLen, prevPos = L, P
		pending = true
		pos++
	}
	if pending {
		w.literal(raw[n-1])
	}
	return w.finish(n)
}
