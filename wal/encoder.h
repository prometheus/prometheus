#pragma once

#include <cstdint>
#include <limits>

#include "bare_bones/vector.h"
#include "primitives/primitives.h"
#include "wal.h"

namespace PromPP::WAL {
class Encoder {
  uint16_t shard_id_;
  uint8_t log_shards_;
  Writer writer_;
  Primitives::TimeseriesSemiview timeseries_;

  template <class Stats>
  void write_stats(Stats* stats) const {
    size_t remaining_cap = std::numeric_limits<uint32_t>::max();

    stats->samples = writer_.buffer().samples_count();
    stats->series = writer_.buffer().series_count();
    stats->earliest_timestamp = writer_.buffer().earliest_sample();
    stats->latest_timestamp = writer_.buffer().latest_sample();
    stats->remainder_size = std::min(remaining_cap, writer_.remainder_size());
  }

 public:
  inline __attribute__((always_inline)) Encoder(uint16_t shard_id, uint8_t log_shards) noexcept
      : shard_id_(shard_id), log_shards_(log_shards), writer_(shard_id, log_shards) {}

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
    auto result = writer_.add_many<decltype(writer_)::add_many_generator_callback_type::with_hash_value, decltype(timeseries_)>(state, stale_ts, add_many_cb);

    write_stats(stats);
    return result;
  }

  template <class Stats>
  inline __attribute__((always_inline)) void collect_source(Stats* stats, Primitives::Timestamp stale_ts, Writer::SourceState state) {
    constexpr auto add_many_cb = [&](auto& add_cb) {};
    auto result = writer_.add_many<decltype(writer_)::add_many_generator_callback_type::with_hash_value, decltype(timeseries_)>(state, stale_ts, add_many_cb);
    write_stats(stats);
    Writer::DestroySourceState(result);
  }

  // finalize - finalize the encoded data in the C++ encoder to Segment.
  template <class Stats, class Output>
  inline __attribute__((always_inline)) void finalize(Stats* stats, Output& out) {
    write_stats(stats);

    writer_.write(out);
  }

  inline __attribute__((always_inline)) ~Encoder() = default;
};
}  // namespace PromPP::WAL
