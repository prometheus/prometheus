#pragma once

#include <cstdint>
#include <limits>

#include "bare_bones/concepts.h"
#include "bare_bones/preprocess.h"
#include "prometheus/relabeler.h"
#include "wal.h"

namespace PromPP::WAL {

template <typename Encoder = Writer>
class GenericEncoder {
  Encoder encoder_;
  Primitives::TimeseriesSemiview timeseries_;

  template <class Stats>
  void write_stats(Stats* stats) const {
    size_t remaining_cap = std::numeric_limits<uint32_t>::max();

    stats->earliest_timestamp = encoder_.buffer().earliest_sample();
    stats->latest_timestamp = encoder_.buffer().latest_sample();
    stats->samples = encoder_.buffer().samples_count();
    stats->series = encoder_.buffer().series_count();
    stats->remainder_size = std::min(remaining_cap, encoder_.remainder_size());

    if constexpr (BareBones::concepts::has_allocated_memory<Stats>) {
      stats->allocated_memory = encoder_.allocated_memory();
    }
  }

 public:
  template <class... Args>
  PROMPP_ALWAYS_INLINE explicit GenericEncoder(Args&&... args) noexcept : encoder_{std::forward<Args>(args)...} {}

  // add_wo_stalenans - add (without any stalenans) to encode incoming data(ShardedData) through C++ encoder.
  template <class Hashdex, class Stats>
  inline __attribute__((always_inline)) void add(Hashdex& hx, Stats* stats) {
    for (const auto& item : hx) {
      if ((item.hash() % (1 << encoder_.pow_two_of_total_shards())) == encoder_.shard_id()) {
        item.read(timeseries_);
        encoder_.add(timeseries_, item.hash());
        timeseries_.clear();
      }
    }

    write_stats(stats);
  }

  template <class Stats, class InnerSeriesSlice>
  inline __attribute__((always_inline)) void add_inner_series(InnerSeriesSlice& incoming_inner_series, Stats* stats) {
    std::ranges::for_each(incoming_inner_series, [&](const Prometheus::Relabel::InnerSeries* inner_series) {
      if (inner_series == nullptr || inner_series->size() == 0) {
        return;
      }

      std::ranges::for_each(inner_series->data(),
                            [&](const Prometheus::Relabel::InnerSerie& inner_serie) { encoder_.add_sample_on_ls_id(inner_serie.ls_id, inner_serie.sample); });
    });

    write_stats(stats);
  }

  template <class Stats>
  inline __attribute__((always_inline)) void add_relabeled_series(Prometheus::Relabel::RelabeledSeries* incoming_relabeled_series,
                                                                  Prometheus::Relabel::RelabelerStateUpdate* relabeler_state_update,
                                                                  Stats* stats) {
    if (incoming_relabeled_series == nullptr || incoming_relabeled_series->size() == 0) {
      return;
    }

    std::ranges::for_each(incoming_relabeled_series->data(), [&](const Prometheus::Relabel::RelabeledSerie& relabeled_serie) {
      if ((relabeled_serie.hash % (1 << encoder_.pow_two_of_total_shards())) == encoder_.shard_id()) {
        uint32_t ls_id = encoder_.add_label_set(relabeled_serie.ls, relabeled_serie.hash);
        for (const auto& sample : relabeled_serie.samples) {
          encoder_.add_sample_on_ls_id(ls_id, sample);
        }
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
        if ((item.hash() % (1 << encoder_.pow_two_of_total_shards())) == encoder_.shard_id()) {
          item.read(timeseries_);
          add_cb(timeseries_, item.hash());
          timeseries_.clear();
        }
      }
    };
    auto result =
        encoder_.template add_many<decltype(encoder_)::add_many_generator_callback_type::with_hash_value, decltype(timeseries_)>(state, stale_ts, add_many_cb);

    write_stats(stats);
    return result;
  }

  template <class Stats>
  inline __attribute__((always_inline)) void collect_source(Stats* stats, Primitives::Timestamp stale_ts, Writer::SourceState state) {
    constexpr auto add_many_cb = [&](auto&) {};
    auto result =
        encoder_.template add_many<decltype(encoder_)::add_many_generator_callback_type::with_hash_value, decltype(timeseries_)>(state, stale_ts, add_many_cb);
    write_stats(stats);
    Writer::DestroySourceState(result);
  }

  // finalize - finalize the encoded data in the C++ encoder to Segment.
  template <class Stats, class Output>
  inline __attribute__((always_inline)) void finalize(Stats* stats, Output& out) {
    write_stats(stats);

    encoder_.write(out);
  }
};

using Encoder = GenericEncoder<>;
using EncoderLightweight = GenericEncoder<BasicEncoder<Primitives::SnugComposites::LabelSet::ShrinkableEncodingBimap, true>>;

}  // namespace PromPP::WAL
