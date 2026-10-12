// Package trueyzstock 是 TrueYZ 的一个变体(B2): 把 MIN_LOOKAHEAD 从
//
//	MAX_MATCH + MIN_MATCH      (TrueYZ 的选择)
//
// 改回 stock zlib 的
//
//	MAX_MATCH + MIN_MATCH + 1
//
// 上游 README 把前者列为"刻意偏离、必须照抄", 但 BotW 里有 5 个文件
// 用的恰恰是 stock 值。导出的符号已改名(tystock_*), 以便和原版 TrueYZ
// 链接进同一个二进制。
package trueyzstock

/*
#cgo CFLAGS: -O2 -DTRUEYZ_STATE=TRUEYZ_STATE_SCRATCH
#include <stdint.h>
#include <stdlib.h>
#include "trueyz.h"

static int mtyz_stock_compress_align(const uint8_t* in, int len, uint8_t* out,
                                     int alignment, void* scratch)
{
    return tystock_compress_align_scratch((const void*)in, len, (void*)out,
                                          alignment, scratch);
}

static size_t mtyz_stock_scratch_size(void)
{
    return (size_t)TRUEYZ_SCRATCH_SIZE;
}
*/
import "C"

import "unsafe"

// ScratchSize 是每个 goroutine 需要独占的 match finder 大小。
func ScratchSize() int { return int(C.mtyz_stock_scratch_size()) }

// AllocScratch 分配一块 scratch, 用完需 FreeScratch。
func AllocScratch() unsafe.Pointer { return C.malloc(C.size_t(ScratchSize())) }

// FreeScratch 释放 AllocScratch 得到的 scratch。
func FreeScratch(p unsafe.Pointer) { C.free(p) }

// CompressAlign 压缩 in[:inLen] 并写入 alignment 提示, 返回字节数(0 = 失败)。
func CompressAlign(in []byte, inLen int, out []byte, align int, scratch unsafe.Pointer) int {
	if inLen < 0 || len(out) == 0 {
		return 0
	}
	return int(C.mtyz_stock_compress_align(
		(*C.uint8_t)(unsafe.Pointer(&in[0])),
		C.int(inLen),
		(*C.uint8_t)(unsafe.Pointer(&out[0])),
		C.int(align),
		scratch))
}
