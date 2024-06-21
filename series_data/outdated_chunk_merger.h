#pragma once

#include "concepts.h"
#include "data_storage.h"
#include "decoder.h"
#include "encoder/sample.h"

namespace series_data {

template <EncoderInterface Encoder>
class OutdatedChunkMerger {
 public:
  OutdatedChunkMerger(DataStorage& storage, Encoder& encoder) : storage_(storage), encoder_(encoder) {}

  void merge() {
    for (auto& [ls_id, chunk] : storage_.outdated_chunks) {
      merge(ls_id, chunk);
    }

    storage_.outdated_chunks.clear();
  }

  void merge(uint32_t ls_id, const chunk::OutdatedChunk& chunk) {
    auto decoded_samples = decode_samples(chunk);

    SamplesSpan samples{decoded_samples.begin(), decoded_samples.end()};
    auto& finalized_chunks = storage_.finalized_chunks;
    if (auto finalized_chunks_it = finalized_chunks.find(ls_id); finalized_chunks_it != finalized_chunks.end()) {
      merge_outdated_samples_in_finalized_chunks(ls_id, finalized_chunks_it->second, samples);
    }

    if (!samples.empty()) {
      merge_outdated_samples<ChunkType::kOpen>(ls_id, storage_.open_chunks[ls_id], std::numeric_limits<int64_t>::max(), samples);
    }
  }

 private:
  using SampleList = BareBones::Vector<encoder::Sample>;
  using ChunkType = chunk::DataChunk::Type;
  using ChunkEncodingType = chunk::DataChunk::EncodingType;
  using SamplesSpan = std::span<encoder::Sample>;

  class IteratorSentinel {};

  class EncodeIterator {
   public:
    using difference_type = ptrdiff_t;

