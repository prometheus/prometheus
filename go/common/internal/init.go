// package internal is a Go counterpart of C bindings. It contains the bridged decoder/encoder API.
// This file contains the C/fastcgo-dependent parts of GO API.
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
import "unsafe"

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
	C.okdb_wal_initialize()
}

// CByteSlice API
func CSliceByteDestroy(p unsafe.Pointer) {
	C.okdb_wal_c_slice_with_string_buffer_destroy((*C.c_slice_with_string_buffer)(p))
}

//
// CDecoder API
//

func CDecoderCtor() CDecoder {
	return CDecoder(C.okdb_wal_c_decoder_ctor())
}

func CDecoderDecode(decoder CDecoder, segment []byte, protobufResult *GoSliceByte) uint32 {
	return uint32(C.okdb_wal_c_decoder_decode(
		C.c_decoder(decoder),
		*(*C.c_slice)(unsafe.Pointer(&segment)),
		(*C.c_slice_with_string_buffer)(unsafe.Pointer(protobufResult)),
	))
}

func CDecoderDecodeDry(decoder CDecoder, segment []byte) uint32 {
	return uint32(C.okdb_wal_c_decoder_decode_dry(
		C.c_decoder(decoder),
		*(*C.c_slice)(unsafe.Pointer(&segment)),
	))
}

func CDecoderDecodeSnapshot(decoder CDecoder, snapshot []byte) {
	C.okdb_wal_c_decoder_snapshot(
		C.c_decoder(decoder),
		*(*C.c_slice)(unsafe.Pointer(&snapshot)),
	)
}

func CDecoderDtor(decoder CDecoder) {
	C.okdb_wal_c_decoder_dtor(C.c_decoder(decoder))
}

//
// CEncoder API
//

func CEncoderCtor(shardID, numberOfShards uint16) CEncoder {
	return CEncoder(C.okdb_wal_c_encoder_ctor(C.uint16_t(shardID), C.uint16_t(numberOfShards)))
}

func CEncoderEncode(encoder CEncoder, hashdex CHashdex, segment *GoSegment, redundant *GoRedundant) {
	C.okdb_wal_c_encoder_encode(
		C.c_encoder(encoder),
		C.c_hashdex(hashdex),
		(*C.c_slice_with_stream_buffer)(unsafe.Pointer(segment)),
		(*C.c_redundant)(unsafe.Pointer(redundant)),
	)
}

func CEncoderSnapshot(encoder CEncoder, redundants []unsafe.Pointer, snapshot *GoSnapshot) {
	C.okdb_wal_c_encoder_snapshot(
		C.c_encoder(encoder),
		*(*C.c_slice)(unsafe.Pointer(&redundants)),
		(*C.c_slice_with_stream_buffer)(unsafe.Pointer(snapshot)),
	)
}

func CEncoderDtor(encoder CEncoder) {
	C.okdb_wal_c_encoder_dtor(C.c_encoder(encoder))
}

//
// CHashdex API.
//

func CHashdexCtor() CHashdex {
	return CHashdex(C.okdb_wal_c_hashdex_ctor())
}

func CHashdexPresharding(hashdex CHashdex, protoData []byte) {
	C.okdb_wal_c_hashdex_presharding(
		C.c_hashdex(hashdex),
		*(*C.c_slice)(unsafe.Pointer(&protoData)),
	)
}

func CHashdexDtor(hashdex CHashdex) {
	C.okdb_wal_c_hashdex_dtor(C.c_hashdex(hashdex))
}

//
// CSlice API
//

func CSliceWithStreamBufferDestroy(p unsafe.Pointer) {
	C.okdb_wal_c_slice_with_stream_buffer_destroy((*C.c_slice_with_stream_buffer)(p))
}

// CRedundantDestroy calls C API for destroying GoRedunant's C API.
func CRedundantDestroy(p unsafe.Pointer) {
	C.okdb_wal_c_redundant_destroy((*C.c_redundant)(p))
}
