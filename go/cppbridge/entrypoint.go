package cppbridge

// #cgo CFLAGS: -I.
// #cgo LDFLAGS: -L.
// #cgo sanitize LDFLAGS: -fsanitize=address
// #cgo sanitize CFLAGS: -fsanitize=address
// #cgo arm64,!sanitize,!dbg LDFLAGS: -l:arm64_entrypoint_init_aio_opt.a
// #cgo arm64,!sanitize,!dbg LDFLAGS: -l:arm64_armv8_a_entrypoint_aio_prefixed_opt.a
// #cgo arm64,!sanitize,!dbg LDFLAGS: -l:arm64_armv8_a_crc_entrypoint_aio_prefixed_opt.a
// #cgo arm64,!sanitize,dbg LDFLAGS: -l:arm64_entrypoint_init_aio_dbg.a
// #cgo arm64,!sanitize,dbg LDFLAGS: -l:arm64_armv8_a_entrypoint_aio_prefixed_dbg.a
// #cgo arm64,!sanitize,dbg LDFLAGS: -l:arm64_armv8_a_crc_entrypoint_aio_prefixed_dbg.a
// #cgo arm64,sanitize,!dbg LDFLAGS: -l:arm64_entrypoint_init_aio_opt_asan.a
// #cgo arm64,sanitize,!dbg LDFLAGS: -l:arm64_armv8_a_entrypoint_aio_prefixed_opt_asan.a
// #cgo arm64,sanitize,!dbg LDFLAGS: -l:arm64_armv8_a_crc_entrypoint_aio_prefixed_opt_asan.a
// #cgo arm64,sanitize,dbg LDFLAGS: -l:arm64_entrypoint_init_aio_dbg_asan.a
// #cgo arm64,sanitize,dbg LDFLAGS: -l:arm64_armv8_a_entrypoint_aio_prefixed_dbg_asan.a
// #cgo arm64,sanitize,dbg LDFLAGS: -l:arm64_armv8_a_crc_entrypoint_aio_prefixed_dbg_asan.a
// #cgo amd64,!sanitize,!dbg LDFLAGS: -l:amd64_entrypoint_init_aio_opt.a
// #cgo amd64,!sanitize,!dbg LDFLAGS: -l:amd64_k8_entrypoint_aio_prefixed_opt.a
// #cgo amd64,!sanitize,!dbg LDFLAGS: -l:amd64_nehalem_entrypoint_aio_prefixed_opt.a
// #cgo amd64,!sanitize,!dbg LDFLAGS: -l:amd64_haswell_entrypoint_aio_prefixed_opt.a
// #cgo amd64,!sanitize,dbg LDFLAGS: -l:amd64_entrypoint_init_aio_dbg.a
// #cgo amd64,!sanitize,dbg LDFLAGS: -l:amd64_k8_entrypoint_aio_prefixed_dbg.a
// #cgo amd64,!sanitize,dbg LDFLAGS: -l:amd64_nehalem_entrypoint_aio_prefixed_dbg.a
// #cgo amd64,!sanitize,dbg LDFLAGS: -l:amd64_haswell_entrypoint_aio_prefixed_dbg.a
// #cgo amd64,sanitize,!dbg LDFLAGS: -l:amd64_entrypoint_init_aio_opt_asan.a
// #cgo amd64,sanitize,!dbg LDFLAGS: -l:amd64_k8_entrypoint_aio_prefixed_opt_asan.a
// #cgo amd64,sanitize,!dbg LDFLAGS: -l:amd64_nehalem_entrypoint_aio_prefixed_opt_asan.a
// #cgo amd64,sanitize,!dbg LDFLAGS: -l:amd64_haswell_entrypoint_aio_prefixed_opt_asan.a
// #cgo amd64,sanitize,dbg LDFLAGS: -l:amd64_entrypoint_init_aio_dbg_asan.a
// #cgo amd64,sanitize,dbg LDFLAGS: -l:amd64_k8_entrypoint_aio_prefixed_dbg_asan.a
// #cgo amd64,sanitize,dbg LDFLAGS: -l:amd64_nehalem_entrypoint_aio_prefixed_dbg_asan.a
// #cgo amd64,sanitize,dbg LDFLAGS: -l:amd64_haswell_entrypoint_aio_prefixed_dbg_asan.a
// #cgo LDFLAGS: -static-libgcc -static-libstdc++ -l:libstdc++.a -l:libm.a
// #cgo static LDFLAGS: -static
// #include "entrypoint.h"
import "C" //nolint:gocritic // because otherwise it won't work
import (
	"runtime"
	"unsafe" //nolint:gocritic // because otherwise it won't work

	"github.com/prometheus/prometheus/pp/go/cppbridge/fastcgo"
	"github.com/prometheus/prometheus/pp/go/model"
)

