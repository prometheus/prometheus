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

#include "prometheus/remote_write.h"
#include "wal.h"

#include "third_party/protozero/basic_pbf_writer.hpp"

namespace PromPP::WAL {
class Decoder {
 private:
  Reader reader_;

 public:
  inline __attribute__((always_inline)) Decoder() noexcept {}

  // decode - decoding incoming data and make protbuf.
  template <class Input, class Output, class Stats>
  inline __attribute__((always_inline)) void decode(Input& in, Output& out, Stats* stats) {
    std::ispanstream inspan(std::string_view(in.data(), in.size()));
    inspan >> reader_;

    protozero::basic_pbf_writer<Output> pb_message(out);
    uint32_t processed_series = 0;
    uint64_t samples_before = reader_.samples();
    reader_.process_segment([&](Reader::timeseries_type timeseries) {
      Prometheus::RemoteWrite::write_timeseries(pb_message, timeseries);
      ++processed_series;
    });

    stats->created_at = reader_.created_at_tsns();
    stats->encoded_at = reader_.encoded_at_tsns();
    stats->samples = reader_.samples() - samples_before;
    stats->series = processed_series;
    stats->segment_id = reader_.last_processed_segment();
  }

  // decode_dry - decoding incoming data without protbuf.
  template <class Input>
  inline __attribute__((always_inline)) void decode_dry(Input& in) {
    std::ispanstream inspan(std::string_view(in.data(), in.size()));
    inspan >> reader_;
    reader_.process_segment([](uint32_t ls_id, uint64_t ts, double v) {});
  }

  // restore_from_stream - restore the decoder state to the required segment from the file.
  template <class Input, class Stats>
  inline __attribute__((always_inline)) void restore_from_stream(Input& in, uint32_t segment_id, Stats* stats) {
    std::ispanstream inspan(std::string_view(in.data(), in.size()));
    while (reader_.last_processed_segment() != segment_id) {
      inspan >> reader_;
      if (inspan.eof()) {
        break;
      }
      stats->offset = inspan.tellg();
      reader_.process_segment([](uint32_t ls_id, uint64_t ts, double v) {});
    }

    stats->segment_id = reader_.last_processed_segment();
  }

  inline __attribute__((always_inline)) ~Decoder(){};
};
};  // namespace PromPP::WAL
