#pragma once

#include "bare_bones/gorilla.h"
#include "primitives/primitives.h"
#include "prometheus/tsdb/chunkenc/bstream.h"
#include "prometheus/tsdb/chunkenc/xor.h"
#include "series_data/decoder.h"
#include "series_data/encoder/bit_sequence.h"

namespace entrypoint {

class ChunkRecoder {
 public:
  struct ChunkInfo {
    PromPP::Primitives::Timestamp min_t{};
    PromPP::Primitives::Timestamp max_t{};
  };

  static constexpr auto kInvalidSeriesId = std::numeric_limits<uint32_t>::max();

  explicit ChunkRecoder(const series_data::DataStorage* data_storage) : iterator_(data_storage) {}

  [[nodiscard]] ChunkInfo recode_next_chunk() {
    stream_.rewind();

    ChunkInfo info;
    if (has_more_data()) {
      write_samples_count();

      Encoder encoder;
      Decoder::decode_chunk(*iterator_->storage(), iterator_->chunk(), iterator_->chunk_type(), [&](const Sample& sample) PROMPP_LAMBDA_INLINE {
        if (encoder.state().state == BareBones::Encoding::Gorilla::GorillaState::kFirstPoint) {
          [[unlikely]];
          info.min_t = sample.timestamp;
        }
        encoder.encode(sample.timestamp, sample.value, stream_, stream_);
      });
      info.max_t = encoder.state().timestamp_encoder.last_ts;

      ++iterator_;
    }

    return info;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE std::span<const uint8_t> bytes() const noexcept { return stream_.bytes(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t series_id() const noexcept { return has_more_data() ? iterator_->series_id() : kInvalidSeriesId; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool has_more_data() const noexcept { return iterator_ != series_data::DataStorage::IteratorSentinel{}; }

 private:
  using Sample = series_data::encoder::Sample;
  using Decoder = series_data::Decoder;
  using Encoder =
      BareBones::Encoding::Gorilla::StreamEncoder<PromPP::Prometheus::tsdb::chunkenc::TimestampEncoder, PromPP::Prometheus::tsdb::chunkenc::ValuesEncoder>;

  series_data::DataStorage::ChunkIterator iterator_;
  PromPP::Prometheus::tsdb::chunkenc::BStream<series_data::encoder::kAllocationSizesTable> stream_;

  PROMPP_ALWAYS_INLINE void write_samples_count() {
    uint16_t samples_count = Decoder::get_samples_count(*iterator_->storage(), iterator_->chunk(), iterator_->chunk_type());
    stream_.write_bits(samples_count, BareBones::Bit::to_bits(sizeof(samples_count)));
  }
};

using ChunkRecoderPtr = std::unique_ptr<ChunkRecoder>;

}  // namespace entrypoint