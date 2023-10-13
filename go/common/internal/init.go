//go:build without_fastcgo && !with_fastcgo
// +build without_fastcgo,!with_fastcgo

// package internal is a Go counterpart of C bindings. It contains the bridged decoder/encoder API.

// This file contains the C/cgo-dependent parts of GO API.
// The usual cgo bindings API is default on arm64, but disabled on amd64,
// but could be enabled via --tags=without_fastcgo build tag.
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
	C.okdb_wal_initialize()
}

// EnableCoreDumps toggles generating coredumps from C++ Exceptions.
// It requres GOTRACEBACK=core env variable for Go runtime.
func EnableCoreDumps(enabled bool) {
	var a int
	if enabled {
		a = 1
	} else {
		a = 0
	}
	C.prompp_enable_coredumps_on_exception(C.int(a))
}

// CByteSlice API
func CSegmentDestroy(p unsafe.Pointer) {
	C.okdb_wal_c_segment_destroy((*C.c_segment)(p))
}

// CEncoderAddManyStateHandle API
func CEncoderAddManyStateHandleDestroy(handle uint64) {
	C.okdb_wal_c_encoder_add_many_state_destroy(C.ulong(handle))
}

//
// CDecoder API
//

type CDecoderDecodeArgs struct {
	segment  *[]byte
	protobuf *GoDecodedSegment
}

type CDecoderDecodeDryArgs struct {
	segment *[]byte
}

type CDecoderDecodeResult struct {
	result uint32
}

// CDecoderDecodeRestoreFromStreamArgs - args for fastCGO RestoreFromStream.
type CDecoderDecodeRestoreFromStreamArgs struct {
	buf            *[]byte
	restoredResult *GoRestoredResult
}

type CDecoderSnapshotArgs struct {
	snapshot *[]byte
}

func CDecoderCtor(err *GoErrorInfo) CDecoder {
	var decoder C.c_decoder_ptr
	C.okdb_wal_uni_c_decoder_ctor(
		&decoder,
		C.ulong(uintptr(unsafe.Pointer(err))),
	)
	return CDecoder(decoder)
}

