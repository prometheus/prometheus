#pragma once

#include <cstdint>
#include <limits>

#include "bare_bones/preprocess.h"
#include "bare_bones/vector.h"
#include "primitives/go_slice.h"
#include "primitives/primitives.h"
#include "prometheus/relabeler.h"
#include "wal.h"

template <class T>
concept have_allocated_memory = requires(const T& t) {
  { t.allocated_memory };
};

namespace PromPP::WAL {
template <typename WriterType = Writer>
class GenericEncoder {
  uint16_t shard_id_;
  uint8_t log_shards_;
  WriterType writer_;
  Primitives::TimeseriesSemiview timeseries_;

  template <class Stats>
  void write_stats(Stats* stats) const {
    size_t remaining_cap = std::numeric_limits<uint32_t>::max();

    stats->earliest_timestamp = writer_.buffer().earliest_sample();
    stats->latest_timestamp = writer_.buffer().latest_sample();
    stats->samples = writer_.buffer().samples_count();
    stats->series = writer_.buffer().series_count();
    stats->remainder_size = std::min(remaining_cap, writer_.remainder_size());

    if constexpr (have_allocated_memory<Stats>) {
      stats->allocated_memory = writer_.allocated_memory();
    }
  }

 public:
  template <class... Args>
  PROMPP_ALWAYS_INLINE GenericEncoder(uint16_t shard_id, uint8_t log_shards, Args&&... args) noexcept
      :  shard_id_(shard_id), log_shards_(log_shards), writer_{std::forward<Args>(args)...} {}

  // add_wo_stalenans - add (without any stalenans) to encode incoming data(ShardedData) through C++ encoder.
  template <class Hashdex, class Stats>
  inline __attribute__((always_inline)) void add(Hashdex& hx, Stats* stats) {
    for (const auto& item : hx) {
      if ((item.hash() % (1 << log_shards_)) == shard_id_) {
        item.read(timeseries_);
        writer_.add(timeseries_, item.hash());
        timeseries_.clear();
      }
    }

    write_stats(stats);
  }

  template <class Stats, class InnerSeriesSlice = PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::InnerSeries*>>
  inline __attribute__((always_inline)) void add_inner_series(InnerSeriesSlice& incoming_inner_series,
                                                              Stats* stats) {
    std::ranges::for_each(incoming_inner_series, [&](const PromPP::Prometheus::Relabel::InnerSeries* inner_series) {
      if (inner_series == nullptr || inner_series->size() == 0) {
        return;
      }

      std::ranges::for_each(inner_series->data(), [&](const PromPP::Prometheus::Relabel::InnerSerie& inner_serie) {
        std::cout << "encoder.add_inner_series, inner_series.samples.size(): " << inner_serie.samples.size() <<  std::endl;
        writer_.add_samples_on_ls_id(inner_serie.ls_id, inner_serie.samples);
      });
    });

    write_stats(stats);
  }

  template <class Stats>
  inline __attribute__((always_inline)) void add_relabeled_series(PromPP::Prometheus::Relabel::RelabeledSeries* incoming_relabeled_series,
                                                                  PromPP::Prometheus::Relabel::RelabelerStateUpdate* relabeler_state_update,
                                                                  Stats* stats) {
    if (incoming_relabeled_series == nullptr || incoming_relabeled_series->size() == 0) {
      return;
    }

    std::ranges::for_each(incoming_relabeled_series->data(), [&](const PromPP::Prometheus::Relabel::RelabeledSerie& relabeled_serie) {
      if ((relabeled_serie.hash % (1 << log_shards_)) == shard_id_) {
        uint32_t ls_id = writer_.add_label_set(relabeled_serie.ls);
        writer_.add_samples_on_ls_id(ls_id, relabeled_serie.samples);
        relabeler_state_update->emplace_back(relabeled_serie.ls_id, ls_id);
      }
    });

    write_stats(stats);
  }

  template <class Hashdex, class Stats>
  inline __attribute__((always_inline)) Writer::SourceState add_with_stalenans(Hashdex& hx,
                                                                               Stats* stats,
                                                                               Primitives::Timestamp stale_ts,
                                                                               Writer::SourceState state) {
    auto add_many_cb = [&](auto& add_cb) {
      for (const auto& item : hx) {
        if ((item.hash() % (1 << log_shards_)) == shard_id_) {
          item.read(timeseries_);
          add_cb(timeseries_, item.hash());
          timeseries_.clear();
        }
      }
    };
    auto result = writer_.template add_many<decltype(writer_)::add_many_generator_callback_type::with_hash_value, decltype(timeseries_)>(state, stale_ts, add_many_cb);

    write_stats(stats);
    return result;
  }

  template <class Stats>
  inline __attribute__((always_inline)) void collect_source(Stats* stats, Primitives::Timestamp stale_ts, Writer::SourceState state) {
    constexpr auto add_many_cb = [&](auto&) {};
    auto result = writer_.template add_many<decltype(writer_)::add_many_generator_callback_type::with_hash_value, decltype(timeseries_)>(state, stale_ts, add_many_cb);
    write_stats(stats);
    Writer::DestroySourceState(result);
  }

