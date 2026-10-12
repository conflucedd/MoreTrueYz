// Package yaz0enc 提供 Yaz0 的解码、token 解析, 以及本项目逆向出的
// "编码器 A"(BotW 地图数据 .sblwp/.smubin 所用的那支)。
package yaz0enc

import "encoding/binary"

// Yaz0 格式常量。
const (
	HeaderSize = 16
	MinMatch   = 3
	MaxMatch   = 273
	MaxDist    = 4096
)

// Decode 解出 Yaz0 的原始数据; 不是合法 Yaz0 时返回 nil。
func Decode(data []byte) []byte {
	if len(data) < HeaderSize || string(data[:4]) != "Yaz0" {
		return nil
	}
	size := int(binary.BigEndian.Uint32(data[4:8]))
	out := make([]byte, 0, size)
	p := HeaderSize
	for len(out) < size {
		if p >= len(data) {
			return nil
		}
		flag := data[p]
		p++
		for bit := 0; bit < 8 && len(out) < size; bit++ {
			if flag&(0x80>>uint(bit)) != 0 {
				if p >= len(data) {
					return nil
				}
				out = append(out, data[p])
				p++
			} else {
				if p+2 > len(data) {
					return nil
				}
				b1, b2 := data[p], data[p+1]
				p += 2
				dist := (int(b1&0x0F)<<8 | int(b2)) + 1
				l := int(b1 >> 4)
				if l == 0 {
					if p >= len(data) {
						return nil
					}
					l = int(data[p]) + 18
					p++
				} else {
					l += 2
				}
				src := len(out) - dist
				if src < 0 {
					return nil
				}
				for k := 0; k < l; k++ {
					out = append(out, out[src+k])
				}
			}
		}
	}
	return out
}

// TokenCount 返回 token 数量和最后一个 token 消费到的偏移, 用于检查
// 压缩流尾部是否有未被消费的字节(编码器 A 会留下一个空 flag 字节)。
func TokenCount(data []byte) (tokens, consumed int, ok bool) {
	if len(data) < HeaderSize || string(data[:4]) != "Yaz0" {
		return 0, 0, false
	}
	size := int(binary.BigEndian.Uint32(data[4:8]))
	dst := 0
	p := HeaderSize
	for dst < size {
		if p >= len(data) {
			return tokens, p, false
		}
		flag := data[p]
		p++
		for bit := 0; bit < 8 && dst < size; bit++ {
			if flag&(0x80>>uint(bit)) != 0 {
				if p >= len(data) {
					return tokens, p, false
				}
				p++
				dst++
			} else {
				if p+2 > len(data) {
					return tokens, p, false
				}
				b1 := data[p]
				p += 2
				l := int(b1 >> 4)
				if l == 0 {
					if p >= len(data) {
						return tokens, p, false
					}
					l = int(data[p]) + 18
					p++
				} else {
					l += 2
				}
				dst += l
			}
			tokens++
		}
	}
	return tokens, p, true
}
