//go:build (!WITHOUT_FASTCGO && !arm64) || WITH_FASTCGO
// +build !WITHOUT_FASTCGO,!arm64 WITH_FASTCGO

// This file contains the C/cgo-dependent parts of GO API using Fastcgo.
// The fastcgo is disabled on arm64, but could be enabled via --tags=WITH_FASTCGO build tag.
package internal

// #cgo CFLAGS: -I.
// #cgo LDFLAGS: -L.
// #cgo arm64 LDFLAGS: -l:aarch64_wal_c_api.a
// #cgo amd64 LDFLAGS: -l:x86_wal_c_api.a
// #cgo LDFLAGS: -lstdc++
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

func CDecoderCtor() CDecoder {
	var decoder unsafe.Pointer = unsafe.Pointer(fastcgo.UnsafeCall0WithRet(C.okdb_wal_c_decoder_ctor))
	return CDecoder(decoder)
}

func CDecoderDecode(decoder CDecoder, segment []byte, protobufResult *GoDecodedSegment) uint32 {
	return uint32(fastcgo.UnsafeCall3WithRet(
		C.okdb_wal_c_decoder_decode,
		uintptr(decoder),
		uintptr((unsafe.Pointer(&segment))),
		uintptr(unsafe.Pointer(protobufResult)),
	))
}

func CDecoderDecodeDry(decoder CDecoder, segment []byte) uint32 {
	return uint32(fastcgo.UnsafeCall2WithRet(
		C.okdb_wal_c_decoder_decode_dry,
		uintptr(decoder),
		uintptr(unsafe.Pointer(&segment)),
	))
}

func CDecoderDecodeSnapshot(decoder CDecoder, snapshot []byte) {
	fastcgo.UnsafeCall2(C.okdb_wal_c_decoder_snapshot,
		uintptr(decoder),
		uintptr((unsafe.Pointer(&snapshot))),
	)
}

func CDecoderDtor(decoder CDecoder) {
	fastcgo.UnsafeCall1(C.okdb_wal_c_decoder_dtor, uintptr(decoder))
}

//
// CEncoder API
//

func CEncoderCtor(shardID, numberOfShards uint16) CEncoder {
	var encoder unsafe.Pointer = unsafe.Pointer(fastcgo.UnsafeCall2WithRet(C.okdb_wal_c_encoder_ctor, uintptr(shardID), uintptr(numberOfShards)))
	return CEncoder(encoder)
}

func CEncoderEncode(encoder CEncoder, hashdex CHashdex, segment *GoSegment, redundant *GoRedundant) {
	fastcgo.UnsafeCall4(
		C.okdb_wal_c_encoder_encode,
		uintptr(encoder),
		uintptr(hashdex),
		uintptr(unsafe.Pointer(segment)),
		uintptr(unsafe.Pointer(redundant)),
	)
}

func CEncoderSnapshot(encoder CEncoder, redundants []unsafe.Pointer, snapshot *GoSnapshot) {
	fastcgo.UnsafeCall3(C.okdb_wal_c_encoder_snapshot,
		uintptr(encoder),
		uintptr(unsafe.Pointer(&redundants)),
		uintptr(unsafe.Pointer(snapshot)),
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

func CHashdexCtor() CHashdex {
	var hashdex unsafe.Pointer = unsafe.Pointer(fastcgo.UnsafeCall0WithRet(C.okdb_wal_c_hashdex_ctor))
	return CHashdex(hashdex)
}

func CHashdexPresharding(hashdex CHashdex, protoData []byte, cluster, replica *CSlice) {
	fastcgo.UnsafeCall4(C.okdb_wal_c_hashdex_presharding,
		uintptr(hashdex),
		uintptr(unsafe.Pointer(&protoData)),
		uintptr(unsafe.Pointer(cluster)),
		uintptr(unsafe.Pointer(replica)),
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
