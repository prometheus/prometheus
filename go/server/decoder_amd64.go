package server

// #cgo LDFLAGS: -L ../common/wal_c_bindings -l:x86_wal_c_api.a
// #cgo LDFLAGS: -lstdc++
// #include <stdlib.h>
// #include "../common/wal_c_decoder.h"
import "C" // nolint
import (
	"unsafe" // nolint
)

type cDecoder C.c_decoder

func cDecoderCtor() cDecoder {
	return cDecoder(C.okdb_wal_c_decoder_ctor())
}

func cDecoderDecode(decoder cDecoder, segment []byte, protobufResult *GoSliceByte) uint32 {
	return uint32(C.okdb_wal_c_decoder_decode(
		C.c_decoder(decoder),
		*(*C.c_slice)(unsafe.Pointer(&segment)),
		(*C.c_slice_with_string_buffer)(unsafe.Pointer(protobufResult)),
	))
}

func cDecoderDecodeDry(decoder cDecoder, segment []byte) uint32 {
	return uint32(C.okdb_wal_c_decoder_decode_dry(
		C.c_decoder(decoder),
		*(*C.c_slice)(unsafe.Pointer(&segment)),
	))
}

func cDecoderDecodeSnapshot(decoder cDecoder, snapshot []byte) {
	C.okdb_wal_c_decoder_snapshot(
		C.c_decoder(decoder),
		*(*C.c_slice)(unsafe.Pointer(&snapshot)),
	)
}

func cDecoderDtor(decoder cDecoder) {
	C.okdb_wal_c_decoder_dtor(C.c_decoder(decoder))
}

// GoSliceByte - GO wrapper for Slice byte, init from GO and filling from C/C++.
// data - slice struct for cast in C/C++. Contained C/C++ memory
// buf - unsafe.Pointer for stringstream in C/C++, need for clear memory.
type GoSliceByte struct {
	data C.c_slice
	buf  unsafe.Pointer
}

// NewGoSliceByte - init GoSliceByte.
func NewGoSliceByte() *GoSliceByte {
	return &GoSliceByte{
		data: C.c_slice{},
	}
}

// Bytes - convert in go-slice byte from struct.
func (gs *GoSliceByte) Bytes() []byte {
	return *(*[]byte)((unsafe.Pointer(&gs.data)))
}

// Destroy - clear memory in C/C++.
func (gs *GoSliceByte) Destroy() {
	C.okdb_wal_c_slice_with_string_buffer_destroy((*C.c_slice_with_string_buffer)(unsafe.Pointer(gs)))
}
