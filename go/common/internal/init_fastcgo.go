//go:build !without_fastcgo || with_fastcgo
// +build !without_fastcgo with_fastcgo

// This file contains the C/cgo-dependent parts of GO API using Fastcgo.
// The fastcgo is disabled on arm64, but could be enabled via --tags=with_fastcgo build tag.
package internal

// #cgo CFLAGS: -I.
// #cgo LDFLAGS: -L.
// #cgo sanitize LDFLAGS: -fsanitize=address
// #cgo sanitize CFLAGS: -fsanitize=address
// #cgo arm64,!sanitize,!dbg LDFLAGS: -l:aarch64_wal_c_api.a
// #cgo arm64,!sanitize,dbg LDFLAGS: -l:aarch64_wal_c_api_dbg.a
// #cgo arm64,sanitize,!dbg LDFLAGS: -l:aarch64_wal_c_api_asan.a
// #cgo arm64,sanitize,dbg LDFLAGS: -l:aarch64_wal_c_api_dbg_asan.a
// #cgo amd64,!sanitize,!dbg LDFLAGS: -l:x86_wal_c_api.a
// #cgo amd64,!sanitize,dbg LDFLAGS: -l:x86_wal_c_api_dbg.a
// #cgo amd64,sanitize,!dbg LDFLAGS: -l:x86_wal_c_api_asan.a
// #cgo amd64,sanitize,dbg LDFLAGS: -l:x86_wal_c_api_dbg_asan.a
// #cgo LDFLAGS: -static-libgcc -static-libstdc++ -l:libstdc++.a
// #cgo static LDFLAGS: -static
// #include "wal_c_encoder.h"
// #include "wal_c_decoder.h"
// #include "wal_c_types.h"
// #include <stdlib.h>
import "C"
import (
	"unsafe"

	"github.com/prometheus/prometheus/pp/go/common/fastcgo"
)

// CDecoder is the internal raw C/C++ decoder wrapper.
// It is intended to be used inside common.Decoder API
type CDecoder C.c_decoder

// CEncoder is the internal raw C/C++ encoder wrapper.
// It is intended to be used inside common.Encoder API
type CEncoder C.c_encoder

// CSlice is the Go<->C/C++ bridged type for forwarding
// Go slices (aka C++ std::span-s).
type CSlice C.c_slice

// CHashdex is the internal raw C/C++ wrapper over raw
// Go's Hashdex dynamic data and C/C++ presharding algo.
type CHashdex C.c_hashdex

// CErrorInfo is the bridge type containing the catched
// exception as the (function name: exception type and message)
// and stacktrace (if available)
type CErrorInfo C.c_api_error_info_ptr

// Init is intended to run C/C++ FFI initialize() function. In our case this function should
// call multiarch init function.
func Init() {
	fastcgo.UnsafeCall0(C.okdb_wal_initialize)
}

// EnableCoreDumps toggles generating coredumps from C++ Exceptions.
// It requres GOTRACEBACK=core env variable for Go runtime.
func EnableCoreDumps(enabled bool) {
	var en uintptr
	if enabled {
		en = 1
	} else {
		en = 0
	}

	fastcgo.UnsafeCall1(
		C.prompp_enable_coredumps_on_exception,
		en,
	)
}

//
// CByteSlice API
//

// CSegmentDestroy - wrapper for destructor CSegment.
func CSegmentDestroy(p unsafe.Pointer) {
	fastcgo.UnsafeCall1(C.okdb_wal_c_segment_destroy,
		uintptr(p))
}

//
// CEncoderAddManyStateHandle API
//

// CEncoderAddManyStateHandleDestroy - wrapper for destructor CEncoder.
func CEncoderAddManyStateHandleDestroy(handle uint64) {
	fastcgo.UnsafeCall1(C.okdb_wal_c_encoder_add_many_state_destroy,
		uintptr(handle))
}

//
// CDecoder API
//

// CDecoderDecodeArgs - args for Decode.
type CDecoderDecodeArgs struct {
	segment  *[]byte
	protobuf *GoDecodedSegment
}

