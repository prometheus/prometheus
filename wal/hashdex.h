#pragma once

#include <cstdint>
#include <string_view>

#include "bare_bones/exception.h"
#include "bare_bones/preprocess.h"
#include "bare_bones/vector.h"
#include "concepts.h"
#include "heartbeat_metrics_inserter.h"
#include "primitives/go_model.h"
#include "primitives/go_slice.h"
#include "primitives/primitives.h"
#include "prometheus/remote_write.h"
#include "wal.h"

#include "third_party/protozero/pbf_reader.hpp"

namespace PromPP::WAL {

template <class LabelSet>
void set_cluser_and_replica_values(const LabelSet& label_set, std::string_view& cluster, std::string_view& replica) {
  for (const auto& [name, value] : label_set) {
    if (name == "cluster") {
      cluster = value;
    }
    if (name == "__replica__") {
      replica = value;
    }
  }
}

struct HashdexLimits {
  uint32_t max_label_name_length;
  uint32_t max_label_value_length;
  uint32_t max_label_names_per_timeseries;
  size_t max_timeseries_count;
};

class GoModelHashdex {
  class Item {
    size_t hash_;
    const PromPP::Primitives::Go::TimeSeries& go_time_series_;

   public:
    PROMPP_ALWAYS_INLINE Item(size_t hash, const PromPP::Primitives::Go::TimeSeries& go_time_series) : hash_(hash), go_time_series_(go_time_series) {}

    PROMPP_ALWAYS_INLINE size_t hash() const { return hash_; }

    template <class Timeseries>
    PROMPP_ALWAYS_INLINE void read(Timeseries& timeseries) const {
      PromPP::Primitives::Go::read_timeseries(go_time_series_, timeseries);
    }
  };

 private:
  BareBones::Vector<Item> items_;
  std::string_view replica_;
  std::string_view cluster_;
  const HashdexLimits limits_{};

 public:
  using iterator_category = BareBones::Vector<Item>::iterator_category;
  using value_type = const Item;
  using const_iterator = BareBones::Vector<Item>::const_iterator;

  inline __attribute__((always_inline)) GoModelHashdex() noexcept {}
  explicit inline __attribute__((always_inline)) GoModelHashdex(const HashdexLimits& limits) noexcept : limits_(limits) {}
  inline __attribute__((always_inline)) ~GoModelHashdex(){};

  constexpr const std::string_view replica() const noexcept { return replica_; }
  constexpr const std::string_view cluster() const noexcept { return cluster_; }
  constexpr const HashdexLimits& limits() const noexcept { return limits_; }

  inline __attribute__((always_inline)) void presharding(PromPP::Primitives::Go::SliceView<PromPP::Primitives::Go::TimeSeries>& go_time_series_slice) {
    if (limits_.max_timeseries_count && std::size(go_time_series_slice) > limits_.max_timeseries_count) {
      throw BareBones::Exception(0x1806e61dde4a3d6f, "Timeseries limit exceeded");
    }

    items_.reserve(std::size(items_) + std::size(go_time_series_slice));
    PromPP::Primitives::LabelViewSet label_set;
    PromPP::Primitives::Go::LabelSetLimits limits = {
        limits_.max_label_name_length,
        limits_.max_label_value_length,
        limits_.max_label_names_per_timeseries,
    };
    bool first = true;
    for (auto& go_time_series : go_time_series_slice) {
      PromPP::Primitives::Go::read_label_set(go_time_series.label_set, label_set, limits);
      items_.emplace_back(hash_value(label_set), go_time_series);
      if (first) {
        first = false;
        set_cluser_and_replica_values(label_set, cluster_, replica_);
      }
      label_set.clear();
    }
  }

  inline __attribute__((always_inline)) const_iterator begin() const noexcept { return std::begin(items_); }
  inline __attribute__((always_inline)) const_iterator end() const noexcept { return std::end(items_); }
};

class ProtobufHashdex {
  class Item {
    size_t hash_;
    std::string_view data_;

   public:
    PROMPP_ALWAYS_INLINE explicit Item(size_t hash, std::string_view data) : hash_(hash), data_(data) {}
    PROMPP_ALWAYS_INLINE size_t hash() const { return hash_; }

