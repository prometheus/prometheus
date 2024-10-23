#include <variant>

#include "_helpers.hpp"
#include "primitives/go_slice.h"
#include "wal/encoder.h"
#include "wal/decoder.h"
#include "wal/hashdex.h"
#include "wal/wal.h"
#include "head/lss.h"
#include "head_wal.h"

/**
 * @brief Construct a new WAL EncoderLightweight
 *
 * @param args {
 *     shardID            uint16  // shard number
 *     logShards          uint8   // logarithm to the base 2 of total shards count
 *     lss                uintptr // lss
 * }
 * @param res {
 *     encoderLightweight uintptr // pointer to constructed encoder
 * }
 */
extern "C" void prompp_head_wal_encoder_ctor(void* args, void* res) {
  using entrypoint::head::LssVariantPtr;
  using BasicEncoder = PromPP::WAL::BasicEncoder<entrypoint::head::QueryableEncodingBimap&>;
  using Encoder = PromPP::WAL::GenericEncoder<BasicEncoder>;

  struct Arguments {
    uint16_t shard_id;
    uint8_t log_shards;
    LssVariantPtr lss;
  };

  struct Result {
    Encoder* encoder;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  auto& lss = std::get<entrypoint::head::QueryableEncodingBimap>(*in->lss);
  out->encoder = new Encoder(in->shard_id, in->log_shards, lss, in->shard_id, in->log_shards);
}

/**
 * @brief Destroy EncoderLightweight
 *
 * @param args {
 *     encoderLightweight uintptr // pointer to constructed encoder
 * }
 */
extern "C" void prompp_head_wal_encoder_dtor(void* args) {
  using BasicEncoder = PromPP::WAL::BasicEncoder<entrypoint::head::QueryableEncodingBimap&>;
  using Encoder = PromPP::WAL::GenericEncoder<BasicEncoder>;

  struct Arguments {
    Encoder* encoder;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  delete in->encoder;
}


/**
 * @brief Add inner series to current segment
 *
 * @param args {
 *     incomingInnerSeries []*InnerSeries // go slice with incoming InnerSeries;
 *     encoderLightweight  uintptr        // pointer to constructed encoder;
 * }
 * @param res {
 *     earliestTimestamp   int64          // minimal sample timestamp in segment
 *     latestTimestamp     int64          // maximal sample timestamp in segment
 *     allocatedMemory     uint64         // size of allocated memory for label sets;
 *     samples             uint32         // number of samples in segment
 *     series              uint32         // number of series in segment
 *     remainderSize       uint32         // rest of internal buffers capacity
 *     error               []byte         // error string if thrown
 * }
 */
extern "C" void prompp_head_wal_encoder_add_inner_series(void* args, void* res) {
  using BasicEncoder = PromPP::WAL::BasicEncoder<entrypoint::head::QueryableEncodingBimap&>;
  using Encoder = PromPP::WAL::GenericEncoder<BasicEncoder>;

  struct Arguments {
    PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::InnerSeries*> incoming_inner_series;
    Encoder* encoder;
  };
  struct Result {
    int64_t earliest_timestamp;
    int64_t latest_timestamp;
    size_t allocated_memory;
    uint32_t samples;
    uint32_t series;
    uint32_t remainder_size;
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    in->encoder->add_inner_series(in->incoming_inner_series, out);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

/**
 * @brief Flush segment
 *
 * @param args {
 *     encoderLightweight uintptr // pointer to constructed encoder
 * }
 * @param res {
 *     earliestTimestamp  int64   // minimal sample timestamp in segment
 *     latestTimestamp    int64   // maximal sample timestamp in segment
 *     allocatedMemory    uint64  // size of allocated memory for label sets;
 *     samples            uint32  // number of samples in segment
 *     series             uint32  // number of series in segment
 *     remainderSize      uint32  // rest of internal buffers capacity
 *     error              []byte  // error string if thrown
 * }
 */
extern "C" void prompp_head_wal_encoder_finalize(void* args, void* res) {
  using BasicEncoder = PromPP::WAL::BasicEncoder<entrypoint::head::QueryableEncodingBimap&>;
  using Encoder = PromPP::WAL::GenericEncoder<BasicEncoder>;

  struct Arguments {
    Encoder* encoder;
  };
  struct Result {
    int64_t earliest_timestamp;
    int64_t latest_timestamp;
    size_t allocated_memory;
    uint32_t samples;
    uint32_t series;
    uint32_t remainder_size;
    PromPP::Primitives::Go::Slice<char> segment;
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  auto out_stream = PromPP::Primitives::Go::BytesStream(&out->segment);

  try {
    in->encoder->finalize(out, out_stream);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

extern "C" void prompp_head_wal_decoder_ctor(void* args, void* res) {
  using entrypoint::head::LssVariantPtr;
  using Decoder = PromPP::WAL::GenericDecoder<entrypoint::head::QueryableEncodingBimap&>;
  using EncoderVersion = PromPP::WAL::BasicEncoderVersion;

  struct Arguments {
    LssVariantPtr lss;
    EncoderVersion encoder_version;
  };
  using Result = struct {
    Decoder* decoder;
  };

  auto* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();
  auto& lss = std::get<entrypoint::head::QueryableEncodingBimap>(*in->lss);
  out->decoder = new Decoder(lss, in->encoder_version);
}

extern "C" void prompp_head_wal_decoder_dtor(void* args) {
  using Decoder = PromPP::WAL::GenericDecoder<entrypoint::head::QueryableEncodingBimap&>;
  struct Arguments {
    Decoder* decoder;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  delete in->decoder;
}

extern "C" void prompp_head_wal_decoder_decode(void* args, void* res) {
  using Decoder = PromPP::WAL::GenericDecoder<entrypoint::head::QueryableEncodingBimap&>;

  struct Arguments {
    Decoder* decoder;
    PromPP::Primitives::Go::SliceView<char> segment;
    PromPP::Prometheus::Relabel::InnerSeries *inner_series;
  };
  using Result = struct {
    int64_t created_at;
    int64_t encoded_at;
    uint32_t samples;
    uint32_t series;
    uint32_t segment_id;
    PromPP::Primitives::Timestamp earliest_block_sample;
    PromPP::Primitives::Timestamp latest_block_sample;
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();


  try {
    in->inner_series->clear();

    in->decoder->decode_to_inner_series(in->segment, *in->inner_series, out);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

extern "C" void prompp_head_wal_decoder_create_encoder(void* args, void* res) {
  using Decoder = PromPP::WAL::GenericDecoder<entrypoint::head::QueryableEncodingBimap&>;
  using BasicEncoder = PromPP::WAL::BasicEncoder<entrypoint::head::QueryableEncodingBimap&>;
  using Encoder = PromPP::WAL::GenericEncoder<BasicEncoder>;

  struct Arguments {
    Decoder* decoder;
  };

  struct Result {
    Encoder* encoder;
  };

  auto* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  out->encoder = new Encoder(PromPP::WAL::create_encoder_from_decoder<Encoder, Decoder>(*in->decoder));
}