func freeBytes(b []byte) {
	fastcgo.UnsafeCall1(
		C.prompp_free_bytes,
		uintptr(unsafe.Pointer(&b)),
	)
	runtime.KeepAlive(b)
}

// GetFlavor returns recognized architecture flavor
//
//revive:disable:confusing-naming // wrapper
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

func walProtobufHashdexCtor(limits WALHashdexLimits) uintptr {
	var res struct {
		hashdex uintptr
	}

	fastcgo.UnsafeCall2(
		C.prompp_wal_protobuf_hashdex_ctor,
		uintptr(unsafe.Pointer(&limits)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.hashdex
}

func walProtobufHashdexDtor(hashdex uintptr) {
	var args = struct {
		hashdex uintptr
	}{hashdex}

	fastcgo.UnsafeCall1(
		C.prompp_wal_protobuf_hashdex_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

func walProtobufHashdexPresharding(hashdex uintptr, protobuf []byte) (cluster, replica string, err []byte) {
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
		C.prompp_wal_protobuf_hashdex_presharding,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.cluster, res.replica, res.exception
}

func walGoModelHashdexCtor(limits WALHashdexLimits) uintptr {
	var res struct {
		hashdex uintptr
	}

	fastcgo.UnsafeCall2(
		C.prompp_wal_go_model_hashdex_ctor,
		uintptr(unsafe.Pointer(&limits)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.hashdex
}

func walGoModelHashdexDtor(hashdex uintptr) {
	var args = struct {
		hashdex uintptr
	}{hashdex}

	fastcgo.UnsafeCall1(
		C.prompp_wal_go_model_hashdex_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

func walGoModelHashdexPresharding(hashdex uintptr, data []model.TimeSeries) (cluster, replica string, err []byte) {
	var args = struct {
		hashdex uintptr
		data    []model.TimeSeries
	}{hashdex, data}
	var res struct {
		cluster   string
		replica   string
		exception []byte
	}

	fastcgo.UnsafeCall2(
		C.prompp_wal_go_model_hashdex_presharding,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.cluster, res.replica, res.exception
}

//
// Encoder
//

// walEncodersVersion - return current encoders version.
func walEncodersVersion() uint8 {
	var res struct {
		encoders_version uint8
	}

	fastcgo.UnsafeCall1(
		C.prompp_wal_encoders_version,
		uintptr(unsafe.Pointer(&res)),
	)

	return res.encoders_version
}

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

func walEncoderAddInnerSeries(encoder uintptr, innerSeries []*InnerSeries) (stats WALEncoderStats, exception []byte) {
	var args = struct {
		innerSeries []*InnerSeries
		encoder     uintptr
	}{innerSeries, encoder}
	var res struct {
		WALEncoderStats
		exception []byte
	}

	fastcgo.UnsafeCall2(
		C.prompp_wal_encoder_add_inner_series,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.WALEncoderStats, res.exception
}

func walEncoderAddRelabeledSeries(
	encoder uintptr,
	relabeledSeries *RelabeledSeries,
	relabelerStateUpdate *RelabelerStateUpdate,
) (stats WALEncoderStats, exception []byte) {
	var args = struct {
		relabelerStateUpdate *RelabelerStateUpdate
		relabeledSeries      *RelabeledSeries
		encoder              uintptr
	}{relabelerStateUpdate, relabeledSeries, encoder}
	var res struct {
		WALEncoderStats
		exception []byte
	}

	fastcgo.UnsafeCall2(
		C.prompp_wal_encoder_add_relabeled_series,
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
// EncoderLightweight
//

// walEncoderLightweightCtor - wrapper for constructor C-EncoderLightweight.
func walEncoderLightweightCtor(shardID uint16, logShards uint8) uintptr {
	var args = struct {
		shardID   uint16
		logShards uint8
	}{shardID, logShards}
	var res struct {
		encoder uintptr
	}

	fastcgo.UnsafeCall2(
		C.prompp_wal_encoder_lightweight_ctor,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.encoder
}

// walEncoderLightweightAdd - add to encode incoming data(ShardedData) through C++ EncoderLightweight.
func walEncoderLightweightAdd(encoder, hashdex uintptr) (stats WALEncoderStats, exception []byte) {
	var args = struct {
		encoder uintptr
		hashdex uintptr
	}{encoder, hashdex}
	var res struct {
		WALEncoderStats
		exception []byte
	}

	fastcgo.UnsafeCall2(
		C.prompp_wal_encoder_lightweight_add,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.WALEncoderStats, res.exception
}

// walEncoderLightweightAddInnerSeries - add inner series to current segment.
func walEncoderLightweightAddInnerSeries(
	encoder uintptr,
	innerSeries []*InnerSeries,
) (stats WALEncoderStats, exception []byte) {
	var args = struct {
		innerSeries []*InnerSeries
		encoder     uintptr
	}{innerSeries, encoder}
	var res struct {
		WALEncoderStats
		exception []byte
	}

	fastcgo.UnsafeCall2(
		C.prompp_wal_encoder_lightweight_add_inner_series,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.WALEncoderStats, res.exception
}

// walEncoderLightweightAddRelabeledSeries - add relabeled series to current segment.
func walEncoderLightweightAddRelabeledSeries(
	encoder uintptr,
	relabeledSeries *RelabeledSeries,
	relabelerStateUpdate *RelabelerStateUpdate,
) (stats WALEncoderStats, exception []byte) {
	var args = struct {
		relabelerStateUpdate *RelabelerStateUpdate
		relabeledSeries      *RelabeledSeries
		encoder              uintptr
	}{relabelerStateUpdate, relabeledSeries, encoder}
	var res struct {
		WALEncoderStats
		exception []byte
	}

	fastcgo.UnsafeCall2(
		C.prompp_wal_encoder_lightweight_add_relabeled_series,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.WALEncoderStats, res.exception
}

// walEncoderLightweightFinalize - finalize the encoded data in the C++ EncoderLightweight to Segment.
func walEncoderLightweightFinalize(encoder uintptr) (stats WALEncoderStats, segment, exception []byte) {
	var args = struct {
		encoder uintptr
	}{encoder}
	var res struct {
		WALEncoderStats
		segment   []byte
		exception []byte
	}

	fastcgo.UnsafeCall2(
		C.prompp_wal_encoder_lightweight_finalize,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.WALEncoderStats, res.segment, res.exception
}

// walEncoderLightweightDtor - wrapper for destructor C-EncoderLightweight.
func walEncoderLightweightDtor(encoder uintptr) {
	var args = struct {
		encoder uintptr
	}{encoder}

	fastcgo.UnsafeCall1(
		C.prompp_wal_encoder_lightweight_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

//
// Decoder
//

// walDecoderCtor - wrapper for constructor C-Decoder.
func walDecoderCtor(encodersVersion uint8) uintptr {
	var args = struct {
		encoder_version uint8
	}{encodersVersion}
	var res struct {
		decoder uintptr
	}

	fastcgo.UnsafeCall2(
		C.prompp_wal_decoder_ctor,
		uintptr(unsafe.Pointer(&args)),
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
func walDecoderDecodeDry(decoder uintptr, segment []byte) (segmentID uint32, err []byte) {
	var args = struct {
		decoder uintptr
		segment []byte
	}{decoder, segment}
	var res struct {
		segmentID uint32
		error     []byte
	}

	fastcgo.UnsafeCall2(
		C.prompp_wal_decoder_decode_dry,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.segmentID, res.error
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

//
// LabelSetStorage EncodingBimap
//

// primitivesLSSCtor - wrapper for constructor C-Lss.
func primitivesLSSCtor(lss_type uint32) uintptr {
	var args = struct {
		lss_type uint32
	}{lss_type}
	var res struct {
		lss uintptr
	}

	fastcgo.UnsafeCall2(
		C.prompp_primitives_lss_ctor,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.lss
}

// primitivesLSSDtor - wrapper for destructor C-EncodingBimap.
func primitivesLSSDtor(lss uintptr) {
	var args = struct {
		lss uintptr
	}{lss}

	fastcgo.UnsafeCall1(
		C.prompp_primitives_lss_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

// primitivesLSSAllocatedMemory -  return size of allocated memory for label sets in C++.
func primitivesLSSAllocatedMemory(lss uintptr) uint64 {
	var args = struct {
		lss uintptr
	}{lss}
	var res struct {
		allocatedMemory uint64
	}

	fastcgo.UnsafeCall2(
		C.prompp_primitives_lss_allocated_memory,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.allocatedMemory
}

//
// StatelessRelabeler
//

// prometheusStatelessRelabelerCtor - wrapper for constructor C-StatelessRelabeler.
func prometheusStatelessRelabelerCtor(cfgs []*RelabelConfig) (statelessRelabeler uintptr, exception []byte) {
	var args = struct {
		cfgs []*RelabelConfig
	}{cfgs}
	var res struct {
		statelessRelabeler uintptr
		exception          []byte
	}

	fastcgo.UnsafeCall2(
		C.prompp_prometheus_stateless_relabeler_ctor,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.statelessRelabeler, res.exception
}

// prometheusStatelessRelabelerDtor - wrapper for destructor C-StatelessRelabeler.
func prometheusStatelessRelabelerDtor(statelessRelabeler uintptr) {
	var args = struct {
		statelessRelabeler uintptr
	}{statelessRelabeler}

	fastcgo.UnsafeCall1(
		C.prompp_prometheus_stateless_relabeler_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

// prometheusStatelessRelabelerResetTo reset configs and replace on new converting go-config..
func prometheusStatelessRelabelerResetTo(statelessRelabeler uintptr, cfgs []*RelabelConfig) (exception []byte) {
	var args = struct {
		statelessRelabeler uintptr
		cfgs               []*RelabelConfig
	}{statelessRelabeler, cfgs}
	var res struct {
		exception []byte
	}

	fastcgo.UnsafeCall2(
		C.prompp_prometheus_stateless_relabeler_reset_to,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.exception
}

//
// InnerSeries
//

// prometheusInnerSeriesCtor - wrapper for constructor C-InnerSeries(vector).
func prometheusInnerSeriesCtor(innerSeries *InnerSeries) {
	var args = struct {
		innerSeries *InnerSeries
	}{innerSeries}

	fastcgo.UnsafeCall1(
		C.prompp_prometheus_inner_series_ctor,
		uintptr(unsafe.Pointer(&args)),
	)
}

// prometheusInnerSeriesDtor - wrapper for destructor C-InnerSeries(vector).
func prometheusInnerSeriesDtor(innerSeries *InnerSeries) {
	var args = struct {
		innerSeries *InnerSeries
	}{innerSeries}

	fastcgo.UnsafeCall1(
		C.prompp_prometheus_inner_series_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

//
// RelabeledSeries
//

// prometheusRelabeledSeriesCtor - wrapper for constructor C-RelabeledSeries(vector).
func prometheusRelabeledSeriesCtor(relabeledSeries *RelabeledSeries) {
	var args = struct {
		relabeledSeries *RelabeledSeries
	}{relabeledSeries}

	fastcgo.UnsafeCall1(
		C.prompp_prometheus_relabeled_series_ctor,
		uintptr(unsafe.Pointer(&args)),
	)
}

// prometheusRelabeledSeriesDtor - wrapper for destructor C-RelabeledSeries(vector).
func prometheusRelabeledSeriesDtor(relabeledSeries *RelabeledSeries) {
	var args = struct {
		relabeledSeries *RelabeledSeries
	}{relabeledSeries}

	fastcgo.UnsafeCall1(
		C.prompp_prometheus_relabeled_series_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

//
// RelabelerStateUpdate
//

// prometheusRelabelerStateUpdateCtor - wrapper for constructor C-RelabelerStateUpdate(vector), filling in c++.
func prometheusRelabelerStateUpdateCtor(relabelerStateUpdate *RelabelerStateUpdate, generation uint32) {
	var args = struct {
		relabelerStateUpdate *RelabelerStateUpdate
		generation           uint32
	}{relabelerStateUpdate, generation}

	fastcgo.UnsafeCall1(
		C.prompp_prometheus_relabeler_state_update_ctor,
		uintptr(unsafe.Pointer(&args)),
	)
}

// prometheusRelabelerStateUpdateDtor - wrapper for destructor C-RelabelerStateUpdate(vector).
func prometheusRelabelerStateUpdateDtor(relabelerStateUpdate *RelabelerStateUpdate) {
	var args = struct {
		relabelerStateUpdate *RelabelerStateUpdate
	}{relabelerStateUpdate}

	fastcgo.UnsafeCall1(
		C.prompp_prometheus_relabeler_state_update_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

//
// PerShardRelabeler
//

// prometheusPerShardRelabelerCtor - wrapper for constructor C-PerShardRelabeler.
func prometheusPerShardRelabelerCtor(
	externalLabels []Label,
	statelessRelabeler uintptr,
	lssGeneration uint32,
	numberOfShards, shardID uint16,
) (perShardRelabeler uintptr, exception []byte) {
	var args = struct {
		externalLabels     []Label
		statelessRelabeler uintptr
		lssGeneration      uint32
		numberOfShards     uint16
		shardID            uint16
	}{externalLabels, statelessRelabeler, lssGeneration, numberOfShards, shardID}
	var res struct {
		perShardRelabeler uintptr
		exception         []byte
	}

	fastcgo.UnsafeCall2(
		C.prompp_prometheus_per_shard_relabeler_ctor,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.perShardRelabeler, res.exception
}

// prometheusPerShardRelabelerDtor - wrapper for destructor C-PerShardRelabeler.
func prometheusPerShardRelabelerDtor(perShardRelabeler uintptr) {
	var args = struct {
		perShardRelabeler uintptr
	}{perShardRelabeler}

	fastcgo.UnsafeCall1(
		C.prompp_prometheus_per_shard_relabeler_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

// prometheusPerShardRelabelerCacheAllocatedMemory - return size of allocated memory for cache map.
func prometheusPerShardRelabelerCacheAllocatedMemory(perShardRelabeler uintptr) uint64 {
	var args = struct {
		perShardRelabeler uintptr
	}{perShardRelabeler}
	var res struct {
		cacheAllocatedMemory uint64
	}

	fastcgo.UnsafeCall2(
		C.prompp_prometheus_per_shard_relabeler_cache_allocated_memory,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.cacheAllocatedMemory
}

// prometheusPerShardRelabelerInputRelabeling - wrapper for relabeling incoming hashdex(first stage).
func prometheusPerShardRelabelerInputRelabeling(
	perShardRelabeler, lss, hashdex uintptr,
	labelLimits *LabelLimits,
	shardsInnerSeries []*InnerSeries,
	shardsRelabeledSeries []*RelabeledSeries,
) []byte {
	var args = struct {
		shardsInnerSeries     []*InnerSeries
		shardsRelabeledSeries []*RelabeledSeries
		labelLimits           *LabelLimits
		perShardRelabeler     uintptr
		hashdex               uintptr
		lss                   uintptr
	}{shardsInnerSeries, shardsRelabeledSeries, labelLimits, perShardRelabeler, hashdex, lss}
	var res struct {
		exception []byte
	}

	fastcgo.UnsafeCall2(
		C.prompp_prometheus_per_shard_relabeler_input_relabeling,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.exception
}

// prometheusPerShardRelabelerAppendRelabelerSeries - wrapper for add relabeled ls to lss,
// add to result and add to cache update(second stage).
func prometheusPerShardRelabelerAppendRelabelerSeries(
	perShardRelabeler, lss uintptr,
	innerSeries *InnerSeries,
	relabeledSeries *RelabeledSeries,
	relabelerStateUpdate *RelabelerStateUpdate,
) []byte {
	var args = struct {
		innerSeries          *InnerSeries
		relabeledSeries      *RelabeledSeries
		relabelerStateUpdate *RelabelerStateUpdate
		perShardRelabeler    uintptr
		lss                  uintptr
	}{innerSeries, relabeledSeries, relabelerStateUpdate, perShardRelabeler, lss}
	var res struct {
		exception []byte
	}

	fastcgo.UnsafeCall2(
		C.prompp_prometheus_per_shard_relabeler_append_relabeler_series,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.exception
}

// prometheusPerShardRelabelerUpdateRelabelerState - wrapper for add to cache relabled data(third stage).
func prometheusPerShardRelabelerUpdateRelabelerState(
	relabelerStateUpdate *RelabelerStateUpdate,
	perShardRelabeler uintptr,
	relabeledShardID uint16,
) []byte {
	var args = struct {
		relabelerStateUpdate *RelabelerStateUpdate
		perShardRelabeler    uintptr
		relabeledShardID     uint16
	}{relabelerStateUpdate, perShardRelabeler, relabeledShardID}
	var res struct {
		exception []byte
	}

	fastcgo.UnsafeCall2(
		C.prompp_prometheus_per_shard_relabeler_update_relabeler_state,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.exception
}

// prometheusPerShardRelabelerOutputRelabeling - wrapper for relabeling output series(fourth stage).
func prometheusPerShardRelabelerOutputRelabeling(
	perShardRelabeler, lss uintptr,
	incomingInnerSeries, encodersInnerSeries []*InnerSeries,
	relabeledSeries *RelabeledSeries,
	generation uint32,
) []byte {
	var args = struct {
		relabeledSeries     *RelabeledSeries
		incomingInnerSeries []*InnerSeries
		encodersInnerSeries []*InnerSeries
		perShardRelabeler   uintptr
		lss                 uintptr
		generation          uint32
	}{relabeledSeries, incomingInnerSeries, encodersInnerSeries, perShardRelabeler, lss, generation}
	var res struct {
		exception []byte
	}

	fastcgo.UnsafeCall2(
		C.prompp_prometheus_per_shard_relabeler_output_relabeling,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.exception
}

// prometheusPerShardRelabelerResetTo - reset cache and store lss generation.
func prometheusPerShardRelabelerResetTo(
	externalLabels []Label,
	perShardRelabeler uintptr,
	lssGeneration uint32,
	numberOfShards uint16,
) {
	var args = struct {
		externalLabels    []Label
		perShardRelabeler uintptr
		lssGeneration     uint32
		numberOfShards    uint16
	}{externalLabels, perShardRelabeler, lssGeneration, numberOfShards}

	fastcgo.UnsafeCall1(
		C.prompp_prometheus_per_shard_relabeler_reset_to,
		uintptr(unsafe.Pointer(&args)),
	)
}

func indexWriterCtor(lss uintptr, chunk_metadata_list *[][]ChunkMetadata) uintptr {
	var args = struct {
		lss                 uintptr
		chunk_metadata_list *[][]ChunkMetadata
	}{lss, chunk_metadata_list}

	var res struct {
		writer uintptr
	}

	fastcgo.UnsafeCall2(
		C.prompp_index_writer_ctor,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.writer
}

func indexWriterDtor(writer uintptr) {
	var args = struct {
		writer uintptr
	}{writer}

	fastcgo.UnsafeCall1(
		C.prompp_index_writer_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

func indexWriterWriteHeader(writer uintptr, data *[]byte) {
	var args = struct {
		writer uintptr
	}{writer}

	var res = struct {
		data *[]byte
	}{data}

	fastcgo.UnsafeCall2(
		C.prompp_index_writer_write_header,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)
}

func indexWriterWriteSymbols(writer uintptr, data *[]byte) {
	var args = struct {
		writer uintptr
	}{writer}

	var res = struct {
		data *[]byte
	}{data}

	fastcgo.UnsafeCall2(
		C.prompp_index_writer_write_symbols,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)
}

func indexWriterWriteNextSeriesBatch(writer uintptr, batch_size uint32, data *[]byte, has_more_data *bool) {
	var args = struct {
		writer     uintptr
		batch_size uint32
	}{writer, batch_size}

	var res = struct {
		has_more_data bool
		data          *[]byte
	}{false, data}

	fastcgo.UnsafeCall2(
		C.prompp_index_writer_write_next_series_batch,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	*has_more_data = res.has_more_data
}

func indexWriterWriteLabelIndices(writer uintptr, data *[]byte) {
	var args = struct {
		writer uintptr
	}{writer}

	var res = struct {
		data *[]byte
	}{data}

	fastcgo.UnsafeCall2(
		C.prompp_index_writer_write_label_indices,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)
}

func indexWriterWriteNextPostingsBatch(writer uintptr, max_batch_size uint32, data *[]byte, has_more_data *bool) {
	var args = struct {
		writer         uintptr
		max_batch_size uint32
	}{writer, max_batch_size}

	var res = struct {
		has_more_data bool
		data          *[]byte
	}{false, data}

	fastcgo.UnsafeCall2(
		C.prompp_index_writer_write_next_postings_batch,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	*has_more_data = res.has_more_data
}

func indexWriterWriteLabelIndicesTable(writer uintptr, data *[]byte) {
	var args = struct {
		writer uintptr
	}{writer}

	var res = struct {
		data *[]byte
	}{data}

	fastcgo.UnsafeCall2(
		C.prompp_index_writer_write_label_indices_table,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)
}

func indexWriterWritePostingsTableOffsets(writer uintptr, data *[]byte) {
	var args = struct {
		writer uintptr
	}{writer}

	var res = struct {
		data *[]byte
	}{data}

	fastcgo.UnsafeCall2(
		C.prompp_index_writer_write_postings_table_offsets,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)
}

func indexWriterWriteTableOfContents(writer uintptr, data *[]byte) {
	var args = struct {
		writer uintptr
	}{writer}

	var res = struct {
		data *[]byte
	}{data}

	fastcgo.UnsafeCall2(
		C.prompp_index_writer_write_table_of_contents,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)
}
