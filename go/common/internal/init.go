//go:build WITHOUT_FASTCGO && !WITH_FASTCGO
// +build WITHOUT_FASTCGO,!WITH_FASTCGO

// package internal is a Go counterpart of C bindings. It contains the bridged decoder/encoder API.

// This file contains the C/cgo-dependent parts of GO API.
// The usual cgo bindings API is default on arm64, but disabled on amd64,
// but could be enabled via --tags=WITHOUT_FASTCGO build tag.
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

// CByteSlice API
func CSegmentDestroy(p unsafe.Pointer) {
	C.okdb_wal_c_segment_destroy((*C.c_segment)(p))
}

//
// CDecoder API
//

func CDecoderCtor(*GoErrorInfo) CDecoder {
	return CDecoder(C.okdb_wal_c_decoder_ctor())
}

func CDecoderDecode(decoder CDecoder, segment []byte, result *GoDecodedSegment, err *GoErrorInfo) uint32 {
	_ = err
	return uint32(C.okdb_wal_c_decoder_decode(
		C.c_decoder(decoder),
		C.size_t(uintptr((unsafe.Pointer(&segment)))),
		(*C.c_decoded_segment)(unsafe.Pointer(result)),
	))
}

func CDecoderDecodeDry(decoder CDecoder, segment []byte, err *GoErrorInfo) uint32 {
	_ = err
	return uint32(C.okdb_wal_c_decoder_decode_dry(
		C.c_decoder(decoder),
		C.size_t(uintptr((unsafe.Pointer(&segment)))),
	))
}

func CDecoderDecodeSnapshot(decoder CDecoder, snapshot []byte, err *GoErrorInfo) {
	_ = err
	C.okdb_wal_c_decoder_snapshot(
		C.c_decoder(decoder),
		C.size_t(uintptr((unsafe.Pointer(&snapshot)))),
	)
}

func CDecoderDtor(decoder CDecoder) {
	C.okdb_wal_c_decoder_dtor(C.c_decoder(decoder))
}

//
// CEncoder API
//

func CEncoderCtor(shardID, numberOfShards uint16, err *GoErrorInfo) CEncoder {
	_ = err
	return CEncoder(C.okdb_wal_c_encoder_ctor(C.uint16_t(shardID), C.uint16_t(numberOfShards)))
}

func CEncoderEncode(encoder CEncoder, hashdex CHashdex, segment *GoSegment, redundant *GoRedundant, err *GoErrorInfo) {
	_ = err
	C.okdb_wal_c_encoder_encode(
		C.c_encoder(encoder),
		C.c_hashdex(hashdex),
		(*C.c_segment)(unsafe.Pointer(segment)),
		(*C.c_redundant)(unsafe.Pointer(redundant)),
	)
}

// CEncoderAdd - add to encode incoming data(ShardedData) through C++ encoder.
func CEncoderAdd(encoder CEncoder, hashdex CHashdex, segment *GoSegment, err *GoErrorInfo) {
	_ = err
	C.okdb_wal_c_encoder_add(
		C.c_encoder(encoder),
		C.c_hashdex(hashdex),
		(*C.c_segment)(unsafe.Pointer(segment)),
	)
}

// CEncoderFinalize - finalize the encoded data in the C++ encoder to Segment.
func CEncoderFinalize(encoder CEncoder, segment *GoSegment, redundant *GoRedundant, err *GoErrorInfo) {
	_ = err
	C.okdb_wal_c_encoder_finalize(
		C.c_encoder(encoder),
		(*C.c_segment)(unsafe.Pointer(segment)),
		(*C.c_redundant)(unsafe.Pointer(redundant)),
	)
}

func CEncoderSnapshot(encoder CEncoder, redundants []unsafe.Pointer, snapshot *GoSnapshot, err *GoErrorInfo) {
	_ = err
	C.okdb_wal_c_encoder_snapshot(
		C.c_encoder(encoder),
		C.size_t(uintptr((unsafe.Pointer(&redundants)))),
		(*C.c_snapshot)(unsafe.Pointer(snapshot)),
	)
}

func CEncoderDtor(encoder CEncoder) {
	C.okdb_wal_c_encoder_dtor(C.c_encoder(encoder))
}

//
// CHashdex API.
//

func CHashdexCtor(args uintptr) CHashdex {
	return CHashdex(C.okdb_wal_c_hashdex_ctor(C.ulong(args)))
}

func CHashdexPresharding(hashdex CHashdex, protoData []byte, cluster, replica *CSlice, err *GoErrorInfo) {
	C.okdb_wal_c_hashdex_presharding(
		C.c_hashdex(hashdex),
		C.size_t(uintptr(unsafe.Pointer(&protoData))),
		C.size_t(uintptr(unsafe.Pointer(cluster))),
		C.size_t(uintptr(unsafe.Pointer(replica))),
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
func CErrorInfoGetError(errinfo CErrorInfo) error {
	return errors.New(C.GoString(C.c_api_error_info_get_error(errinfo)))
}

func CErrorInfoGetStacktrace(errinfo CErrorInfo) string {
	return C.GoString(C.c_api_error_info_get_stacktrace(errinfo))
}

func CErrorInfoDestroy(errinfo CErrorInfo) {
	C.destroy_c_api_error_info(errinfo)
}