// CDecoderDecodeDryArgs - args for DecodeDry.
type CDecoderDecodeDryArgs struct {
	segment *[]byte
}

// CDecoderDecodeResult - result after decode.
type CDecoderDecodeResult struct {
	result uint32
}

// CDecoderDecodeRestoreFromStreamArgs - args for fastCGO RestoreFromStream.
type CDecoderDecodeRestoreFromStreamArgs struct {
	buf            *[]byte
	restoredResult *GoRestoredResult
}

// CDecoderCtor - wrapper for constructor CDecoder.
func CDecoderCtor(err *GoErrorInfo) CDecoder {
	var decoder uintptr = 0
	fastcgo.UnsafeCall2(
		C.okdb_wal_uni_c_decoder_ctor,
		uintptr(unsafe.Pointer(&decoder)),
		uintptr(unsafe.Pointer(err)),
	)
	return CDecoder(unsafe.Pointer(decoder))
}

func CDecoderDecode(decoder CDecoder, segment []byte, protobufResult *GoDecodedSegment, err *GoErrorInfo) uint32 {
	var args = CDecoderDecodeArgs{
		segment:  &segment,
		protobuf: protobufResult,
	}

	var result = CDecoderDecodeResult{
		result: 0,
	}

	fastcgo.UnsafeCall4(
		C.okdb_wal_uni_c_decoder_decode,
		uintptr(unsafe.Pointer(decoder)),
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&result)),
		uintptr(unsafe.Pointer(err)),
	)

	return result.result
}

func CDecoderDecodeDry(decoder CDecoder, segment []byte, err *GoErrorInfo) uint32 {
	var args = CDecoderDecodeDryArgs{
		segment: &segment,
	}

	var result = CDecoderDecodeResult{
		result: 0,
	}

	fastcgo.UnsafeCall4(C.okdb_wal_uni_c_decoder_decode_dry,
		uintptr(unsafe.Pointer(decoder)),
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&result)),
		uintptr(unsafe.Pointer(err)),
	)

	return result.result
}

// CDecoderRestoreFromStream - fast GO wrapper for C/C++, calls C++ class Decoder methods.
func CDecoderRestoreFromStream(decoder CDecoder, buf []byte, restoredResult *GoRestoredResult, cerr *GoErrorInfo) {
	var args = CDecoderDecodeRestoreFromStreamArgs{
		buf:            &buf,
		restoredResult: restoredResult,
	}

	fastcgo.UnsafeCall3(C.okdb_wal_uni_c_decoder_restore_from_stream,
		uintptr(unsafe.Pointer(decoder)),
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(cerr)),
	)

}

// CDecoderDtor - wrapper for destructor CDecoder.
func CDecoderDtor(decoder CDecoder) {
	fastcgo.UnsafeCall1(C.okdb_wal_c_decoder_dtor, uintptr(decoder))
}

//
// CEncoder API
//

type CEncoderCtorArgs struct {
	shardID, numberOfShards uint16
}

type CEncoderEncodeArgs struct {
	hashdex   CHashdex
	segment   *GoSegment
	redundant *GoRedundant
}

type CEncoderAddArgs struct {
	hashdex CHashdex
	segment *GoSegment
}

type CEncoderFinalizeArgs struct {
	segment   *GoSegment
	redundant *GoRedundant
}

// CEncoderCtor - wrapper for constructor CEncoder.
func CEncoderCtor(shardID, numberOfShards uint16, err *GoErrorInfo) CEncoder {
	var encoder uintptr = 0
	var args = CEncoderCtorArgs{
		shardID:        shardID,
		numberOfShards: numberOfShards,
	}

	fastcgo.UnsafeCall3(
		C.okdb_wal_uni_c_encoder_ctor,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&encoder)),
		uintptr(unsafe.Pointer(err)),
	)
	return CEncoder(unsafe.Pointer(encoder))
}

