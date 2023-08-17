//go:build !WITHOUT_FASTCGO || WITH_FASTCGO
// +build !WITHOUT_FASTCGO WITH_FASTCGO

// This file contains the C/cgo-dependent parts of GO API using Fastcgo.
// The fastcgo is disabled on arm64, but could be enabled via --tags=WITH_FASTCGO build tag.
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
// #cgo LDFLAGS: -lstdc++
// #include "wal_c_encoder.h"
// #include "wal_c_decoder.h"
// #include "wal_c_types.h"
// #include <stdlib.h>
import "C"
import (
	"errors"
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

// CByteSlice API
func CSegmentDestroy(p unsafe.Pointer) {
	fastcgo.UnsafeCall1(C.okdb_wal_c_segment_destroy,
		uintptr(p))
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

type CDecoderSnapshotArgs struct {
	snapshot *[]byte
}

func CDecoderCtor(cerr *GoErrorInfo) CDecoder {
	var decoder uintptr = 0
	fastcgo.UnsafeCall2(
		C.okdb_wal_uni_c_decoder_ctor,
		uintptr(unsafe.Pointer(&decoder)),
		uintptr(unsafe.Pointer(cerr)),
	)
	return CDecoder(unsafe.Pointer(decoder))
}

func CDecoderDecode(decoder CDecoder, segment []byte, protobufResult *GoDecodedSegment, cerr *GoErrorInfo) uint32 {
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
		uintptr(unsafe.Pointer(cerr)),
	)

	return result.result
}

func CDecoderDecodeDry(decoder CDecoder, segment []byte, cerr *GoErrorInfo) uint32 {
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
		uintptr(unsafe.Pointer(cerr)),
	)

	return result.result
}

func CDecoderDecodeSnapshot(decoder CDecoder, snapshot []byte, cerr *GoErrorInfo) {
	var args = CDecoderSnapshotArgs{
		snapshot: &snapshot,
	}

	fastcgo.UnsafeCall3(C.okdb_wal_uni_c_decoder_snapshot,
		uintptr(decoder),
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(cerr)),
	)
}

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

type CEncoderSnapshotArgs struct {
	redundants *[]unsafe.Pointer
	snapshot   *GoSnapshot
}

func CEncoderCtor(shardID, numberOfShards uint16, cerr *GoErrorInfo) CEncoder {
	var encoder uintptr = 0
	var args = CEncoderCtorArgs{
		shardID:        shardID,
		numberOfShards: numberOfShards,
	}

	fastcgo.UnsafeCall3(
		C.okdb_wal_uni_c_encoder_ctor,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&encoder)),
		uintptr(unsafe.Pointer(cerr)),
	)
	return CEncoder(unsafe.Pointer(encoder))
}

func CEncoderEncode(encoder CEncoder, hashdex CHashdex, segment *GoSegment, redundant *GoRedundant, cerr *GoErrorInfo) {
	var args = CEncoderEncodeArgs{
		hashdex:   hashdex,
		segment:   segment,
		redundant: redundant,
	}

	fastcgo.UnsafeCall3(
		C.okdb_wal_uni_c_encoder_encode,
		uintptr(encoder),
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(cerr)),
	)
}

// CEncoderAdd - add to encode incoming data(ShardedData) through C++ encoder.
func CEncoderAdd(encoder CEncoder, hashdex CHashdex, segment *GoSegment, cerr *GoErrorInfo) {
	var args = CEncoderAddArgs{
		hashdex: hashdex,
		segment: segment,
	}
	fastcgo.UnsafeCall3(
		C.okdb_wal_uni_c_encoder_add,
		uintptr(encoder),
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(cerr)),
	)
}

// CEncoderFinalize - finalize the encoded data in the C++ encoder to Segment.
func CEncoderFinalize(encoder CEncoder, segment *GoSegment, redundant *GoRedundant, cerr *GoErrorInfo) {
	var args = CEncoderFinalizeArgs{
		segment:   segment,
		redundant: redundant,
	}
	fastcgo.UnsafeCall3(
		C.okdb_wal_uni_c_encoder_finalize,
		uintptr(encoder),
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(cerr)),
	)
}

func CEncoderSnapshot(encoder CEncoder, redundants []unsafe.Pointer, snapshot *GoSnapshot, cerr *GoErrorInfo) {
	var args = CEncoderSnapshotArgs{
		redundants: &redundants,
		snapshot:   snapshot,
	}

	fastcgo.UnsafeCall3(C.okdb_wal_uni_c_encoder_snapshot,
		uintptr(encoder),
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(cerr)),
	)
}

func CEncoderDtor(encoder CEncoder) {
	fastcgo.UnsafeCall1(C.okdb_wal_c_encoder_dtor,
		uintptr(encoder),
	)
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
	var hashdex uintptr = 0
	var error CErrorInfo = nil

	fastcgo.UnsafeCall3(
		C.okdb_wal_uni_c_hashdex_ctor,
		args,
		uintptr(unsafe.Pointer(&hashdex)),
		uintptr(unsafe.Pointer(&error)),
	)

	return CHashdex(uintptr(unsafe.Pointer(hashdex)))
}

func CHashdexPresharding(hashdex CHashdex, protoData []byte, cluster, replica *CSlice, cerr *GoErrorInfo) {
	var args = CHashdexPreshardingParams{
		protoData: &protoData,
		cluster:   cluster,
		replica:   replica,
	}

	fastcgo.UnsafeCall3(C.okdb_wal_uni_c_hashdex_presharding,
		uintptr(hashdex),
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(cerr)),
	)
}

func CHashdexDtor(hashdex CHashdex) {
	fastcgo.UnsafeCall1(C.okdb_wal_c_hashdex_dtor,
		uintptr(hashdex))
}

//
// CSlice-like objects API
//

func CSnapshotDestroy(p unsafe.Pointer) {
	fastcgo.UnsafeCall1(C.okdb_wal_c_snapshot_destroy,
		uintptr(p))
}

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

// CErrorInfo API
func CErrorInfoGetError(errinfo CErrorInfo) error {
	return errors.New(C.GoString(C.c_api_error_info_get_error(errinfo)))
}

func CErrorInfoGetStacktrace(errinfo CErrorInfo) string {
	return C.GoString(C.c_api_error_info_get_stacktrace(errinfo))
}

func CErrorInfoDestroy(errinfo CErrorInfo) {
	C.destroy_c_api_error_info(errinfo)
}
