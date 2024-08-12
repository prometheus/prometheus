#pragma once

#include <cstdint>
#if __has_include(<spanstream>)  // sanity checks..
#if __cplusplus <= 202002L
#error "Please set -std="c++2b" or similar flag for C++23 for your compiler."
#endif
#include <spanstream>
#else
#error "Your C++ Standard library doesn't implement the std::spanstream. Make sure that you use conformant Library (e.g., libstdc++ from GCC 12)"
#endif

#include "bare_bones/preprocess.h"
#include "primitives/go_slice.h"
#include "prometheus/remote_write.h"
#include "third_party/protozero/pbf_writer.hpp"
#include "wal.h"

template <class T>
concept have_segment_id = requires(const T& t) {
  { t.segment_id };
};

namespace PromPP::WAL {

// MetaInjection metedata for injection metrics.
struct MetaInjection {
  int64_t sent_at{0};
  Primitives::Go::String agent_uuid;
  Primitives::Go::String hostname;
};

template <class BasicDecoder, class Output>
class TimeseriesProtobufWriter {
 public:
  TimeseriesProtobufWriter(BasicDecoder& decoder, Output& out) : pb_message_(out), decoder_(decoder), samples_before_(decoder_.samples()) {}

  PROMPP_ALWAYS_INLINE void operator()(Reader::timeseries_type timeseries) {
    Prometheus::RemoteWrite::write_timeseries(pb_message_, timeseries);
    ++processed_series_;
  }

  PROMPP_ALWAYS_INLINE void operator()(PromPP::Primitives::LabelSetID, Reader::timeseries_type timeseries) { operator()(timeseries); }

  template <class Stats>
  PROMPP_ALWAYS_INLINE void get_statistic(Stats& stats) {
    stats.created_at = decoder_.created_at_tsns();
    stats.encoded_at = decoder_.encoded_at_tsns();
    stats.samples = decoder_.samples() - samples_before_;
    stats.series = processed_series_;
    stats.earliest_block_sample = decoder_.earliest_sample();
    stats.latest_block_sample = decoder_.latest_sample();

    if constexpr (have_segment_id<Stats>) {
      stats.segment_id = decoder_.last_processed_segment();
    }
  }

 private:
  protozero::basic_pbf_writer<Output> pb_message_;
  BasicDecoder& decoder_;
  uint64_t samples_before_;
  uint32_t processed_series_{};
};

class Decoder {
 private:
  Primitives::SnugComposites::LabelSet::DecodingTable label_set_;
  Reader reader_;

 public:
  explicit PROMPP_ALWAYS_INLINE Decoder(BasicEncoderVersion encoder_version) noexcept : reader_(label_set_, encoder_version) {}

  // decode - decoding incoming data and make protbuf.
  template <class Input, class Output, class Stats>
  PROMPP_ALWAYS_INLINE void decode(Input& in, Output& out, Stats& stats) {
    std::ispanstream inspan(std::string_view(in.data(), in.size()));
    inspan >> reader_;

    TimeseriesProtobufWriter<Reader, Output> protobuf_writer(reader_, out);
    reader_.process_segment(protobuf_writer);
    protobuf_writer.get_statistic(stats);
  }

  // decode_to_hashdex decoding incoming data and add to hashdex.
  template <class Input, class Hashdex, class Stats>
  PROMPP_ALWAYS_INLINE void decode_to_hashdex(Input& in, Hashdex& hx, Stats& stats) {
    std::ispanstream inspan(std::string_view(in.data(), in.size()));
    inspan >> reader_;

    hx.presharding(reader_);

    // write stats
    stats.created_at = reader_.created_at_tsns();
    stats.encoded_at = reader_.encoded_at_tsns();
    stats.samples = hx.size();
    stats.series = hx.series();
    stats.earliest_block_sample = reader_.earliest_sample();
    stats.latest_block_sample = reader_.latest_sample();
    if constexpr (have_segment_id<Stats>) {
      stats.segment_id = reader_.last_processed_segment();
    }
  }

  // decode_to_hashdex decoding incoming data and add to hashdex with metadata.
  template <class Input, class Hashdex, class Stats, class Meta>
  PROMPP_ALWAYS_INLINE void decode_to_hashdex(Input& in, Hashdex& hx, Stats& stats, Meta& meta) {
    std::ispanstream inspan(std::string_view(in.data(), in.size()));
    inspan >> reader_;

    hx.presharding(reader_, meta);

    // write stats
    stats.created_at = reader_.created_at_tsns();
    stats.encoded_at = reader_.encoded_at_tsns();
    stats.samples = hx.size();
    stats.series = hx.series();
    stats.earliest_block_sample = reader_.earliest_sample();
    stats.latest_block_sample = reader_.latest_sample();
    if constexpr (have_segment_id<Stats>) {
      stats.segment_id = reader_.last_processed_segment();
    }
  }

  // decode_dry - decoding incoming data without protbuf.
  template <class Input, class Stats>
  PROMPP_ALWAYS_INLINE void decode_dry(Input& in, Stats* stats) {
    std::ispanstream inspan(std::string_view(in.data(), in.size()));
    inspan >> reader_;
    reader_.process_segment([](uint32_t, int64_t, double) PROMPP_LAMBDA_INLINE {});
    stats->segment_id = reader_.last_processed_segment();
  }

  // restore_from_stream - restore the decoder state to the required segment from the file.
  template <class Input, class Stats>
  PROMPP_ALWAYS_INLINE void restore_from_stream(Input& in, uint32_t segment_id, Stats* stats) {
    std::ispanstream inspan(std::string_view(in.data(), in.size()));
    while (reader_.last_processed_segment() != segment_id) {
      inspan >> reader_;
      if (inspan.eof()) {
        break;
      }
      stats->offset = inspan.tellg();
      reader_.process_segment([](uint32_t, uint64_t, double) PROMPP_LAMBDA_INLINE {});
    }

    stats->segment_id = reader_.last_processed_segment();
  }
};

}  // namespace PromPP::WAL
