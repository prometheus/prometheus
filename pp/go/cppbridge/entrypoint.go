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
// #cgo LDFLAGS: -static-libgcc -static-libstdc++ -l:libstdc++.a -l:libm.a -l:libunwind.a -l:liblzma.a -l:libstdc++_libbacktrace.a
// #cgo static LDFLAGS: -static
// #include "entrypoint.h"
import "C" //nolint:gocritic // because otherwise it won't work
import (
	"runtime"
	"time"
	"unsafe" //nolint:gocritic // because otherwise it won't work

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/prometheus/pp/go/cppbridge/fastcgo"
	"github.com/prometheus/prometheus/pp/go/model"
)

var unsafeCall = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "prompp_cppbridge_unsafecall_nanoseconds",
		Help: "The time duration cpp call.",
	},
	[]string{"object", "method"},
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

func dumpMemoryProfile(filename string) int {
	args := struct {
		filename string
	}{filename}

	res := struct {
		error int
	}{0}

	fastcgo.UnsafeCall2(
		C.prompp_dump_memory_profile,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)
	return res.error
}

//
// ProtobufHashdex
//

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

func walHashdexDtor(hashdex uintptr) {
	args := struct {
		hashdex uintptr
	}{hashdex}

	fastcgo.UnsafeCall1(
		C.prompp_wal_hashdex_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

func walProtobufHashdexSnappyPresharding(
	hashdex uintptr,
	compressedProtobuf []byte,
) (cluster, replica string, err []byte) {
	args := struct {
		hashdex            uintptr
		compressedProtobuf []byte
	}{hashdex, compressedProtobuf}
	var res struct {
		cluster   string
		replica   string
		exception []byte
	}

	fastcgo.UnsafeCall2(
		C.prompp_wal_protobuf_hashdex_snappy_presharding,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.cluster, res.replica, res.exception
}

func walProtobufHashdexGetMetadata(hashdex uintptr) []WALScraperHashdexMetadata {
	args := struct {
		hashdex uintptr
	}{hashdex}
	var res struct {
		metadata []WALScraperHashdexMetadata
	}

	fastcgo.UnsafeCall2(
		C.prompp_wal_protobuf_hashdex_get_metadata,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.metadata
}

//
// GoModelHashdex
//

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

func walGoModelHashdexPresharding(hashdex uintptr, data []model.TimeSeries) (cluster, replica string, err []byte) {
	args := struct {
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
	args := struct {
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
	args := struct {
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
	args := struct {
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
	args := struct {
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
	args := struct {
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
	args := struct {
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
	args := struct {
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
	args := struct {
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
	args := struct {
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
	args := struct {
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
	args := struct {
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
	args := struct {
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
	args := struct {
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
	args := struct {
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
	args := struct {
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
	args := struct {
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

// walDecoderDecodeToHashdex decode WAL-segment into BasicDecoderHashdex through C++ decoder.
func walDecoderDecodeToHashdex(
	decoder uintptr,
	segment []byte,
) (
	stats DecodedSegmentStats,
	hashdex uintptr,
	cluster, replica string,
	err []byte,
) {
	args := struct {
		decoder uintptr
		segment []byte
	}{decoder, segment}
	var res struct {
		DecodedSegmentStats
		hashdex uintptr
		cluster string
		replica string
		error   []byte
	}

	fastcgo.UnsafeCall2(
		C.prompp_wal_decoder_decode_to_hashdex,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.DecodedSegmentStats, res.hashdex, res.cluster, res.replica, res.error
}

// walDecoderDecodeToHashdexWithMetricInjection decode WAL-segment into BasicDecoderHashdex through C++ decoder
// with metadata for injection metrics.
func walDecoderDecodeToHashdexWithMetricInjection(
	decoder uintptr,
	meta *MetaInjection,
	segment []byte,
) (
	stats DecodedSegmentStats,
	hashdex uintptr,
	cluster, replica string,
	err []byte,
) {
	args := struct {
		decoder uintptr
		meta    *MetaInjection
		segment []byte
	}{decoder, meta, segment}
	var res struct {
		DecodedSegmentStats
		hashdex uintptr
		cluster string
		replica string
		error   []byte
	}

	fastcgo.UnsafeCall2(
		C.prompp_wal_decoder_decode_to_hashdex_with_metric_injection,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.DecodedSegmentStats, res.hashdex, res.cluster, res.replica, res.error
}

// decoderDecode - decode WAL-segment and drop decoded data through C++ decoder.
func walDecoderDecodeDry(decoder uintptr, segment []byte) (segmentID uint32, err []byte) {
	args := struct {
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
	args := struct {
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
	args := struct {
		decoder uintptr
	}{decoder}

	fastcgo.UnsafeCall1(
		C.prompp_wal_decoder_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

//
// OutputDecoder
//

// walOutputDecoderCtor - wrapper for constructor C-WalOutputDecoder.
func walOutputDecoderCtor(
	externalLabels []Label,
	statelessRelabeler, outputLss uintptr,
	encodersVersion uint8,
) uintptr {
	args := struct {
		externalLabels     []Label
		statelessRelabeler uintptr
		outputLss          uintptr
		encodersVersion    uint8
	}{externalLabels, statelessRelabeler, outputLss, encodersVersion}
	var res struct {
		decoder uintptr
	}

	fastcgo.UnsafeCall2(
		C.prompp_wal_output_decoder_ctor,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.decoder
}

// walOutputDecoderDtor - wrapper for destructor C-WalOutputDecoder.
func walOutputDecoderDtor(decoder uintptr) {
	args := struct {
		decoder uintptr
	}{decoder}

	fastcgo.UnsafeCall1(
		C.prompp_wal_output_decoder_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

// walOutputDecoderDumpTo dump output decoder state(output_lss and cache) to slice byte.
func walOutputDecoderDumpTo(decoder uintptr) (dump, err []byte) {
	args := struct {
		decoder uintptr
	}{decoder}
	var res struct {
		dump  []byte
		error []byte
	}

	fastcgo.UnsafeCall2(
		C.prompp_wal_output_decoder_dump_to,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.dump, res.error
}

// walOutputDecoderLoadFrom load from dump(slice byte) output decoder state(output_lss and cache).
func walOutputDecoderLoadFrom(decoder uintptr, dump []byte) []byte {
	args := struct {
		dump    []byte
		decoder uintptr
	}{dump, decoder}
	var res struct {
		error []byte
	}

	fastcgo.UnsafeCall2(
		C.prompp_wal_output_decoder_load_from,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.error
}

// walOutputDecoderDecode decode segment to slice RefSample.
func walOutputDecoderDecode(
	segment []byte,
	decoder uintptr,
	lowerLimitTimestamp int64,
) (stats OutputDecoderStats, dump []RefSample, err []byte) {
	args := struct {
		segment             []byte
		decoder             uintptr
		lowerLimitTimestamp int64
	}{segment, decoder, lowerLimitTimestamp}
	var res struct {
		OutputDecoderStats
		refSamples []RefSample
		error      []byte
	}

	fastcgo.UnsafeCall2(
		C.prompp_wal_output_decoder_decode,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.OutputDecoderStats, res.refSamples, res.error
}

//
// ProtobufEncoder
//

// walProtobufEncoderCtor - wrapper for constructor C-ProtobufEncoder.
func walProtobufEncoderCtor(outputLsses []uintptr) uintptr {
	args := struct {
		outputLsses []uintptr
	}{outputLsses}
	var res struct {
		decoder uintptr
	}

	fastcgo.UnsafeCall2(
		C.prompp_wal_protobuf_encoder_ctor,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.decoder
}

// walProtobufEncoderDtor - wrapper for destructor C-ProtobufEncoder.
func walProtobufEncoderDtor(decoder uintptr) {
	args := struct {
		decoder uintptr
	}{decoder}

	fastcgo.UnsafeCall1(
		C.prompp_wal_protobuf_encoder_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

// walProtobufEncoderEncode encode batch slice ShardRefSamples to snapped protobufs on shards.
func walProtobufEncoderEncode(
	batch []*DecodedRefSamples,
	outSlices [][]byte,
	stats []protobufEncoderStats,
	encoder uintptr,
) []byte {
	args := struct {
		batch     []*DecodedRefSamples
		outSlices [][]byte
		stats     []protobufEncoderStats
		encoder   uintptr
	}{batch, outSlices, stats, encoder}
	var res struct {
		error []byte
	}

	fastcgo.UnsafeCall2(
		C.prompp_wal_protobuf_encoder_encode,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.error
}

//
// LabelSetStorage EncodingBimap
//

// primitivesLSSCtor - wrapper for constructor C-Lss.
func primitivesLSSCtor(lss_type uint32) uintptr {
	args := struct {
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
	args := struct {
		lss uintptr
	}{lss}

	fastcgo.UnsafeCall1(
		C.prompp_primitives_lss_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

// primitivesLSSAllocatedMemory -  return size of allocated memory for label sets in C++.
func primitivesLSSAllocatedMemory(lss uintptr) uint64 {
	args := struct {
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

func primitivesLSSFindOrEmplace(lss uintptr, labelSet model.LabelSet) uint32 {
	args := struct {
		lss      uintptr
		labelSet model.LabelSet
	}{lss, labelSet}
	var res struct {
		labelSetID uint32
	}

	fastcgo.UnsafeCall2(
		C.prompp_primitives_lss_find_or_emplace,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.labelSetID
}

func primitivesLSSQuery(lss uintptr, matchers []model.LabelMatcher, querySource uint32) (uint32, []uint32) {
	args := struct {
		lss         uintptr
		matchers    []model.LabelMatcher
		querySource uint32
	}{lss, matchers, querySource}
	var res struct {
		status  uint32
		matches []uint32
	}

	fastcgo.UnsafeCall2(
		C.prompp_primitives_lss_query,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.status, res.matches
}

func primitivesLSSGetLabelSets(lss uintptr, labelSetIDs []uint32) []Labels {
	args := struct {
		lss         uintptr
		labelSetIDs []uint32
	}{lss, labelSetIDs}
	var res struct {
		labelSets []Labels
	}

	fastcgo.UnsafeCall2(
		C.prompp_primitives_lss_get_label_sets,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.labelSets
}

func primitivesLSSFreeLabelSets(labelSets []Labels) {
	args := struct {
		labelSets []Labels
	}{labelSets}

	fastcgo.UnsafeCall1(
		C.prompp_primitives_lss_free_label_sets,
		uintptr(unsafe.Pointer(&args)),
	)
}

func primitivesLSSQueryLabelNames(lss uintptr, matchers []model.LabelMatcher) (uint32, []string) {
	args := struct {
		lss      uintptr
		matchers []model.LabelMatcher
	}{lss, matchers}
	var res struct {
		status uint32
		names  []string
	}

	fastcgo.UnsafeCall2(
		C.prompp_primitives_lss_query_label_names,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.status, res.names
}

func primitivesLSSQueryLabelValues(lss uintptr, label_name string, matchers []model.LabelMatcher) (uint32, []string) {
	args := struct {
		lss        uintptr
		label_name string
		matchers   []model.LabelMatcher
	}{lss, label_name, matchers}
	var res struct {
		status uint32
		values []string
	}

	fastcgo.UnsafeCall2(
		C.prompp_primitives_lss_query_label_values,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.status, res.values
}

//
// StatelessRelabeler
//

// prometheusStatelessRelabelerCtor - wrapper for constructor C-StatelessRelabeler.
func prometheusStatelessRelabelerCtor(cfgs []*RelabelConfig) (statelessRelabeler uintptr, exception []byte) {
	args := struct {
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
	args := struct {
		statelessRelabeler uintptr
	}{statelessRelabeler}

	fastcgo.UnsafeCall1(
		C.prompp_prometheus_stateless_relabeler_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

// prometheusStatelessRelabelerResetTo reset configs and replace on new converting go-config..
func prometheusStatelessRelabelerResetTo(statelessRelabeler uintptr, cfgs []*RelabelConfig) (exception []byte) {
	args := struct {
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
	args := struct {
		innerSeries *InnerSeries
	}{innerSeries}

	fastcgo.UnsafeCall1(
		C.prompp_prometheus_inner_series_ctor,
		uintptr(unsafe.Pointer(&args)),
	)
}

// prometheusInnerSeriesDtor - wrapper for destructor C-InnerSeries(vector).
func prometheusInnerSeriesDtor(innerSeries *InnerSeries) {
	args := struct {
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
	args := struct {
		relabeledSeries *RelabeledSeries
	}{relabeledSeries}

	fastcgo.UnsafeCall1(
		C.prompp_prometheus_relabeled_series_ctor,
		uintptr(unsafe.Pointer(&args)),
	)
}

// prometheusRelabeledSeriesDtor - wrapper for destructor C-RelabeledSeries(vector).
func prometheusRelabeledSeriesDtor(relabeledSeries *RelabeledSeries) {
	args := struct {
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
func prometheusRelabelerStateUpdateCtor(relabelerStateUpdate *RelabelerStateUpdate) {
	args := struct {
		relabelerStateUpdate *RelabelerStateUpdate
	}{relabelerStateUpdate}

	fastcgo.UnsafeCall1(
		C.prompp_prometheus_relabeler_state_update_ctor,
		uintptr(unsafe.Pointer(&args)),
	)
}

// prometheusRelabelerStateUpdateDtor - wrapper for destructor C-RelabelerStateUpdate(vector).
func prometheusRelabelerStateUpdateDtor(relabelerStateUpdate *RelabelerStateUpdate) {
	args := struct {
		relabelerStateUpdate *RelabelerStateUpdate
	}{relabelerStateUpdate}

	fastcgo.UnsafeCall1(
		C.prompp_prometheus_relabeler_state_update_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

//
// StalenansState
//

func prometheusRelabelStaleNansStateCtor() uintptr {
	var res struct {
		state uintptr
	}

	fastcgo.UnsafeCall1(
		C.prompp_prometheus_relabel_stalenans_state_ctor,
		uintptr(unsafe.Pointer(&res)),
	)

	return res.state
}

func prometheusRelabelStaleNansStateReset(state uintptr) {
	args := struct {
		state uintptr
	}{state}

	fastcgo.UnsafeCall1(
		C.prompp_prometheus_relabel_stalenans_state_reset,
		uintptr(unsafe.Pointer(&args)),
	)
}

func prometheusRelabelStaleNansStateDtor(state uintptr) {
	args := struct {
		state uintptr
	}{state}

	fastcgo.UnsafeCall1(
		C.prompp_prometheus_relabel_stalenans_state_dtor,
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
	numberOfShards, shardID uint16,
) (perShardRelabeler uintptr, exception []byte) {
	args := struct {
		externalLabels     []Label
		statelessRelabeler uintptr
		numberOfShards     uint16
		shardID            uint16
	}{externalLabels, statelessRelabeler, numberOfShards, shardID}
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
	args := struct {
		perShardRelabeler uintptr
	}{perShardRelabeler}

	fastcgo.UnsafeCall1(
		C.prompp_prometheus_per_shard_relabeler_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

// prometheusPerShardRelabelerCacheAllocatedMemory - return size of allocated memory for cache map.
func prometheusPerShardRelabelerCacheAllocatedMemory(perShardRelabeler uintptr) uint64 {
	args := struct {
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
	perShardRelabeler, inputLss, targetLss, cache, hashdex uintptr,
	options RelabelerOptions,
	shardsInnerSeries []*InnerSeries,
	shardsRelabeledSeries []*RelabeledSeries,
) (samplesAdded, seriesAdded uint32, exception []byte) {
	args := struct {
		shardsInnerSeries     []*InnerSeries
		shardsRelabeledSeries []*RelabeledSeries
		options               RelabelerOptions
		perShardRelabeler     uintptr
		hashdex               uintptr
		cache                 uintptr
		inputLss              uintptr
		targetLss             uintptr
	}{shardsInnerSeries, shardsRelabeledSeries, options, perShardRelabeler, hashdex, cache, inputLss, targetLss}
	var res struct {
		samplesAdded uint32
		seriesAdded  uint32
		exception    []byte
	}
	start := time.Now()
	fastcgo.UnsafeCall2(
		C.prompp_prometheus_per_shard_relabeler_input_relabeling,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)
	unsafeCall.With(
		prometheus.Labels{"object": "input_relabeler", "method": "input_relabeling"},
	).Add(float64(time.Since(start).Nanoseconds()))

	return res.samplesAdded, res.seriesAdded, res.exception
}

// prometheusPerShardRelabelerInputRelabelingWithStalenans wrapper for relabeling incoming
// hashdex(first stage) with state stalenans.
func prometheusPerShardRelabelerInputRelabelingWithStalenans(
	perShardRelabeler, inputLss, targetLss, cache, hashdex, sourceState uintptr,
	defTimestamp int64,
	options RelabelerOptions,
	shardsInnerSeries []*InnerSeries,
	shardsRelabeledSeries []*RelabeledSeries,
) (samplesAdded, seriesAdded uint32, exception []byte) {
	args := struct {
		shardsInnerSeries     []*InnerSeries
		shardsRelabeledSeries []*RelabeledSeries
		options               RelabelerOptions
		perShardRelabeler     uintptr
		hashdex               uintptr
		cache                 uintptr
		inputLss              uintptr
		targetLss             uintptr
		state                 uintptr
		defTimestamp          int64
	}{
		shardsInnerSeries,
		shardsRelabeledSeries,
		options,
		perShardRelabeler,
		hashdex,
		cache,
		inputLss,
		targetLss,
		sourceState,
		defTimestamp,
	}
	var res struct {
		samplesAdded uint32
		seriesAdded  uint32
		exception    []byte
	}
	start := time.Now()
	fastcgo.UnsafeCall2(
		C.prompp_prometheus_per_shard_relabeler_input_relabeling_with_stalenans,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)
	unsafeCall.With(
		prometheus.Labels{"object": "input_relabeler", "method": "relabeling_with_stalenans"},
	).Add(float64(time.Since(start).Nanoseconds()))

	return res.samplesAdded, res.seriesAdded, res.exception
}

// prometheusPerShardRelabelerAppendRelabelerSeries - wrapper for add relabeled ls to lss,
// add to result and add to cache update(second stage).
func prometheusPerShardRelabelerAppendRelabelerSeries(
	perShardRelabeler, lss uintptr,
	innerSeries *InnerSeries,
	relabeledSeries *RelabeledSeries,
	relabelerStateUpdate *RelabelerStateUpdate,
) []byte {
	args := struct {
		innerSeries          *InnerSeries
		relabeledSeries      *RelabeledSeries
		relabelerStateUpdate *RelabelerStateUpdate
		perShardRelabeler    uintptr
		lss                  uintptr
	}{innerSeries, relabeledSeries, relabelerStateUpdate, perShardRelabeler, lss}
	var res struct {
		exception []byte
	}
	start := time.Now()
	fastcgo.UnsafeCall2(
		C.prompp_prometheus_per_shard_relabeler_append_relabeler_series,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)
	unsafeCall.With(
		prometheus.Labels{"object": "input_relabeler", "method": "append_relabeler_series"},
	).Add(float64(time.Since(start).Nanoseconds()))

	return res.exception
}

// prometheusPerShardRelabelerUpdateRelabelerState - wrapper for add to cache relabled data(third stage).
func prometheusPerShardRelabelerUpdateRelabelerState(
	relabelerStateUpdate *RelabelerStateUpdate,
	perShardRelabeler, cache uintptr,
	relabeledShardID uint16,
) []byte {
	args := struct {
		relabelerStateUpdate *RelabelerStateUpdate
		perShardRelabeler    uintptr
		cache                uintptr
		relabeledShardID     uint16
	}{relabelerStateUpdate, perShardRelabeler, cache, relabeledShardID}
	var res struct {
		exception []byte
	}
	start := time.Now()
	fastcgo.UnsafeCall2(
		C.prompp_prometheus_per_shard_relabeler_update_relabeler_state,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)
	unsafeCall.With(
		prometheus.Labels{"object": "input_relabeler", "method": "update_relabeler_state"},
	).Add(float64(time.Since(start).Nanoseconds()))

	return res.exception
}

// prometheusPerShardRelabelerOutputRelabeling - wrapper for relabeling output series(fourth stage).
func prometheusPerShardRelabelerOutputRelabeling(
	perShardRelabeler, lss, cache uintptr,
	incomingInnerSeries, encodersInnerSeries []*InnerSeries,
	relabeledSeries *RelabeledSeries,
) []byte {
	args := struct {
		relabeledSeries     *RelabeledSeries
		incomingInnerSeries []*InnerSeries
		encodersInnerSeries []*InnerSeries
		perShardRelabeler   uintptr
		lss                 uintptr
		cache               uintptr
	}{relabeledSeries, incomingInnerSeries, encodersInnerSeries, perShardRelabeler, lss, cache}
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

// prometheusPerShardRelabelerResetTo - reset set new number_of_shards and external_labels.
func prometheusPerShardRelabelerResetTo(
	externalLabels []Label,
	perShardRelabeler uintptr,
	numberOfShards uint16,
) {
	args := struct {
		externalLabels    []Label
		perShardRelabeler uintptr
		numberOfShards    uint16
	}{externalLabels, perShardRelabeler, numberOfShards}
	start := time.Now()
	fastcgo.UnsafeCall1(
		C.prompp_prometheus_per_shard_relabeler_reset_to,
		uintptr(unsafe.Pointer(&args)),
	)
	unsafeCall.With(
		prometheus.Labels{"object": "input_relabeler", "method": "reset_to"},
	).Add(float64(time.Since(start).Nanoseconds()))
}

func seriesDataDataStorageCtor() uintptr {
	var res struct {
		dataStorage uintptr
	}

	fastcgo.UnsafeCall1(
		C.prompp_series_data_data_storage_ctor,
		uintptr(unsafe.Pointer(&res)),
	)

	return res.dataStorage
}

func seriesDataDataStorageReset(dataStorage uintptr) {
	args := struct {
		dataStorage uintptr
	}{dataStorage}
	start := time.Now()
	fastcgo.UnsafeCall1(
		C.prompp_series_data_data_storage_reset,
		uintptr(unsafe.Pointer(&args)),
	)
	unsafeCall.With(
		prometheus.Labels{"object": "head_data_storage", "method": "reset"},
	).Add(float64(time.Since(start).Nanoseconds()))
}

func seriesDataDataStorageAllocatedMemory(dataStorage uintptr) uint64 {
	args := struct {
		dataStorage uintptr
	}{dataStorage}
	var res struct {
		allocatedMemory uint64
	}
	start := time.Now()
	fastcgo.UnsafeCall2(
		C.prompp_series_data_data_storage_allocated_memory,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)
	unsafeCall.With(
		prometheus.Labels{"object": "head_data_storage", "method": "allocated_memory"},
	).Add(float64(time.Since(start).Nanoseconds()))

	return res.allocatedMemory
}

func seriesDataDataStorageQuery(dataStorage uintptr, query HeadDataStorageQuery) []byte {
	args := struct {
		dataStorage uintptr
		query       HeadDataStorageQuery
	}{dataStorage, query}
	var res struct {
		serializedChunks []byte
	}
	start := time.Now()
	fastcgo.UnsafeCall2(
		C.prompp_series_data_data_storage_query,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)
	unsafeCall.With(
		prometheus.Labels{"object": "head_data_storage", "method": "query"},
	).Add(float64(time.Since(start).Nanoseconds()))

	return res.serializedChunks
}

func seriesDataDataStorageTimeInterval(dataStorage uintptr) TimeInterval {
	args := struct {
		dataStorage uintptr
	}{dataStorage}
	res := struct {
		interval TimeInterval
	}{}

	fastcgo.UnsafeCall2(
		C.prompp_series_data_data_storage_time_interval,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.interval
}

func seriesDataDataStorageDtor(dataStorage uintptr) {
	args := struct {
		dataStorage uintptr
	}{dataStorage}

	fastcgo.UnsafeCall1(
		C.prompp_series_data_data_storage_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

func seriesDataEncoderCtor(dataStorage uintptr) uintptr {
	args := struct {
		dataStorage uintptr
	}{dataStorage}
	var res struct {
		encoder uintptr
	}

	fastcgo.UnsafeCall2(
		C.prompp_series_data_encoder_ctor,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.encoder
}

func seriesDataEncoderEncode(encoder uintptr, seriesID uint32, timestamp int64, value float64) {
	args := struct {
		encoder   uintptr
		seriesID  uint32
		timestamp int64
		value     float64
	}{encoder, seriesID, timestamp, value}
	start := time.Now()
	fastcgo.UnsafeCall1(
		C.prompp_series_data_encoder_encode,
		uintptr(unsafe.Pointer(&args)),
	)
	unsafeCall.With(
		prometheus.Labels{"object": "head_data_storage", "method": "encode"},
	).Add(float64(time.Since(start).Nanoseconds()))
}

func seriesDataEncoderEncodeInnerSeriesSlice(encoder uintptr, innerSeriesSlice []*InnerSeries) {
	args := struct {
		encoder          uintptr
		innerSeriesSlice []*InnerSeries
	}{encoder, innerSeriesSlice}
	start := time.Now()
	fastcgo.UnsafeCall1(
		C.prompp_series_data_encoder_encode_inner_series_slice,
		uintptr(unsafe.Pointer(&args)),
	)
	unsafeCall.With(
		prometheus.Labels{"object": "head_data_storage", "method": "encode_inner_series_slice"},
	).Add(float64(time.Since(start).Nanoseconds()))
}

func seriesDataEncoderMergeOutOfOrderChunks(encoder uintptr) {
	args := struct {
		encoder uintptr
	}{encoder}
	start := time.Now()
	fastcgo.UnsafeCall1(
		C.prompp_series_data_encoder_merge_out_of_order_chunks,
		uintptr(unsafe.Pointer(&args)),
	)
	unsafeCall.With(
		prometheus.Labels{"object": "head_data_storage", "method": "merge_out_of_order_chunks"},
	).Add(float64(time.Since(start).Nanoseconds()))
}

func seriesDataEncoderDtor(encoder uintptr) {
	args := struct {
		encoder uintptr
	}{encoder}

	fastcgo.UnsafeCall1(
		C.prompp_series_data_encoder_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

func seriesDataDeserializerCtor(serializedChunks []byte) uintptr {
	args := struct {
		serializedChunks []byte
	}{serializedChunks}
	var res struct {
		deserializer uintptr
	}

	fastcgo.UnsafeCall2(
		C.prompp_series_data_deserializer_ctor,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.deserializer
}

func seriesDataDeserializerCreateDecodeIterator(deserializer uintptr, chunkMetadata []byte) uintptr {
	args := struct {
		deserializer  uintptr
		chunkMetadata []byte
	}{deserializer, chunkMetadata}
	var res struct {
		decodeIterator uintptr
	}

	fastcgo.UnsafeCall2(
		C.prompp_series_data_deserializer_create_decode_iterator,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.decodeIterator
}

func seriesDataDecodeIteratorNext(decodeIterator uintptr) bool {
	args := struct {
		decodeIterator uintptr
	}{decodeIterator}
	var res struct {
		hasValue bool
	}

	fastcgo.UnsafeCall2(
		C.prompp_series_data_decode_iterator_next,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.hasValue
}

func seriesDataDecodeIteratorSample(decodeIterator uintptr) (int64, float64) {
	args := struct {
		decodeIterator uintptr
	}{decodeIterator}
	var res struct {
		timestamp int64
		value     float64
	}

	fastcgo.UnsafeCall2(
		C.prompp_series_data_decode_iterator_sample,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.timestamp, res.value
}

func seriesDataDecodeIteratorDtor(decodeIterator uintptr) {
	args := struct {
		decodeIterator uintptr
	}{decodeIterator}

	fastcgo.UnsafeCall1(
		C.prompp_series_data_decode_iterator_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

func seriesDataDeserializerDtor(deserializer uintptr) {
	args := struct {
		deserializer uintptr
	}{deserializer}

	fastcgo.UnsafeCall1(
		C.prompp_series_data_deserializer_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

func seriesDataChunkRecoderCtor(lss, dataStorage uintptr, timeInterval TimeInterval) uintptr {
	args := struct {
		lss         uintptr
		dataStorage uintptr
		TimeInterval
	}{lss, dataStorage, timeInterval}
	var res struct {
		chunkRecoder uintptr
	}

	fastcgo.UnsafeCall2(
		C.prompp_series_data_chunk_recoder_ctor,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.chunkRecoder
}

func seriesDataChunkRecoderRecodeNextChunk(chunkRecoder uintptr, recodedChunk *RecodedChunk) {
	args := struct {
		chunkRecoder uintptr
	}{chunkRecoder}
	start := time.Now()
	fastcgo.UnsafeCall2(
		C.prompp_series_data_chunk_recoder_recode_next_chunk,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(recodedChunk)),
	)
	unsafeCall.With(
		prometheus.Labels{"object": "chunk_recoder", "method": "recode_next_chunk"},
	).Add(float64(time.Since(start).Nanoseconds()))
}

func seriesDataChunkRecoderDtor(chunkRecoder uintptr) {
	args := struct {
		chunkRecoder uintptr
	}{chunkRecoder}

	fastcgo.UnsafeCall1(
		C.prompp_series_data_chunk_recoder_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

func indexWriterCtor(lss uintptr) uintptr {
	args := struct {
		lss uintptr
	}{lss}

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
	args := struct {
		writer uintptr
	}{writer}

	fastcgo.UnsafeCall1(
		C.prompp_index_writer_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

func indexWriterWriteHeader(writer uintptr, data []byte) []byte {
	args := struct {
		writer uintptr
	}{writer}

	res := struct {
		data []byte
	}{data}

	fastcgo.UnsafeCall2(
		C.prompp_index_writer_write_header,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.data
}

func indexWriterWriteSymbols(writer uintptr, data []byte) []byte {
	args := struct {
		writer uintptr
	}{writer}

	res := struct {
		data []byte
	}{data}

	fastcgo.UnsafeCall2(
		C.prompp_index_writer_write_symbols,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.data
}

func indexWriterWriteNextSeriesBatch(writer uintptr, ls_id uint32, chunks_meta []ChunkMetadata, data []byte) []byte {
	args := struct {
		writer      uintptr
		chunks_meta []ChunkMetadata
		ls_id       uint32
	}{writer, chunks_meta, ls_id}

	res := struct {
		data []byte
	}{data}

	fastcgo.UnsafeCall2(
		C.prompp_index_writer_write_next_series_batch,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.data
}

func indexWriterWriteLabelIndices(writer uintptr, data []byte) []byte {
	args := struct {
		writer uintptr
	}{writer}

	res := struct {
		data []byte
	}{data}

	fastcgo.UnsafeCall2(
		C.prompp_index_writer_write_label_indices,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.data
}

func indexWriterWriteNextPostingsBatch(writer uintptr, max_batch_size uint32, data []byte) ([]byte, bool) {
	args := struct {
		writer         uintptr
		max_batch_size uint32
	}{writer, max_batch_size}

	res := struct {
		data          []byte
		has_more_data bool
	}{data, false}

	fastcgo.UnsafeCall2(
		C.prompp_index_writer_write_next_postings_batch,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.data, res.has_more_data
}

func indexWriterWriteLabelIndicesTable(writer uintptr, data []byte) []byte {
	args := struct {
		writer uintptr
	}{writer}

	res := struct {
		data []byte
	}{data}

	fastcgo.UnsafeCall2(
		C.prompp_index_writer_write_label_indices_table,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.data
}

func indexWriterWritePostingsTableOffsets(writer uintptr, data []byte) []byte {
	args := struct {
		writer uintptr
	}{writer}

	res := struct {
		data []byte
	}{data}

	fastcgo.UnsafeCall2(
		C.prompp_index_writer_write_postings_table_offsets,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.data
}

func indexWriterWriteTableOfContents(writer uintptr, data []byte) []byte {
	args := struct {
		writer uintptr
	}{writer}

	res := struct {
		data []byte
	}{data}

	fastcgo.UnsafeCall2(
		C.prompp_index_writer_write_table_of_contents,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.data
}

func getHeadStatus(lss, dataStorage uintptr, status *HeadStatus, limit int) {
	args := struct {
		lss         uintptr
		dataStorage uintptr
		limit       int
	}{lss, dataStorage, limit}

	fastcgo.UnsafeCall2(
		C.prompp_get_head_status,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(status)),
	)
}

func freeHeadStatus(status *HeadStatus) {
	fastcgo.UnsafeCall1(
		C.prompp_free_head_status,
		uintptr(unsafe.Pointer(status)),
	)
}

//
// Prometheus Scraper
//

func walPrometheusScraperHashdexCtor() uintptr {
	var res struct {
		hashdex uintptr
	}

	fastcgo.UnsafeCall1(
		C.prompp_wal_prometheus_scraper_hashdex_ctor,
		uintptr(unsafe.Pointer(&res)),
	)

	return res.hashdex
}

func walPrometheusScraperHashdexParse(hashdex uintptr, buffer []byte, default_timestamp int64) (uint32, uint32) {
	args := struct {
		hashdex           uintptr
		buffer            []byte
		default_timestamp int64
	}{hashdex, buffer, default_timestamp}
	var res struct {
		error   uint32
		scraped uint32
	}
	start := time.Now()
	fastcgo.UnsafeCall2(
		C.prompp_wal_prometheus_scraper_hashdex_parse,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)
	unsafeCall.With(
		prometheus.Labels{"object": "hashdex", "method": "parse"},
	).Add(float64(time.Since(start).Nanoseconds()))

	return res.scraped, res.error
}

func walPrometheusScraperHashdexGetMetadata(hashdex uintptr) []WALScraperHashdexMetadata {
	args := struct {
		hashdex uintptr
	}{hashdex}
	var res struct {
		metadata []WALScraperHashdexMetadata
	}

	fastcgo.UnsafeCall2(
		C.prompp_wal_prometheus_scraper_hashdex_get_metadata,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.metadata
}

//
// OpenMetrics scraper
//

func walOpenMetricsScraperHashdexCtor() uintptr {
	var res struct {
		hashdex uintptr
	}

	fastcgo.UnsafeCall1(
		C.prompp_wal_open_metrics_scraper_hashdex_ctor,
		uintptr(unsafe.Pointer(&res)),
	)

	return res.hashdex
}

func walOpenMetricsScraperHashdexParse(hashdex uintptr, buffer []byte, default_timestamp int64) (uint32, uint32) {
	args := struct {
		hashdex           uintptr
		buffer            []byte
		default_timestamp int64
	}{hashdex, buffer, default_timestamp}
	var res struct {
		error   uint32
		scraped uint32
	}
	start := time.Now()
	fastcgo.UnsafeCall2(
		C.prompp_wal_open_metrics_scraper_hashdex_parse,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)
	unsafeCall.With(
		prometheus.Labels{"object": "hashdex", "method": "parse"},
	).Add(float64(time.Since(start).Nanoseconds()))

	return res.scraped, res.error
}

func walOpenMetricsScraperHashdexGetMetadata(hashdex uintptr) []WALScraperHashdexMetadata {
	args := struct {
		hashdex uintptr
	}{hashdex}
	var res struct {
		metadata []WALScraperHashdexMetadata
	}

	fastcgo.UnsafeCall2(
		C.prompp_wal_open_metrics_scraper_hashdex_get_metadata,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.metadata
}

//
// Relabeler cache
//

// prometheusCacheCtor wrapper for constructor C-Cache.
func prometheusCacheCtor() uintptr {
	var res struct {
		cache uintptr
	}

	fastcgo.UnsafeCall1(
		C.prompp_prometheus_cache_ctor,
		uintptr(unsafe.Pointer(&res)),
	)

	return res.cache
}

// prometheusCacheDtor wrapper for destructor C-Cache.
func prometheusCacheDtor(cache uintptr) {
	args := struct {
		cache uintptr
	}{cache}

	fastcgo.UnsafeCall1(
		C.prompp_prometheus_cache_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

// prometheusCacheAllocatedMemory return size of allocated memory for caches.
func prometheusCacheAllocatedMemory(cache uintptr) uint64 {
	args := struct {
		cache uintptr
	}{cache}
	var res struct {
		cacheAllocatedMemory uint64
	}

	fastcgo.UnsafeCall2(
		C.prompp_prometheus_cache_allocated_memory,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.cacheAllocatedMemory
}

// prometheusCacheResetTo reset cache.
func prometheusCacheResetTo(cache uintptr) {
	args := struct {
		cache uintptr
	}{cache}

	fastcgo.UnsafeCall1(
		C.prompp_prometheus_cache_reset_to,
		uintptr(unsafe.Pointer(&args)),
	)
}

func headWalEncoderCtor(shardID uint16, logShards uint8, lss uintptr) uintptr {
	args := struct {
		shardID   uint16
		logShards uint8
		lss       uintptr
	}{shardID, logShards, lss}

	res := struct {
		encoder uintptr
	}{}

	fastcgo.UnsafeCall2(
		C.prompp_head_wal_encoder_ctor,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.encoder
}

func headWalEncoderAddInnerSeries(encoder uintptr, innerSeries []*InnerSeries) (stats WALEncoderStats, err error) {
	args := struct {
		innerSeries []*InnerSeries
		encoder     uintptr
	}{innerSeries, encoder}
	var res struct {
		WALEncoderStats
		exception []byte
	}

	fastcgo.UnsafeCall2(
		C.prompp_head_wal_encoder_add_inner_series,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.WALEncoderStats, handleException(res.exception)
}

// headWalEncoderFinalize - finalize the encoded data in the C++ encoder to Segment.
func headWalEncoderFinalize(encoder uintptr) (stats WALEncoderStats, segment []byte, err error) {
	args := struct {
		encoder uintptr
	}{encoder}
	var res struct {
		WALEncoderStats
		segment   []byte
		exception []byte
	}

	fastcgo.UnsafeCall2(
		C.prompp_head_wal_encoder_finalize,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.WALEncoderStats, res.segment, handleException(res.exception)
}

func headWalEncoderDtor(encoder uintptr) {
	args := struct {
		encoder uintptr
	}{encoder}

	fastcgo.UnsafeCall1(
		C.prompp_head_wal_encoder_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}

func headWalDecoderCtor(lss uintptr, encoderVersion uint8) uintptr {
	args := struct {
		lss            uintptr
		encoderVersion uint8
	}{lss, encoderVersion}

	res := struct {
		decoder uintptr
	}{}

	fastcgo.UnsafeCall2(
		C.prompp_head_wal_decoder_ctor,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.decoder
}

func headWalDecoderDecode(decoder uintptr, segment []byte, innerSeries *InnerSeries) error {
	args := struct {
		decoder     uintptr
		segment     []byte
		innerSeries *InnerSeries
	}{decoder, segment, innerSeries}
	var res struct {
		DecodedSegmentStats
		exception []byte
	}

	fastcgo.UnsafeCall2(
		C.prompp_head_wal_decoder_decode,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return handleException(res.exception)
}

func headWalDecoderDecodeToDataStorage(decoder uintptr, segment []byte, encoder uintptr) error {
	args := struct {
		decoder uintptr
		segment []byte
		encoder uintptr
	}{decoder, segment, encoder}
	var res struct {
		exception []byte
	}

	fastcgo.UnsafeCall2(
		C.prompp_head_wal_decoder_decode_to_data_storage,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return handleException(res.exception)
}

func headWalDecoderCreateEncoder(decoder uintptr) uintptr {
	args := struct {
		decoder uintptr
	}{decoder}
	var res struct {
		encoder uintptr
	}

	fastcgo.UnsafeCall2(
		C.prompp_head_wal_encoder_ctor_from_decoder,
		uintptr(unsafe.Pointer(&args)),
		uintptr(unsafe.Pointer(&res)),
	)

	return res.encoder
}

func headWalDecoderDtor(decoder uintptr) {
	args := struct {
		decoder uintptr
	}{decoder}

	fastcgo.UnsafeCall1(
		C.prompp_head_wal_decoder_dtor,
		uintptr(unsafe.Pointer(&args)),
	)
}