  // finalize - finalize the encoded data in the C++ encoder to Segment.
  template <class Stats, class Output>
  inline __attribute__((always_inline)) void finalize(Stats* stats, Output& out) {
    write_stats(stats);

    writer_.write(out);
  }

  inline __attribute__((always_inline)) ~GenericEncoder() = default;
};

class EncoderLightweight {
  uint16_t shard_id_;
  uint8_t log_shards_;
  BasicEncoder<Primitives::SnugComposites::LabelSet::EncodingTable, true> writer_;
  Primitives::TimeseriesSemiview timeseries_;

  template <class Stats>
  void write_stats(Stats* stats) const {
    size_t remaining_cap = std::numeric_limits<uint32_t>::max();

    stats->earliest_timestamp = writer_.buffer().earliest_sample();
    stats->latest_timestamp = writer_.buffer().latest_sample();
    stats->samples = writer_.buffer().samples_count();
    stats->series = writer_.buffer().series_count();
    stats->remainder_size = std::min(remaining_cap, writer_.remainder_size());

    if constexpr (have_allocated_memory<Stats>) {
      stats->allocated_memory = writer_.allocated_memory();
    }
  }

 public:
  PROMPP_ALWAYS_INLINE EncoderLightweight(uint16_t shard_id, uint8_t log_shards) noexcept
      : shard_id_(shard_id), log_shards_(log_shards), writer_(shard_id, log_shards) {}

  // add - add to encode incoming data(ShardedData) through C++ encoder.
  template <class Hashdex, class Stats>
  PROMPP_ALWAYS_INLINE void add(Hashdex& hx, Stats* stats) {
    for (const auto& item : hx) {
      if ((item.hash() % (1 << log_shards_)) == shard_id_) {
        item.read(timeseries_);
        writer_.add(timeseries_, item.hash());
        timeseries_.clear();
      }
    }

    write_stats(stats);
  }

  template <class Stats>
  PROMPP_ALWAYS_INLINE void add_inner_series(PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::InnerSeries*>& incoming_inner_series, Stats* stats) {
    std::ranges::for_each(incoming_inner_series, [&](const PromPP::Prometheus::Relabel::InnerSeries* inner_series) {
      if (inner_series == nullptr || inner_series->size() == 0) {
        return;
      }

      std::ranges::for_each(inner_series->data(), [&](const PromPP::Prometheus::Relabel::InnerSerie& inner_serie) {
        writer_.add_samples_on_ls_id(inner_serie.ls_id, inner_serie.samples);
      });
    });

    write_stats(stats);
  }

  template <class Stats>
  PROMPP_ALWAYS_INLINE void add_relabeled_series(PromPP::Prometheus::Relabel::RelabeledSeries* incoming_relabeled_series,
                                                 PromPP::Prometheus::Relabel::RelabelerStateUpdate* relabeler_state_update,
                                                 Stats* stats) {
    if (incoming_relabeled_series == nullptr || incoming_relabeled_series->size() == 0) {
      return;
    }

    std::ranges::for_each(incoming_relabeled_series->data(), [&](const PromPP::Prometheus::Relabel::RelabeledSerie& relabeled_serie) {
      if ((relabeled_serie.hash % (1 << log_shards_)) == shard_id_) {
        uint32_t ls_id = writer_.add_label_set(relabeled_serie.ls, relabeled_serie.hash);
        writer_.add_samples_on_ls_id(ls_id, relabeled_serie.samples);
        relabeler_state_update->emplace_back(relabeled_serie.ls_id, ls_id);
      }
    });

    write_stats(stats);
  }

  template <class Hashdex, class Stats>
  PROMPP_ALWAYS_INLINE Writer::SourceState add_with_stalenans(Hashdex& hx, Stats* stats, Primitives::Timestamp stale_ts, Writer::SourceState state) {
    auto add_many_cb = [&](auto& add_cb) {
      for (const auto& item : hx) {
        if ((item.hash() % (1 << log_shards_)) == shard_id_) {
          item.read(timeseries_);
          add_cb(timeseries_, item.hash());
          timeseries_.clear();
        }
      }
    };
    auto result = writer_.add_many<decltype(writer_)::add_many_generator_callback_type::with_hash_value, decltype(timeseries_)>(state, stale_ts, add_many_cb);

    write_stats(stats);
    return result;
  }

  template <class Stats>
  PROMPP_ALWAYS_INLINE void collect_source(Stats* stats, Primitives::Timestamp stale_ts, Writer::SourceState state) {
    constexpr auto add_many_cb = [&](auto&) {};
    auto result = writer_.add_many<decltype(writer_)::add_many_generator_callback_type::with_hash_value, decltype(timeseries_)>(state, stale_ts, add_many_cb);
    write_stats(stats);
    Writer::DestroySourceState(result);
  }

  // finalize - finalize the encoded data in the C++ encoder to Segment.
  template <class Stats, class Output>
  PROMPP_ALWAYS_INLINE void finalize(Stats* stats, Output& out) {
    write_stats(stats);

    writer_.write(out);
  }

  PROMPP_ALWAYS_INLINE ~EncoderLightweight() = default;
};

using Encoder = GenericEncoder<>;
}  // namespace PromPP::WAL
