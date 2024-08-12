#include <cstdint>

#include "_helpers.hpp"
#include "wal_decoder.h"

#include "primitives/go_slice.h"
#include "primitives/go_slice_protozero.h"
#include "wal/decoder.h"

extern "C" void prompp_wal_decoder_ctor(void* args, void* res) {
  struct Arguments {
    uint8_t encoder_version;
  };
  using Result = struct {
    PromPP::WAL::Decoder* decoder;
  };

  auto* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();
  out->decoder = new PromPP::WAL::Decoder(static_cast<PromPP::WAL::BasicEncoderVersion>(in->encoder_version));
}

extern "C" void prompp_wal_decoder_dtor(void* args) {
  struct Arguments {
    PromPP::WAL::Decoder* decoder;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  delete in->decoder;
}

extern "C" void prompp_wal_decoder_decode(void* args, void* res) {
  struct Arguments {
    PromPP::WAL::Decoder* decoder;
    PromPP::Primitives::Go::SliceView<char> segment;
  };
  using Result = struct {
    int64_t created_at;
    int64_t encoded_at;
    uint32_t samples;
    uint32_t series;
    uint32_t segment_id;
    PromPP::Primitives::Timestamp earliest_block_sample;
    PromPP::Primitives::Timestamp latest_block_sample;
    PromPP::Primitives::Go::Slice<char> protobuf;
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    in->decoder->decode(in->segment, out->protobuf, *out);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

extern "C" void prompp_wal_decoder_decode_to_hashdex(void* args, void* res) {
  struct Arguments {
    PromPP::WAL::Decoder* decoder;
    PromPP::Primitives::Go::SliceView<char> segment;
  };
  using Result = struct {
    int64_t created_at;
    int64_t encoded_at;
    uint32_t samples;
    uint32_t series;
    uint32_t segment_id;
    PromPP::Primitives::Timestamp earliest_block_sample;
    PromPP::Primitives::Timestamp latest_block_sample;
    HashdexVariant* hashdex_variant;
    PromPP::Primitives::Go::String cluster;
    PromPP::Primitives::Go::String replica;
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    out->hashdex_variant = new HashdexVariant{std::in_place_index<HashdexType::decoder>};
    auto& hashdex = std::get<PromPP::WAL::BasicDecoderHashdex>(*out->hashdex_variant);
    in->decoder->decode_to_hashdex(in->segment, hashdex, *out);
    auto cluster = hashdex.cluster();
    out->cluster.reset_to(cluster.data(), cluster.size());
    auto replica = hashdex.replica();
    out->replica.reset_to(replica.data(), replica.size());
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

extern "C" void prompp_wal_decoder_decode_to_hashdex_with_metric_injection(void* args, void* res) {
  struct Arguments {
    PromPP::WAL::Decoder* decoder;
    PromPP::WAL::MetaInjection* meta;
    PromPP::Primitives::Go::SliceView<char> segment;
  };
  using Result = struct {
    int64_t created_at;
    int64_t encoded_at;
    uint32_t samples;
    uint32_t series;
    uint32_t segment_id;
    PromPP::Primitives::Timestamp earliest_block_sample;
    PromPP::Primitives::Timestamp latest_block_sample;
    HashdexVariant* hashdex_variant;
    PromPP::Primitives::Go::String cluster;
    PromPP::Primitives::Go::String replica;
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    out->hashdex_variant = new HashdexVariant{std::in_place_index<HashdexType::decoder>};
    auto& hashdex = std::get<PromPP::WAL::BasicDecoderHashdex>(*out->hashdex_variant);
    in->decoder->decode_to_hashdex(in->segment, hashdex, *out, *in->meta);
    auto cluster = hashdex.cluster();
    out->cluster.reset_to(cluster.data(), cluster.size());
    auto replica = hashdex.replica();
    out->replica.reset_to(replica.data(), replica.size());
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

extern "C" void prompp_wal_decoder_decode_dry(void* args, void* res) {
  struct Arguments {
    PromPP::WAL::Decoder* decoder;
    PromPP::Primitives::Go::SliceView<char> segment;
  };
  struct Result {
    uint32_t segment_id;
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    in->decoder->decode_dry(in->segment, out);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

extern "C" void prompp_wal_decoder_restore_from_stream(void* args, void* res) {
  struct Arguments {
    PromPP::WAL::Decoder* decoder;
    PromPP::Primitives::Go::SliceView<char> stream;
    uint32_t segment_id;
  };
  struct Result {
    size_t offset = 0;
    uint32_t segment_id;
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    in->decoder->restore_from_stream(in->stream, in->segment_id, out);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}
