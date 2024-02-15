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
#include "prometheus/remote_write.h"
#include "third_party/protozero/pbf_writer.hpp"
#include "wal.h"

namespace PromPP::WAL {
class Decoder {
 private:
  Reader reader_;

 public:
  explicit PROMPP_ALWAYS_INLINE Decoder(BasicEncoderVersion encoder_version) noexcept : reader_(encoder_version) {}

  // decode - decoding incoming data and make protbuf.
  template <class Input, class Output, class Stats>
  PROMPP_ALWAYS_INLINE void decode(Input& in, Output& out, Stats& stats) {
    std::ispanstream inspan(std::string_view(in.data(), in.size()));
    inspan >> reader_;

    process_segment(reader_, out, stats);
  }

  template <class Output, class Stats, bool include_segment_id = true>
  PROMPP_ALWAYS_INLINE static void process_segment(Reader& reader, Output& out, Stats& stats) {
    protozero::basic_pbf_writer<Output> pb_message(out);
    uint32_t processed_series = 0;
    uint64_t samples_before = reader.samples();
    reader.process_segment([&](Reader::timeseries_type timeseries) PROMPP_LAMBDA_INLINE {
      Prometheus::RemoteWrite::write_timeseries(pb_message, timeseries);
      ++processed_series;
    });

    stats.created_at = reader.created_at_tsns();
    stats.encoded_at = reader.encoded_at_tsns();
    stats.samples = reader.samples() - samples_before;
    stats.series = processed_series;

    if constexpr (include_segment_id) {
      stats.segment_id = reader.last_processed_segment();
    }
  }

  // decode_dry - decoding incoming data without protbuf.
  template <class Input, class Stats>
  PROMPP_ALWAYS_INLINE void decode_dry(Input& in, Stats* stats) {
    std::ispanstream inspan(std::string_view(in.data(), in.size()));
    inspan >> reader_;
    reader_.process_segment([](uint32_t, uint64_t, double) PROMPP_LAMBDA_INLINE {});
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
};  // namespace PromPP::WAL
