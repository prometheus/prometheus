#pragma once

#include "bare_bones/gorilla.h"
#include "primitives/primitives.h"
#include "prometheus/tsdb/chunkenc/bstream.h"
#include "prometheus/tsdb/chunkenc/xor.h"
#include "series_data/decoder.h"
#include "series_data/encoder/bit_sequence.h"

namespace head {

template <class ChunkInfo>
concept ChunkInfoInterface = requires(ChunkInfo& info) {
  { info.interval } -> std::same_as<PromPP::Primitives::TimeInterval&>;
  { info.series_id } -> std::same_as<PromPP::Primitives::LabelSetID&>;
  { info.samples_count } -> std::same_as<uint8_t&>;
};

class ChunkRecoder {
 public:
  static constexpr auto kInvalidSeriesId = std::numeric_limits<uint32_t>::max();

  explicit ChunkRecoder(const series_data::DataStorage* data_storage, const PromPP::Primitives::TimeInterval& time_interval)
      : iterator_(data_storage), time_interval_{.min = time_interval.min, .max = time_interval.max - 1} {
    advance_to_non_empty_chunk();
  }

  void recode_next_chunk(ChunkInfoInterface auto& info) {
    reset_info(info);
    stream_.rewind();

    while (has_more_data()) {
      write_samples_count_placeholder();
      recode_chunk(info);

      ++iterator_;
      advance_to_non_empty_chunk();

      if (info.samples_count != 0) [[likely]] {
        write_samples_count(info.samples_count);
        break;
      }

      stream_.rewind();
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE std::span<const uint8_t> bytes() const noexcept { return stream_.bytes(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool has_more_data() const noexcept { return iterator_ != series_data::DataStorage::IteratorSentinel{}; }

 private:
  using Sample = series_data::encoder::Sample;
  using Decoder = series_data::Decoder;
  using Encoder =
      BareBones::Encoding::Gorilla::StreamEncoder<PromPP::Prometheus::tsdb::chunkenc::TimestampEncoder, PromPP::Prometheus::tsdb::chunkenc::ValuesEncoder>;

  series_data::DataStorage::ChunkIterator iterator_;
  PromPP::Prometheus::tsdb::chunkenc::BStream<series_data::encoder::kAllocationSizesTable> stream_;
  const PromPP::Primitives::TimeInterval time_interval_;

  PROMPP_ALWAYS_INLINE static void reset_info(ChunkInfoInterface auto& info) noexcept {
    info.interval.reset(0, 0);
    info.samples_count = 0;
    info.series_id = kInvalidSeriesId;
  }

  PROMPP_ALWAYS_INLINE void write_samples_count_placeholder() noexcept { stream_.write_bits(0, BareBones::Bit::to_bits(sizeof(uint16_t))); }
  PROMPP_ALWAYS_INLINE void write_samples_count(uint16_t samples_count) noexcept {
    *reinterpret_cast<uint16_t*>(stream_.raw_bytes()) = BareBones::Bit::be(samples_count);
  }

  void recode_chunk(ChunkInfoInterface auto& info) {
    Encoder encoder;
    Decoder::decode_chunk(*iterator_, [&](const Sample& sample) PROMPP_LAMBDA_INLINE {
      if (sample.timestamp > time_interval_.max) {
        return false;
      }
      if (sample.timestamp < time_interval_.min) {
        return true;
      }

      if (encoder.state().state == BareBones::Encoding::Gorilla::GorillaState::kFirstPoint) [[unlikely]] {
        info.interval.min = sample.timestamp;
      }
      encoder.encode(sample.timestamp, sample.value, stream_, stream_);
      ++info.samples_count;
      return true;
    });

    if (info.samples_count > 0) [[likely]] {
      info.interval.max = encoder.state().timestamp_encoder.last_ts;
      info.series_id = iterator_->series_id();
    }
  }

  void advance_to_non_empty_chunk() noexcept {
    const auto chunk_is_empty = [this] PROMPP_LAMBDA_INLINE {
      if (iterator_->chunk().is_empty()) {
        return true;
      }

      return !time_interval_.intersect({.min = Decoder::get_chunk_first_timestamp(*iterator_), .max = Decoder::get_chunk_last_timestamp(*iterator_)});
    };

    while (has_more_data() && chunk_is_empty()) {
      ++iterator_;
    }
  }
};

}  // namespace head