    template <class Timeseries>
    PROMPP_ALWAYS_INLINE void read(Timeseries& timeseries) const {
      Prometheus::RemoteWrite::read_timeseries(protozero::pbf_reader(data_), timeseries);
    }
  };

 private:
  BareBones::Vector<Item> items_;
  std::string_view replica_;
  std::string_view cluster_;
  const HashdexLimits limits_{};  // no limits on default.

 public:
  using iterator_category = BareBones::Vector<const Item>::iterator_category;
  using value_type = const Item;
  using const_iterator = BareBones::Vector<const Item>::const_iterator;

  inline __attribute__((always_inline)) ProtobufHashdex() noexcept {}
  explicit inline __attribute__((always_inline)) ProtobufHashdex(const HashdexLimits& limits) noexcept : limits_(limits) {}
  inline __attribute__((always_inline)) ~ProtobufHashdex(){};

  constexpr const std::string_view replica() const noexcept { return replica_; }
  constexpr const std::string_view cluster() const noexcept { return cluster_; }
  constexpr const HashdexLimits& limits() const noexcept { return limits_; }

  // presharding - from protobuf make presharding slice with hash end proto.
  inline __attribute__((always_inline)) void presharding(const char* proto_data, size_t proto_len) {
    size_t current_timeseries_n = 0;
    Prometheus::RemoteWrite::PbLabelSetMemoryLimits pb_limits = {
        limits_.max_label_name_length,
        limits_.max_label_value_length,
        limits_.max_label_names_per_timeseries,
        limits_.max_timeseries_count,
    };
    Primitives::LabelViewSet label_set;
    protozero::pbf_reader pb(std::string_view{proto_data, proto_len});
    bool first = true;
    try {
      while (pb.next(1)) {
        if (pb_limits.max_timeseries_count && current_timeseries_n >= pb_limits.max_timeseries_count) {
          throw BareBones::Exception(0xdedb5b24d946cc4d, "Max Timeseries count limit exceeded");
          break;
        }
        auto pb_view = pb.get_view();
        Prometheus::RemoteWrite::read_timeseries_label_set(protozero::pbf_reader{pb_view}, label_set, pb_limits);
        items_.emplace_back(hash_value(label_set), pb_view);
        if (first) {
          first = false;
          set_cluser_and_replica_values(label_set, cluster_, replica_);
        }
        label_set.clear();
        current_timeseries_n++;
      }
    } catch (protozero::exception& e) {
      throw BareBones::Exception(0xbe40bda82f01b869, "Protobuf parsing timeseries exception: %s", e.what());
    }
  };

  inline __attribute__((always_inline)) const_iterator begin() const noexcept { return std::begin(items_); }
  inline __attribute__((always_inline)) const_iterator end() const noexcept { return std::end(items_); }
};

class BasicDecoderHashdex {
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
  std::string_view replica_;
  std::string_view cluster_;
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
  PROMPP_ALWAYS_INLINE void presharding(BasicDecoder<>& decoder) {
    BasicDecoder<>::label_set_value_type ls_view;  // composite_type
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
    if (items_.size()) [[likely]] {
      set_cluser_and_replica_values(ls_view, cluster_, replica_);
    }
  }

  // presharding from decoder make presharding slice with hash and TimeseriesSemiview with metadata.
  PROMPP_ALWAYS_INLINE void presharding(BasicDecoder<>& decoder, const MetaInjection& meta) {
    presharding(decoder);
    metric_injection(meta);
  }

  // write_stats write sharding stats.
  template <class Stats>
  PROMPP_ALWAYS_INLINE void write_stats(BasicDecoder<>& decoder, Stats& stats) {
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

  PROMPP_ALWAYS_INLINE const_iterator begin() const noexcept { return std::begin(items_); }
  PROMPP_ALWAYS_INLINE const_iterator end() const noexcept { return std::end(items_); }
  PROMPP_ALWAYS_INLINE uint32_t series() const noexcept { return series_; }
  PROMPP_ALWAYS_INLINE size_t size() const noexcept { return items_.size(); }
  constexpr const std::string_view replica() const noexcept { return replica_; }
  constexpr const std::string_view cluster() const noexcept { return cluster_; }
};

}  // namespace PromPP::WAL
