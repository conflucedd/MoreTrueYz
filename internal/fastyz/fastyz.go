// Package fastyz 用 cgo 封装上游 FastYZ 的 Yaz0 解压。
//
// 编译宏 -DFASTYZ_HTAB=FASTYZ_HTAB_SCRATCH 只是为了让压缩 API 变成
// 可重入的; 解压本身一直是线程安全的。
package fastyz

/*
#cgo CFLAGS: -O2 -DFASTYZ_HTAB=FASTYZ_HTAB_SCRATCH
#include <stdint.h>
#include <stdlib.h>
#include "fastyz.h"

static int mtyz_decompress(const uint8_t* in, int in_len, uint8_t* out, int max_out)
{
    return yaz0_decompress((const void*)in, in_len, (void*)out, max_out);
}

static uint32_t mtyz_raw_size(const uint8_t* in)
{
    return yaz0_get_decompressed_size((const void*)in);
}

static int mtyz_is_yaz0(const uint8_t* in)
{
    return yaz0_is_valid((const void*)in);
}
*/
import "C"

import "unsafe"

// HeaderSize 是 Yaz0 文件头长度。
const HeaderSize = 16

// IsYaz0 判断 b 是否以 Yaz0 魔数开头。
func IsYaz0(b []byte) bool {
	if len(b) < 4 {
		return false
	}
	return C.mtyz_is_yaz0((*C.uint8_t)(unsafe.Pointer(&b[0]))) != 0
}

// RawSize 返回文件头里记录的原始大小(非 Yaz0 返回 0)。
func RawSize(b []byte) uint32 {
	if len(b) < HeaderSize {
		return 0
	}
	return uint32(C.mtyz_raw_size((*C.uint8_t)(unsafe.Pointer(&b[0]))))
}

// Decompress 把 in 解压到 out, 返回写入字节数。
// 返回 0 表示失败, 或解压结果是空(两者 FastYZ 不可区分)。
func Decompress(in, out []byte, maxOut int) int {
	if len(in) == 0 || len(out) == 0 {
		return 0
	}
	return int(C.mtyz_decompress(
		(*C.uint8_t)(unsafe.Pointer(&in[0])),
		C.int(len(in)),
		(*C.uint8_t)(unsafe.Pointer(&out[0])),
		C.int(maxOut)))
}