    EncodeIterator(Encoder& encoder, chunk::DataChunk& chunk, uint32_t ls_id) : encoder_(&encoder), chunk_(&chunk), ls_id_(ls_id) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE EncodeIterator& operator*() noexcept { return *this; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE EncodeIterator& operator=(const encoder::Sample& sample) noexcept {
      encoder_->encode(ls_id_, sample.timestamp, sample.value, *chunk_);
      return *this;
    }
    [[nodiscard]] PROMPP_ALWAYS_INLINE EncodeIterator& operator++() noexcept { return *this; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE EncodeIterator operator++(int) noexcept { return *this; }

    [[nodiscard]] PROMPP_ALWAYS_INLINE bool operator==(const IteratorSentinel&) const noexcept { return false; }

   private:
    Encoder* encoder_;
    chunk::DataChunk* chunk_;
    uint32_t ls_id_;
  };

  class SampleMergeIterator {
   public:
    using iterator_category = std::forward_iterator_tag;
    using value_type = encoder::Sample;
    using difference_type = ptrdiff_t;
    using pointer = encoder::Sample*;
    using reference = encoder::Sample&;

    explicit SampleMergeIterator(SamplesSpan::iterator& begin, SamplesSpan::iterator end, int64_t max_timestamp)
        : iterator_(&begin), end_(end), max_timestamp_(max_timestamp) {
      skip_repeatable_timestamps();
    }

    const encoder::Sample& operator*() const noexcept { return **iterator_; }

    PROMPP_ALWAYS_INLINE SampleMergeIterator& operator++() noexcept {
      ++(*iterator_);
      skip_repeatable_timestamps();
      return *this;
    }

    PROMPP_ALWAYS_INLINE SampleMergeIterator operator++(int) noexcept {
      auto result = *this;
      this->operator++();
      return result;
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE bool operator==(const IteratorSentinel&) const noexcept {
      return *iterator_ == end_ || (*iterator_)->timestamp >= max_timestamp_;
    }

   private:
    SamplesSpan::iterator* iterator_;
    SamplesSpan::iterator end_;
    int64_t max_timestamp_{};

    void skip_repeatable_timestamps() {
      for (; *iterator_ != end_; ++*iterator_) {
        auto next = std::next(*iterator_);
        if (next == end_ || (*iterator_)->timestamp != next->timestamp) {
          break;
        }
      }
    }
  };

  DataStorage& storage_;
  Encoder& encoder_;

  [[nodiscard]] static SampleList decode_samples(const chunk::OutdatedChunk& chunk) {
    SampleList samples = Decoder::decode_gorilla_chunk(chunk.stream().stream);
    std::sort(samples.begin(), samples.end());
    return samples;
  }

  void merge_outdated_samples_in_finalized_chunks(uint32_t ls_id, const chunk::FinalizedChunkList& finalized_chunks, SamplesSpan& samples) {
    for (auto it = finalized_chunks.begin(), next_it = std::next(it); it != finalized_chunks.end(); ++next_it) {
      if (next_it == finalized_chunks.end()) {
        if (auto open_chunk_timestamp = Decoder::get_chunk_first_timestamp<ChunkType::kOpen>(storage_, storage_.open_chunks[ls_id]);
            open_chunk_timestamp > samples.front().timestamp) {
          merge_outdated_samples<ChunkType::kFinalized>(ls_id, *it, open_chunk_timestamp, samples);
        }

        return;
      } else {
        if (auto next_chunk_timestamp = Decoder::get_chunk_first_timestamp<ChunkType::kFinalized>(storage_, *next_it);
            next_chunk_timestamp > samples.front().timestamp) {
          merge_outdated_samples<ChunkType::kFinalized>(ls_id, *it, next_chunk_timestamp, samples);
          it = next_it;
        } else {
          ++it;
        }
      }
    }
  }

  template <ChunkType chunk_type>
  void merge_outdated_samples(uint32_t ls_id, const chunk::DataChunk& source_chunk, int64_t max_timestamp, SamplesSpan& samples) {
    auto chunk = merge_outdated_samples_in_new_chunk<chunk_type>(ls_id, source_chunk, max_timestamp, samples);

    if constexpr (chunk_type == ChunkType::kFinalized) {
      ChunkFinalizer::finalize(storage_, ls_id, chunk);
      erase_finalized_chunk(ls_id, source_chunk);
    } else {
      replace_open_chunk(ls_id, chunk);
    }
  }

  template <ChunkType chunk_type>
  chunk::DataChunk merge_outdated_samples_in_new_chunk(uint32_t ls_id, const chunk::DataChunk& source_chunk, int64_t max_timestamp, SamplesSpan& samples) {
    using enum chunk::DataChunk::EncodingType;

    chunk::DataChunk chunk;
    switch (source_chunk.encoding_type) {
      case kUnknown: {
        assert(source_chunk.encoding_type != kUnknown);
        break;
      }

      case kUint32Constant: {
        merge_outdated_samples<kUint32Constant, chunk_type>(source_chunk, max_timestamp, EncodeIterator{encoder_, chunk, ls_id}, samples);
        break;
      }

      case kDoubleConstant: {
        merge_outdated_samples<kDoubleConstant, chunk_type>(source_chunk, max_timestamp, EncodeIterator{encoder_, chunk, ls_id}, samples);
        break;
      }

      case kTwoDoubleConstant: {
        merge_outdated_samples<kTwoDoubleConstant, chunk_type>(source_chunk, max_timestamp, EncodeIterator{encoder_, chunk, ls_id}, samples);
        break;
      }

      case kAscIntegerValuesGorilla: {
        merge_outdated_samples<kAscIntegerValuesGorilla, chunk_type>(source_chunk, max_timestamp, EncodeIterator{encoder_, chunk, ls_id}, samples);
        break;
      }

      case kValuesGorilla: {
        merge_outdated_samples<kValuesGorilla, chunk_type>(source_chunk, max_timestamp, EncodeIterator{encoder_, chunk, ls_id}, samples);
        break;
      }

      case kGorilla: {
        merge_outdated_samples<kGorilla, chunk_type>(source_chunk, max_timestamp, EncodeIterator{encoder_, chunk, ls_id}, samples);
        break;
      }
    }

    return chunk;
  }

  template <ChunkEncodingType encoding_type, ChunkType chunk_type>
  void merge_outdated_samples(const chunk::DataChunk& source_chunk, int64_t max_timestamp, const EncodeIterator& encode_iterator, SamplesSpan& samples) {
    SamplesSpan::iterator begin = samples.begin();
    std::ranges::set_union(SampleMergeIterator{begin, samples.end(), max_timestamp}, IteratorSentinel{},
                           Decoder::create_decode_iterator<encoding_type, chunk_type>(storage_, source_chunk), decoder::DecodeIteratorSentinel{},
                           encode_iterator);
    samples = {begin, samples.end()};
  }

  void erase_finalized_chunk(uint32_t ls_id, const chunk::DataChunk& chunk) {
    if (auto finalized_it = storage_.finalized_chunks.find(ls_id); finalized_it != storage_.finalized_chunks.end()) {
      storage_.erase_chunk<chunk::DataChunk::Type::kFinalized>(chunk);
      finalized_it->second.erase(chunk);
    }
  }

  void replace_open_chunk(uint32_t ls_id, const chunk::DataChunk& chunk) {
    auto& open_chunk = storage_.open_chunks[ls_id];
    storage_.erase_chunk<chunk::DataChunk::Type::kOpen>(open_chunk);
    open_chunk = chunk;
  }
};

}  // namespace series_data