#pragma once

#include "bare_bones/gorilla.h"
#include "primitives/primitives.h"
#include "prometheus/tsdb/chunkenc/bstream.h"
#include "prometheus/tsdb/chunkenc/xor.h"
#include "series_data/decoder.h"
#include "series_data/encoder/bit_sequence.h"

namespace entrypoint {

template <class ChunkInfo>
concept ChunkInfoInterface = requires(ChunkInfo& info) {
  { info.min_t } -> std::same_as<PromPP::Primitives::Timestamp&>;
  { info.max_t } -> std::same_as<PromPP::Primitives::Timestamp&>;
  { info.samples_count } -> std::same_as<uint8_t&>;
};

class ChunkRecoder {
 public:
  static constexpr auto kInvalidSeriesId = std::numeric_limits<uint32_t>::max();

  explicit ChunkRecoder(const series_data::DataStorage* data_storage) : iterator_(data_storage) {}

  void recode_next_chunk(ChunkInfoInterface auto& info) {
    stream_.rewind();

    if (has_more_data()) {
      write_samples_count(info);
      recode_chunk(info);
      ++iterator_;
    } else {
      info.min_t = 0;
      info.max_t = 0;
      info.samples_count = 0;
    }
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

  PROMPP_ALWAYS_INLINE void write_samples_count(ChunkInfoInterface auto& info) {
    info.samples_count = Decoder::get_samples_count(*iterator_->storage(), iterator_->chunk(), iterator_->chunk_type());
    uint16_t samples_count = info.samples_count;
    stream_.write_bits(samples_count, BareBones::Bit::to_bits(sizeof(samples_count)));
  }

  void recode_chunk(ChunkInfoInterface auto& info) {
    Encoder encoder;
    Decoder::decode_chunk(*iterator_->storage(), iterator_->chunk(), iterator_->chunk_type(), [&](const Sample& sample) PROMPP_LAMBDA_INLINE {
      if (encoder.state().state == BareBones::Encoding::Gorilla::GorillaState::kFirstPoint) {
        [[unlikely]];
        info.min_t = sample.timestamp;
      }
      encoder.encode(sample.timestamp, sample.value, stream_, stream_);
    });
    info.max_t = encoder.state().timestamp_encoder.last_ts;
  }
};

using ChunkRecoderPtr = std::unique_ptr<ChunkRecoder>;

}  // namespace entrypoint