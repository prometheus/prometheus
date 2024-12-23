#include <cstdint>

#include "_helpers.hpp"
#include "wal_decoder.h"

#include "head/lss.h"
#include "primitives/go_slice.h"
#include "primitives/go_slice_protozero.h"
#include "wal/decoder.h"
#include "wal/output_decoder.h"

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
    out->hashdex_variant = new HashdexVariant{std::in_place_index<HashdexType::kDecoder>};
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
  struct MetaInjection {
    std::chrono::system_clock::time_point now;
    std::chrono::nanoseconds sent_at{0};
    PromPP::Primitives::Go::String agent_uuid;
    PromPP::Primitives::Go::String hostname;

    [[nodiscard]] explicit PROMPP_ALWAYS_INLINE operator PromPP::WAL::BasicDecoderHashdex::MetaInjection() const noexcept {
      return PromPP::WAL::BasicDecoderHashdex::MetaInjection{
          .now = now,
          .sent_at = sent_at,
          .agent_uuid = static_cast<std::string_view>(agent_uuid),
          .hostname = static_cast<std::string_view>(hostname),
      };
    }
  };

  struct Arguments {
    PromPP::WAL::Decoder* decoder;
    MetaInjection* meta;
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
    out->hashdex_variant = new HashdexVariant{std::in_place_index<HashdexType::kDecoder>};
    auto& hashdex = std::get<PromPP::WAL::BasicDecoderHashdex>(*out->hashdex_variant);
    in->decoder->decode_to_hashdex(in->segment, hashdex, *out, static_cast<PromPP::WAL::BasicDecoderHashdex::MetaInjection>(*in->meta));
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

//
// OutputDecoder
//

using entrypoint::head::LssVariantPtr;

extern "C" void prompp_wal_output_decoder_ctor(void* args, void* res) {
  struct Arguments {
    PromPP::Primitives::Go::SliceView<std::pair<PromPP::Primitives::Go::String, PromPP::Primitives::Go::String>> external_labels;
    PromPP::Prometheus::Relabel::StatelessRelabeler* stateless_relabeler;
    LssVariantPtr output_lss;
    uint8_t encoder_version;
  };
  using Result = struct {
    PromPP::WAL::OutputDecoder* decoder;
  };

  auto* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();
  auto& output_lss = std::get<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap>(*in->output_lss);
  out->decoder =
      new PromPP::WAL::OutputDecoder(*in->stateless_relabeler, output_lss, in->external_labels, static_cast<PromPP::WAL::BasicEncoderVersion>(in->encoder_version));
}

extern "C" void prompp_wal_output_decoder_dtor(void* args) {
  struct Arguments {
    PromPP::WAL::OutputDecoder* decoder;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  delete in->decoder;
}

extern "C" void prompp_wal_output_decoder_dump_to(void* args, void* res) {
  struct Arguments {
    PromPP::WAL::OutputDecoder* decoder;
  };

  using Result = struct {
    PromPP::Primitives::Go::Slice<char> dump;
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    PromPP::Primitives::Go::BytesStream bytes_stream{&out->dump};
    in->decoder->dump_to(bytes_stream);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

extern "C" void prompp_wal_output_decoder_load_from(void* args, void* res) {
  struct Arguments {
    PromPP::Primitives::Go::SliceView<char> dump;
    PromPP::WAL::OutputDecoder* decoder;
  };

  using Result = struct {
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    std::ispanstream bytes_stream(static_cast<std::string_view>(in->dump));
    in->decoder->load_from(bytes_stream);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

extern "C" void prompp_wal_output_decoder_decode(void* args, void* res) {
  struct Arguments {
    PromPP::Primitives::Go::SliceView<char> segment;
    PromPP::WAL::OutputDecoder* decoder;
    int64_t lower_limit_timestamp;
  };

  struct Result {
    int64_t max_timestamp{};
    uint64_t dropped_sample_count{};
    PromPP::Primitives::Go::Slice<PromPP::WAL::RefSample> ref_samples;
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    std::ispanstream{static_cast<std::string_view>(in->segment)} >> *in->decoder;
    in->decoder->process_segment([in, out](PromPP::Primitives::LabelSetID ls_id, PromPP::Primitives::Timestamp ts, PromPP::Primitives::Sample::value_type v,
                                           bool is_dropped) PROMPP_LAMBDA_INLINE {
      if (is_dropped) {
        // skip dropped sample
        ++out->dropped_sample_count;
        return;
      }

      if (ts < in->lower_limit_timestamp) {
        // skip sample lower limit timestamp
        ++out->dropped_sample_count;
        return;
      }

      if (out->max_timestamp < ts) {
        out->max_timestamp = ts;
      }

      out->ref_samples.emplace_back(ls_id, ts, v);
    });
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

//
// ProtobufEncoder
//

extern "C" void prompp_wal_protobuf_encoder_ctor(void* args, void* res) {
  struct Arguments {
    PromPP::Primitives::Go::SliceView<LssVariantPtr> output_lsses;
  };
  using Result = struct {
    PromPP::WAL::ProtobufEncoder* encoder;
  };

  auto* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  std::vector<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap*> output_lsses;
  output_lsses.reserve(in->output_lsses.size());
  for (const auto& output_lss : in->output_lsses) {
    output_lsses.push_back(&std::get<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap>(*output_lss));
  }

  out->encoder = new PromPP::WAL::ProtobufEncoder(std::move(output_lsses));
}

extern "C" void prompp_wal_protobuf_encoder_dtor(void* args) {
  struct Arguments {
    PromPP::WAL::ProtobufEncoder* encoder;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  delete in->encoder;
}

extern "C" void prompp_wal_protobuf_encoder_encode(void* args, void* res) {
  struct Arguments {
    PromPP::Primitives::Go::SliceView<PromPP::WAL::ShardRefSample*> batch;
    PromPP::Primitives::Go::Slice<PromPP::Primitives::Go::Slice<char>> out_slices;
    PromPP::Primitives::Go::Slice<PromPP::WAL::ProtobufEncoderStats> stats;
    PromPP::WAL::ProtobufEncoder* encoder;
  };

  using Result = struct {
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    in->encoder->encode(in->batch, in->out_slices, in->stats);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}
