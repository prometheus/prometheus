package cppbridge

// #cgo CFLAGS: -I.
// #cgo LDFLAGS: -L.
// #cgo sanitize LDFLAGS: -fsanitize=address
// #cgo sanitize CFLAGS: -fsanitize=address
// #cgo arm64,!sanitize,!dbg LDFLAGS: -l:arm64_entrypoint_opt.a
// #cgo arm64,!sanitize,dbg LDFLAGS: -l:arm64_entrypoint_dbg.a
// #cgo arm64,sanitize,!dbg LDFLAGS: -l:arm64_entrypoint_opt_asan.a
// #cgo arm64,sanitize,dbg LDFLAGS: -l:arm64_entrypoint_dbg_asan.a
// #cgo amd64,!sanitize,!dbg LDFLAGS: -l:amd64_entrypoint_opt.a
// #cgo amd64,!sanitize,dbg LDFLAGS: -l:amd64_entrypoint_dbg.a
// #cgo amd64,sanitize,!dbg LDFLAGS: -l:amd64_entrypoint_opt_asan.a
// #cgo amd64,sanitize,dbg LDFLAGS: -l:amd64_entrypoint_dbg_asan.a
// #cgo LDFLAGS: -static-libgcc -static-libstdc++ -l:libstdc++.a
// #cgo static LDFLAGS: -static
// #include "entrypoint.h"
import "C" //nolint:gocritic // because otherwise it won't work
import (
	"runtime"
	"unsafe" //nolint:gocritic // because otherwise it won't work

	"github.com/prometheus/prometheus/pp/go/cppbridge/fastcgo"
)

func init() {
	fastcgo.UnsafeCall0(C.prompp_init)
}

func freeBytes(b []byte) {
	fastcgo.UnsafeCall1(
		C.prompp_free_bytes,
		uintptr(unsafe.Pointer(&b)),
	)
	runtime.KeepAlive(b)
}

func getFlavor() string {
	var res struct {
		flavor string
	}
	fastcgo.UnsafeCall1(
		C.prompp_get_flavor,
		uintptr(unsafe.Pointer(&res)),
	)
	return res.flavor
}

func memInfo() (res MemInfo) {
	fastcgo.UnsafeCall1(
		C.prompp_mem_info,
		uintptr(unsafe.Pointer(&res)),
	)
	return res
}

