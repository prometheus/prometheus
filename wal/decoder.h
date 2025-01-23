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
#include "concepts.h"
#include "prometheus/remote_write.h"
#include "third_party/protozero/pbf_writer.hpp"
#include "wal.h"

namespace PromPP::WAL {

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

    if constexpr (concepts::has_field_segment_id<Stats>) {
      stats.segment_id = decoder_.last_processed_segment();
    }
  }

 private:
  protozero::basic_pbf_writer<Output> pb_message_;
  BasicDecoder& decoder_;
  uint64_t samples_before_;
  uint32_t processed_series_{};
};

template <typename LSS = Primitives::SnugComposites::LabelSet::DecodingTable>
class GenericDecoder {
  using Decoder = BasicDecoder<std::remove_reference_t<LSS>>;

  LSS label_set_;
  Decoder decoder_;

 public:
  explicit PROMPP_ALWAYS_INLINE GenericDecoder(BasicEncoderVersion encoder_version) noexcept : decoder_(label_set_, encoder_version) {}
  explicit PROMPP_ALWAYS_INLINE GenericDecoder(LSS& lss, BasicEncoderVersion encoder_version) : label_set_{lss}, decoder_(label_set_, encoder_version) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE const Decoder& decoder() const noexcept { return decoder_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE LSS& label_set() const noexcept { return label_set_; }

  // decode - decoding incoming data and make protbuf.
  template <class Input, class Output, class Stats>
  PROMPP_ALWAYS_INLINE void decode(Input& in, Output& out, Stats& stats) {
    std::ispanstream inspan(std::string_view(in.data(), in.size()));
    inspan >> decoder_;

    TimeseriesProtobufWriter<Reader, Output> protobuf_writer(decoder_, out);
    decoder_.process_segment(protobuf_writer);
    protobuf_writer.get_statistic(stats);
  }

  template <class Input, class SegmentProcessor>
    requires std::is_invocable_v<SegmentProcessor, Primitives::LabelSetID, Primitives::Timestamp, double>
  PROMPP_ALWAYS_INLINE void decode(Input& in, SegmentProcessor&& processor) {
    std::ispanstream(std::string_view(in.data(), in.size())) >> decoder_;

    decoder_.process_segment(processor);
  }

  // decode_to_hashdex decoding incoming data and add to hashdex with metadata.
  template <class Input, class Hashdex, class Stats, class... PreshardingArgs>
  PROMPP_ALWAYS_INLINE void decode_to_hashdex(Input& in, Hashdex& hx, Stats& stats, PreshardingArgs&&... presharding_args) {
    std::ispanstream inspan(std::string_view(in.data(), in.size()));
    inspan >> decoder_;

    hx.presharding(decoder_, std::forward<PreshardingArgs>(presharding_args)...);
    hx.write_stats(decoder_, stats);
  }

  template <class Input, class InnerSeriesContainer, class Stats>
  PROMPP_ALWAYS_INLINE void decode_to_inner_series(Input& in, InnerSeriesContainer& container, [[maybe_unused]] Stats* stats) {
    std::ispanstream inspan(std::string_view(in.data(), in.size()));
    inspan >> decoder_;
    BareBones::Vector<Primitives::Sample> samples;
    uint32_t last_ls_id = std::numeric_limits<uint32_t>::max();
    decoder_.process_segment([&last_ls_id, &samples, &container](uint32_t ls_id, int64_t ts, double v) PROMPP_LAMBDA_INLINE {
      if (ls_id != last_ls_id) {
        if (!samples.empty()) {
          container.emplace_back(samples, last_ls_id);
        }
        samples.clear();
        last_ls_id = ls_id;
      }
      samples.emplace_back(ts, v);
    });

    if (!samples.empty()) {
      container.emplace_back(samples, last_ls_id);
    }
  }

  // decode_dry - decoding incoming data without protbuf.
  template <class Input, class Stats>
  PROMPP_ALWAYS_INLINE void decode_dry(Input& in, Stats* stats) {
    std::ispanstream inspan(std::string_view(in.data(), in.size()));
    inspan >> decoder_;
    decoder_.process_segment([](uint32_t, int64_t, double) PROMPP_LAMBDA_INLINE {});
    stats->segment_id = decoder_.last_processed_segment();
  }

  // restore_from_stream - restore the decoder state to the required segment from the file.
  template <class Input, class Stats>
  PROMPP_ALWAYS_INLINE void restore_from_stream(Input& in, uint32_t segment_id, Stats* stats) {
    std::ispanstream inspan(std::string_view(in.data(), in.size()));
    while (decoder_.last_processed_segment() != segment_id) {
      inspan >> decoder_;
      if (inspan.eof()) {
        break;
      }
      stats->offset = inspan.tellg();
      decoder_.process_segment([](uint32_t, uint64_t, double) PROMPP_LAMBDA_INLINE {});
    }

    stats->segment_id = decoder_.last_processed_segment();
  }
};

using Decoder = GenericDecoder<>;
}  // namespace PromPP::WAL
