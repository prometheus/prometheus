#pragma once

#include <chrono>

#include "primitives/primitives.h"
#include "prometheus/hashdex.h"
#include "wal/concepts.h"
#include "wal/heartbeat_metrics_inserter.h"
#include "wal/wal.h"

namespace PromPP::WAL::hashdex {

class BasicDecoder : public Prometheus::hashdex::Abstract {
 public:
  struct MetaInjection {
    std::chrono::system_clock::time_point now;
    std::chrono::nanoseconds sent_at{0};
    std::string_view agent_uuid;
    std::string_view hostname;
  };

  class Item {
    size_t hash_;
    Primitives::TimeseriesSemiview data_;

   public:
    template <class LabelSet>
    PROMPP_ALWAYS_INLINE explicit Item(const LabelSet& ls, Primitives::Sample sample) : hash_(hash_value(ls)), data_(ls, {sample}) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE size_t hash() const { return hash_; }

    template <class Timeseries>
    PROMPP_ALWAYS_INLINE void read(Timeseries& timeseries) const {
      timeseries = data_;
    }
  };

 private:
  std::vector<Item> items_;
  uint32_t series_{0};

  // metrics_injection injection of additional metrics with Heartbeat.
  void metric_injection(const MetaInjection& meta) {
    HeartbeatMetricsInserter inserter({.agent_uuid = meta.agent_uuid, .hostname = meta.hostname});
    inserter.insert(meta.now, meta.sent_at, [this](const auto& timeseries) PROMPP_LAMBDA_INLINE {
      items_.emplace_back(timeseries.label_set(), timeseries.samples()[0]);
      ++series_;
    });
  }

 public:
  using const_iterator = std::vector<Item>::const_iterator;

  // presharding from decoder make presharding slice with hash and TimeseriesSemiview.
  PROMPP_ALWAYS_INLINE void presharding(WAL::BasicDecoder<>& decoder) {
    WAL::BasicDecoder<>::label_set_value_type ls_view;  // composite_type
    Primitives::LabelSetID last_ls_id = std::numeric_limits<Primitives::LabelSetID>::max();
    const auto& label_sets = decoder.label_sets();
    decoder.process_segment([&](Primitives::LabelSetID ls_id, Primitives::Timestamp ts, double value) PROMPP_LAMBDA_INLINE {
      if (last_ls_id != ls_id) {
        ls_view = label_sets[ls_id];
        last_ls_id = ls_id;
        ++series_;
      }
      items_.emplace_back(ls_view, Primitives::Sample(ts, value));
    });
    if (!items_.empty()) [[likely]] {
      set_cluser_and_replica_values(ls_view);
    }
  }

  // presharding from decoder make presharding slice with hash and TimeseriesSemiview with metadata.
  PROMPP_ALWAYS_INLINE void presharding(WAL::BasicDecoder<>& decoder, const MetaInjection& meta) {
    presharding(decoder);
    metric_injection(meta);
  }

  // write_stats write sharding stats.
  template <class Stats>
  PROMPP_ALWAYS_INLINE void write_stats(WAL::BasicDecoder<>& decoder, Stats& stats) {
    stats.created_at = decoder.created_at_tsns();
    stats.encoded_at = decoder.encoded_at_tsns();
    stats.samples = items_.size();
    stats.series = series_;
    stats.earliest_block_sample = decoder.earliest_sample();
    stats.latest_block_sample = decoder.latest_sample();
    if constexpr (concepts::has_field_segment_id<Stats>) {
      stats.segment_id = decoder.last_processed_segment();
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const_iterator begin() const noexcept { return std::begin(items_); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const_iterator end() const noexcept { return std::end(items_); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t series() const noexcept { return series_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t size() const noexcept { return items_.size(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const auto& metrics() const noexcept { return items_; }
  [[nodiscard]] static PROMPP_ALWAYS_INLINE auto metadata() noexcept {
    struct Stub {};
    return Stub{};
  }
};

static_assert(Prometheus::hashdex::HashdexInterface<BasicDecoder>);

}  // namespace PromPP::WAL::hashdex