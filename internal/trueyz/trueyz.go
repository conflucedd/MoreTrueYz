// Package trueyz 用 cgo 封装上游 TrueYZ 的字节精确压缩(B1)。
package trueyz

/*
#cgo CFLAGS: -O2 -DTRUEYZ_STATE=TRUEYZ_STATE_SCRATCH
#include <stdint.h>
#include <stdlib.h>
#include "trueyz.h"

static int mtyz_compress_align(const uint8_t* in, int len, uint8_t* out,
                               int alignment, void* scratch)
{
    return trueyz_compress_align_scratch((const void*)in, len, (void*)out,
                                         alignment, scratch);
}

static size_t mtyz_scratch_size(void)
{
    return (size_t)TRUEYZ_SCRATCH_SIZE;
}
*/
import "C"

import "unsafe"

// ScratchSize 是每个 goroutine 需要独占的 match finder 大小。
func ScratchSize() int { return int(C.mtyz_scratch_size()) }

// AllocScratch 分配一块 scratch, 用完需 FreeScratch。
func AllocScratch() unsafe.Pointer { return C.malloc(C.size_t(ScratchSize())) }

// FreeScratch 释放 AllocScratch 得到的 scratch。
func FreeScratch(p unsafe.Pointer) { C.free(p) }

// CompressAlign 压缩 in[:inLen] 并写入 alignment 提示, 返回字节数(0 = 失败)。
func CompressAlign(in []byte, inLen int, out []byte, align int, scratch unsafe.Pointer) int {
	if inLen < 0 || len(out) == 0 {
		return 0
	}
	return int(C.mtyz_compress_align(
		(*C.uint8_t)(unsafe.Pointer(&in[0])),
		C.int(inLen),
		(*C.uint8_t)(unsafe.Pointer(&out[0])),
		C.int(align),
		scratch))
}