func walHashdexCtor(limits HashdexLimits) uintptr {
	var res struct {
		hashdex uintptr
	}

	fastcgo.UnsafeCall2(
		C.prompp_wal_hashdex_ctor,
		uintptr(unsafe.Pointer(&limits)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.hashdex
}

func walHashdexDtor(hashdex uintptr) {
	var args = struct {
		hashdex uintptr
	}{hashdex}

	fastcgo.UnsafeCall1(
		C.prompp_wal_hashdex_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

func walHashdexPresharding(hashdex uintptr, protobuf []byte) (cluster, replica string, err []byte) {
	var args = struct {
		hashdex  uintptr
		protobuf []byte
	}{hashdex, protobuf}
	var res struct {
		cluster   string
		replica   string
		exception []byte
	}

	fastcgo.UnsafeCall2(
		C.prompp_wal_hashdex_presharding,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.cluster, res.replica, res.exception
}

//
// Encoder
//

// walEncoderCtor - wrapper for constructor C-Encoder.
func walEncoderCtor(shardID uint16, logShards uint8) uintptr {
	var args = struct {
		shardID   uint16
		logShards uint8
	}{shardID, logShards}
	var res struct {
		encoder uintptr
	}

	fastcgo.UnsafeCall2(
		C.prompp_wal_encoder_ctor,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.encoder
}

// walEncoderAdd - add to encode incoming data(ShardedData) through C++ encoder.
func walEncoderAdd(encoder, hashdex uintptr) (stats WALEncoderStats, exception []byte) {
	var args = struct {
		encoder uintptr
		hashdex uintptr
	}{encoder, hashdex}
	var res struct {
		WALEncoderStats
		exception []byte
	}

	fastcgo.UnsafeCall2(
		C.prompp_wal_encoder_add,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.WALEncoderStats, res.exception
}

// walEncoderFinalize - finalize the encoded data in the C++ encoder to Segment.
func walEncoderFinalize(encoder uintptr) (stats WALEncoderStats, segment, exception []byte) {
	var args = struct {
		encoder uintptr
	}{encoder}
	var res struct {
		WALEncoderStats
		segment   []byte
		exception []byte
	}

	fastcgo.UnsafeCall2(
		C.prompp_wal_encoder_finalize,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.WALEncoderStats, res.segment, res.exception
}

// walEncoderAddWithStaleNans - add to encode incoming data(ShardedData)
// to current segment and mark as stale obsolete series through C++ encoder.
func walEncoderAddWithStaleNans(
	encoder, hashdex, sourceState uintptr,
	staleTS int64,
) (stats WALEncoderStats, state uintptr, exception []byte) {
	var args = struct {
		encoder     uintptr
		hashdex     uintptr
		staleTS     int64
		sourceState uintptr
	}{encoder, hashdex, staleTS, sourceState}
	var res struct {
		WALEncoderStats
		sourceState uintptr
		exception   []byte
	}

	fastcgo.UnsafeCall2(
		C.prompp_wal_encoder_add_with_stale_nans,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.WALEncoderStats, res.sourceState, res.exception
}

// walEncoderCollectSource - destroy source state and mark all series as stale.
func walEncoderCollectSource(encoder, sourceState uintptr, staleTS int64) (stats WALEncoderStats, exception []byte) {
	var args = struct {
		encoder     uintptr
		staleTS     int64
		sourceState uintptr
	}{encoder, staleTS, sourceState}
	var res struct {
		WALEncoderStats
		exception []byte
	}

	fastcgo.UnsafeCall2(
		C.prompp_wal_encoder_collect_source,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.WALEncoderStats, res.exception
}

// walEncoderDtor - wrapper for destructor C-Encoder.
func walEncoderDtor(encoder uintptr) {
	var args = struct {
		encoder uintptr
	}{encoder}

	fastcgo.UnsafeCall1(
		C.prompp_wal_encoder_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

//
// Decoder
//

// walDecoderCtor - wrapper for constructor C-Decoder.
func walDecoderCtor() uintptr {
	var res struct {
		decoder uintptr
	}

	fastcgo.UnsafeCall1(
		C.prompp_wal_decoder_ctor,
		uintptr(unsafe.Pointer(&res)),
	)

	return res.decoder
}

// walDecoderDecode - decode WAL-segment into protobuf message through C++ decoder.
func walDecoderDecode(decoder uintptr, segment []byte) (stats DecodedSegmentStats, protobuf, err []byte) {
	var args = struct {
		decoder uintptr
		segment []byte
	}{decoder, segment}
	var res struct {
		DecodedSegmentStats
		protobuf []byte
		error    []byte
	}

	fastcgo.UnsafeCall2(
		C.prompp_wal_decoder_decode,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.DecodedSegmentStats, res.protobuf, res.error
}

// decoderDecode - decode WAL-segment and drop decoded data through C++ decoder.
func walDecoderDecodeDry(decoder uintptr, segment []byte) (err []byte) {
	var args = struct {
		decoder uintptr
		segment []byte
	}{decoder, segment}
	var res struct {
		error []byte
	}

	fastcgo.UnsafeCall2(
		C.prompp_wal_decoder_decode_dry,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.error
}

// decoderDecode - decode all segments from given stream dump through C++ decoder.
func walDecoderRestoreFromStream(
	decoder uintptr,
	segment []byte,
	segmentID uint32,
) (offset uint64, rSegmentID uint32, err []byte) {
	var args = struct {
		decoder   uintptr
		segment   []byte
		segmentID uint32
	}{decoder, segment, segmentID}
	var res struct {
		offset    uint64
		segmentID uint32
		error     []byte
	}

	fastcgo.UnsafeCall2(
		C.prompp_wal_decoder_restore_from_stream,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.offset, res.segmentID, res.error
}

// walDecoderDtor - wrapper for destructor C-Decoder.
func walDecoderDtor(decoder uintptr) {
	var args = struct {
		decoder uintptr
	}{decoder}

	fastcgo.UnsafeCall1(
		C.prompp_wal_decoder_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}