// CEncoderAdd - add to encode incoming data(ShardedData) through C++ encoder.
func CEncoderAdd(encoder CEncoder, hashdex CHashdex, segment *GoSegment, err *GoErrorInfo) {
	var args = CEncoderAddArgs{
		hashdex: hashdex,
		segment: segment,
	}
	fastcgo.UnsafeCall3(
		C.okdb_wal_uni_c_encoder_add,
		uintptr(encoder),
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(err)),
	)
}

// CEncoderFinalize - finalize the encoded data in the C++ encoder to Segment.
func CEncoderFinalize(encoder CEncoder, segment *GoSegment, err *GoErrorInfo) {
	var args = CEncoderFinalizeArgs{
		segment:   segment,
		redundant: NewGoRedundant(),
	}
	fastcgo.UnsafeCall3(
		C.okdb_wal_uni_c_encoder_finalize,
		uintptr(encoder),
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(err)),
	)
}

// CEncoderDtor - wrapper for destructor CEncoder.
func CEncoderDtor(encoder CEncoder) {
	fastcgo.UnsafeCall1(C.okdb_wal_c_encoder_dtor,
		uintptr(encoder),
	)
}

//
// CHashdex API.
//

// CHashdexPreshardingParams - parameters for presharding CHashdex from protobuf.
type CHashdexPreshardingParams struct {
	protoData *[]byte
	cluster   *CSlice
	replica   *CSlice
}

// CHashdexCtor - wrapper for constructor CHashdex.
func CHashdexCtor(args uintptr) CHashdex {
	var hashdex uintptr
	var error CErrorInfo

	fastcgo.UnsafeCall3(
		C.okdb_wal_uni_c_hashdex_ctor,
		args,
		uintptr(unsafe.Pointer(&hashdex)),
		uintptr(unsafe.Pointer(&error)),
	)

	return CHashdex(uintptr(unsafe.Pointer(hashdex)))
}

// CHashdexPresharding - wrapper for presharding CHashdex from protobuf.
func CHashdexPresharding(hashdex CHashdex, protoData []byte, cluster, replica *CSlice, err *GoErrorInfo) {
	var args = CHashdexPreshardingParams{
		protoData: &protoData,
		cluster:   cluster,
		replica:   replica,
	}

	fastcgo.UnsafeCall3(C.okdb_wal_uni_c_hashdex_presharding,
		uintptr(hashdex),
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(err)),
	)
}

// CHashdexDtor - wrapper for destructor CHashdex.
func CHashdexDtor(hashdex CHashdex) {
	fastcgo.UnsafeCall1(C.okdb_wal_c_hashdex_dtor,
		uintptr(hashdex))
}

//
// CSlice-like objects API
//

// CRedundantDestroy calls C API for destroying GoRedundant's C API.
func CRedundantDestroy(p unsafe.Pointer) {
	fastcgo.UnsafeCall1(C.okdb_wal_c_redundant_destroy,
		uintptr(p))
}

// CDecodedSegmentDestroy calls C API for destroying GoDecodedSegment's C API.
func CDecodedSegmentDestroy(p unsafe.Pointer) {
	fastcgo.UnsafeCall1(C.okdb_wal_c_decoded_segment_destroy,
		uintptr(p))
}

//
// CErrorInfo API
//

// CErrorInfoGetError - wrapper for getter error info.
func CErrorInfoGetError(errinfo CErrorInfo) string {
	return C.GoString(C.c_api_error_info_get_error(errinfo))
}

// CErrorInfoGetStacktrace - wrapper for getter stacktrace.
func CErrorInfoGetStacktrace(errinfo CErrorInfo) string {
	return C.GoString(C.c_api_error_info_get_stacktrace(errinfo))
}

// CErrorInfoDestroy - wrapper for destructor CErrorInfo.
func CErrorInfoDestroy(errinfo CErrorInfo) {
	C.destroy_c_api_error_info(errinfo)
}

//
// C-Memory info API
//

// CMemInfo - get c-memory stat usage.
func CMemInfo(result *GoMemInfoResult) {
	fastcgo.UnsafeCall1(
		C.okdb_wal_c_mem_info,
		uintptr(unsafe.Pointer(result)),
	)
}
