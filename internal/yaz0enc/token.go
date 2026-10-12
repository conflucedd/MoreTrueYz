package yaz0enc

// Token 是 Yaz0 流里的一个词法单元。
type Token struct {
	Off     int  // token 在压缩流中的字节偏移
	Pos     int  // 在解压结果中的起始位置
	Literal bool // true: 字面字节; false: 回溯引用
	Len     int  // 字面为 1, 回溯为 3..273
	Dist    int  // 回溯距离(1-based); 字面为 0
}

// ParseTokens 解析完整 Yaz0 文件(含 16 字节头)的 token 序列。
// 遇到损坏时返回已解析的部分。
func ParseTokens(data []byte) ([]Token, error) {
	if len(data) < HeaderSize || string(data[:4]) != "Yaz0" {
		return nil, errNotYaz0
	}
	size := int(data[4])<<24 | int(data[5])<<16 | int(data[6])<<8 | int(data[7])
	var out []Token
	dst := 0
	p := HeaderSize
	for dst < size {
		if p >= len(data) {
			return out, errTruncated
		}
		flagOff := p
		flag := data[p]
		p++
		for bit := 0; bit < 8 && dst < size; bit++ {
			if flag&(0x80>>uint(bit)) != 0 {
				if p >= len(data) {
					return out, errTruncated
				}
				out = append(out, Token{Off: p, Pos: dst, Literal: true, Len: 1})
				p++
				dst++
			} else {
				if p+2 > len(data) {
					return out, errTruncated
				}
				off := p
				b1, b2 := data[p], data[p+1]
				p += 2
				d := (int(b1&0x0F)<<8 | int(b2)) + 1
				l := int(b1 >> 4)
				if l == 0 {
					if p >= len(data) {
						return out, errTruncated
					}
					l = int(data[p]) + 18
					p++
				} else {
					l += 2
				}
				out = append(out, Token{Off: off, Pos: dst, Len: l, Dist: d})
				dst += l
			}
		}
		_ = flagOff
	}
	return out, nil
}

type yaz0Error string

func (e yaz0Error) Error() string { return string(e) }

const (
	errNotYaz0   = yaz0Error("不是 Yaz0")
	errTruncated = yaz0Error("压缩流被截断")
)