func CDecoderDecode(decoder CDecoder, segment []byte, protobufResult *GoDecodedSegment, err *GoErrorInfo) uint32 {
	var args = CDecoderDecodeArgs{
		segment:  &segment,
		protobuf: protobufResult,
	}

	var result = CDecoderDecodeResult{
		result: 0,
	}

	C.okdb_wal_uni_c_decoder_decode(
		C.c_decoder(decoder),
		C.ulong(uintptr(unsafe.Pointer(&args))),
		C.ulong(uintptr(unsafe.Pointer(&result))),
		C.ulong(uintptr(unsafe.Pointer(err))),
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

	C.okdb_wal_uni_c_decoder_decode_dry(
		C.c_decoder(decoder),
		C.ulong(uintptr(unsafe.Pointer(&args))),
		C.ulong(uintptr(unsafe.Pointer(&result))),
		C.ulong(uintptr(unsafe.Pointer(err))),
	)

	return result.result
}

// CDecoderRestoreFromStream - GO wrapper for C/C++, calls C++ class Decoder methods.
func CDecoderRestoreFromStream(decoder CDecoder, buf []byte, restoredResult *GoRestoredResult, err *GoErrorInfo) {
	var args = CDecoderDecodeRestoreFromStreamArgs{
		buf:            &buf,
		restoredResult: restoredResult,
	}

	C.okdb_wal_uni_c_decoder_restore_from_stream(
		C.c_decoder(decoder),
		C.ulong(uintptr(unsafe.Pointer(&args))),
		C.ulong(uintptr(unsafe.Pointer(err))),
	)
}

func CDecoderDecodeSnapshot(decoder CDecoder, snapshot []byte, err *GoErrorInfo) {
	var args = CDecoderSnapshotArgs{
		snapshot: &snapshot,
	}

	C.okdb_wal_uni_c_decoder_snapshot(
		C.c_decoder(decoder),
		C.ulong(uintptr(unsafe.Pointer(&args))),
		C.ulong(uintptr(unsafe.Pointer(err))),
	)
}

func CDecoderDtor(decoder CDecoder) {
	C.okdb_wal_c_decoder_dtor(C.c_decoder(decoder))
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

type CEncoderSnapshotArgs struct {
	redundants *[]unsafe.Pointer
	snapshot   *GoSnapshot
}

func CEncoderCtor(shardID, numberOfShards uint16, err *GoErrorInfo) CEncoder {
	var encoder C.c_encoder_ptr
	var args = CEncoderCtorArgs{
		shardID:        shardID,
		numberOfShards: numberOfShards,
	}
	C.okdb_wal_uni_c_encoder_ctor(
		C.ulong(uintptr(unsafe.Pointer(&args))),
		C.ulong(uintptr(unsafe.Pointer(&encoder))),
		C.ulong(uintptr(unsafe.Pointer(err))),
	)
	return CEncoder(encoder)
}

func CEncoderEncode(encoder CEncoder, hashdex CHashdex, segment *GoSegment, redundant *GoRedundant, err *GoErrorInfo) {
	var args = CEncoderEncodeArgs{
		hashdex:   hashdex,
		segment:   segment,
		redundant: redundant,
	}
	C.okdb_wal_uni_c_encoder_encode(
		C.ulong(uintptr(encoder)),
		C.ulong(uintptr(unsafe.Pointer(&args))),
		C.ulong(uintptr(unsafe.Pointer(err))),
	)
}

// CEncoderAdd - add to encode incoming data(ShardedData) through C++ encoder.
func CEncoderAdd(encoder CEncoder, hashdex CHashdex, segment *GoSegment, err *GoErrorInfo) {
	var args = CEncoderAddArgs{
		hashdex: hashdex,
		segment: segment,
	}
	C.okdb_wal_uni_c_encoder_add(
		C.c_encoder(encoder),
		C.ulong(uintptr(unsafe.Pointer(&args))),
		C.ulong(uintptr(unsafe.Pointer(err))),
	)
}

// CEncoderFinalize - finalize the encoded data in the C++ encoder to Segment.
func CEncoderFinalize(encoder CEncoder, segment *GoSegment, redundant *GoRedundant, err *GoErrorInfo) {
	var args = CEncoderFinalizeArgs{
		segment:   segment,
		redundant: redundant,
	}
	C.okdb_wal_uni_c_encoder_finalize(
		C.c_encoder_ptr(encoder),
		C.ulong(uintptr(unsafe.Pointer(&args))),
		C.ulong(uintptr(unsafe.Pointer(err))),
	)
}

func CEncoderSnapshot(encoder CEncoder, redundants []unsafe.Pointer, snapshot *GoSnapshot, err *GoErrorInfo) {
	var args = CEncoderSnapshotArgs{
		redundants: &redundants,
		snapshot:   snapshot,
	}

	C.okdb_wal_uni_c_encoder_snapshot(
		C.c_encoder(encoder),
		C.ulong(uintptr(unsafe.Pointer(&args))),
		C.ulong(uintptr(unsafe.Pointer(err))),
	)
}

func CEncoderDtor(encoder CEncoder) {
	C.okdb_wal_c_encoder_dtor(C.c_encoder(encoder))
}

//
// CHashdex API.
//

type CHashdexPreshardingParams struct {
	protoData *[]byte
	cluster   *CSlice
	replica   *CSlice
}

func CHashdexCtor(args uintptr) CHashdex {
	return CHashdex(C.okdb_wal_c_hashdex_ctor(C.ulong(args)))
}

func CHashdexPresharding(hashdex CHashdex, protoData []byte, cluster, replica *CSlice, err *GoErrorInfo) {
	var args = CHashdexPreshardingParams{
		protoData: &protoData,
		cluster:   cluster,
		replica:   replica,
	}
	C.okdb_wal_uni_c_hashdex_presharding(
		C.c_hashdex(hashdex),
		C.ulong(uintptr(unsafe.Pointer(&args))),
		C.ulong(uintptr(unsafe.Pointer(err))),
	)
}

func CHashdexDtor(hashdex CHashdex) {
	C.okdb_wal_c_hashdex_dtor(C.c_hashdex(hashdex))
}

//
// CSlice API
//

func CSnapshotDestroy(p unsafe.Pointer) {
	C.okdb_wal_c_snapshot_destroy((*C.c_snapshot)(p))
}

// CRedundantDestroy calls C API for destroying GoRedundant's C API.
func CRedundantDestroy(p unsafe.Pointer) {
	C.okdb_wal_c_redundant_destroy((*C.c_redundant)(p))
}

// CDecodedSegmentDestroy calls C API for destroying GoDecodedSegment's C API.
func CDecodedSegmentDestroy(p unsafe.Pointer) {
	C.okdb_wal_c_decoded_segment_destroy((*C.c_decoded_segment)(p))
}

// CErrorInfo API
func CErrorInfoGetError(errinfo CErrorInfo) string {
	return C.GoString(C.c_api_error_info_get_error(errinfo))
}

func CErrorInfoGetStacktrace(errinfo CErrorInfo) string {
	return C.GoString(C.c_api_error_info_get_stacktrace(errinfo))
}

func CErrorInfoDestroy(errinfo CErrorInfo) {
	C.destroy_c_api_error_info(errinfo)
}

//
// C-Memory info API
//

// CMemInfo - get c-memory stat usage.
func CMemInfo(result *GoMemInfoResult) {
	C.okdb_wal_c_mem_info(C.ulong(uintptr(unsafe.Pointer(result))))
}
