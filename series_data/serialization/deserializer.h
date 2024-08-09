#pragma once

#include <span>

#include "bare_bones/preprocess.h"
#include "series_data/chunk/serialized_chunk.h"
#include "series_data/decoder/universal_decode_iterator.h"

namespace series_data::serialization {

class Deserializer {
 public:
  using SerializedChunkSpan = std::span<const chunk::SerializedChunk>;

  class IteratorSentinel {};

  class Iterator {
   public:
    class Data {
     public:
      Data(std::span<const uint8_t> buffer, SerializedChunkSpan chunks) : buffer_(buffer), chunk_iterator_(chunks.begin()), chunk_end_iterator_(chunks.end()) {}

      [[nodiscard]] PROMPP_ALWAYS_INLINE PromPP::Primitives::LabelSetID label_set_id() const noexcept { return chunk_iterator_->label_set_id; }
      [[nodiscard]] PROMPP_ALWAYS_INLINE chunk::DataChunk::EncodingType encoding_type() const noexcept { return chunk_iterator_->encoding_type; }
      [[nodiscard]] PROMPP_ALWAYS_INLINE decoder::UniversalDecodeIterator decode_iterator() const { return create_decode_iterator(buffer_, *chunk_iterator_); }

     private:
      friend class Iterator;

      const std::span<const uint8_t> buffer_;
      SerializedChunkSpan::iterator chunk_iterator_;
      SerializedChunkSpan::iterator chunk_end_iterator_;

      PROMPP_ALWAYS_INLINE void next_value() noexcept { ++chunk_iterator_; }

      [[nodiscard]] PROMPP_ALWAYS_INLINE bool has_value() const noexcept { return chunk_iterator_ != chunk_end_iterator_; }
    };

    using iterator_category = std::forward_iterator_tag;
    using value_type = Data;
    using difference_type = ptrdiff_t;
    using pointer = value_type*;
    using reference = value_type&;

    explicit Iterator(std::span<const uint8_t> buffer) : data_(buffer, get_chunks(buffer)) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE const Data& operator*() const noexcept { return data_; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE const Data* operator->() const noexcept { return &data_; }

    PROMPP_ALWAYS_INLINE Iterator& operator++() noexcept {
      data_.next_value();
      return *this;
    }

    PROMPP_ALWAYS_INLINE Iterator operator++(int) noexcept {
      auto it = *this;
      ++*this;
      return it;
    }

    PROMPP_ALWAYS_INLINE bool operator==(const IteratorSentinel&) const noexcept { return !data_.has_value(); }

   private:
    Data data_;
  };

  [[nodiscard]] PROMPP_ALWAYS_INLINE static bool is_valid_buffer(std::span<const uint8_t> buffer) noexcept {
    if (buffer.size() < sizeof(uint32_t)) {
      return false;
    }

    uint32_t chunks_count = *reinterpret_cast<const uint32_t*>(buffer.data());
    return buffer.size() >= sizeof(uint32_t) + chunks_count * sizeof(chunk::SerializedChunk);
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE static SerializedChunkSpan get_chunks(std::span<const uint8_t> buffer) noexcept {
    uint32_t chunks_count = *reinterpret_cast<const uint32_t*>(buffer.data());
    return {reinterpret_cast<const chunk::SerializedChunk*>(buffer.data() + sizeof(uint32_t)), chunks_count};
  }
  [[nodiscard]] static decoder::UniversalDecodeIterator create_decode_iterator(std::span<const uint8_t> buffer, const chunk::SerializedChunk& chunk) {
    using enum chunk::DataChunk::EncodingType;

    switch (chunk.encoding_type) {
      case kUint32Constant: {
        auto timestamp_buffer = buffer.subspan(chunk.timestamps_offset);
        return decoder::UniversalDecodeIterator(
            std::in_place_type<decoder::ConstantDecodeIterator>,
            decoder::ConstantDecodeIterator(encoder::BitSequenceWithItemsCount::count(timestamp_buffer.data()),
                                            encoder::BitSequenceWithItemsCount::reader(timestamp_buffer), chunk.values_offset));
      }

      case kDoubleConstant: {
        auto timestamp_buffer = buffer.subspan(chunk.timestamps_offset);
        auto values_buffer = buffer.subspan(chunk.values_offset);
        assert(values_buffer.size() >= sizeof(double));
        return decoder::UniversalDecodeIterator(
            std::in_place_type<decoder::ConstantDecodeIterator>, encoder::BitSequenceWithItemsCount::count(timestamp_buffer.data()),
            encoder::BitSequenceWithItemsCount::reader(timestamp_buffer), *reinterpret_cast<const double*>(values_buffer.data()));
      }

      case kTwoDoubleConstant: {
        auto timestamp_buffer = buffer.subspan(chunk.timestamps_offset);
        auto values_buffer = buffer.subspan(chunk.values_offset);
        assert(values_buffer.size() >= sizeof(encoder::value::TwoDoubleConstantEncoder));
        return decoder::UniversalDecodeIterator(std::in_place_type<decoder::TwoDoubleConstantDecodeIterator>,
                                                encoder::BitSequenceWithItemsCount::count(timestamp_buffer.data()),
                                                encoder::BitSequenceWithItemsCount::reader(timestamp_buffer),
                                                *reinterpret_cast<const encoder::value::TwoDoubleConstantEncoder*>(values_buffer.data()));
      }

      case kAscIntegerValuesGorilla: {
        auto timestamp_buffer = buffer.subspan(chunk.timestamps_offset);
        auto values_buffer = buffer.subspan(chunk.values_offset);
        return decoder::UniversalDecodeIterator(std::in_place_type<decoder::AscIntegerValuesGorillaDecodeIterator>,
                                                encoder::BitSequenceWithItemsCount::count(timestamp_buffer.data()),
                                                encoder::BitSequenceWithItemsCount::reader(timestamp_buffer),
                                                BareBones::BitSequenceReader(values_buffer.data(), BareBones::Bit::to_bits(values_buffer.size())));
      }

      case kValuesGorilla: {
        auto timestamp_buffer = buffer.subspan(chunk.timestamps_offset);
        auto values_buffer = buffer.subspan(chunk.values_offset);
        return decoder::UniversalDecodeIterator(std::in_place_type<decoder::ValuesGorillaDecodeIterator>,
                                                encoder::BitSequenceWithItemsCount::count(timestamp_buffer.data()),
                                                encoder::BitSequenceWithItemsCount::reader(timestamp_buffer),
                                                BareBones::BitSequenceReader(values_buffer.data(), BareBones::Bit::to_bits(values_buffer.size())));
      }

      case kGorilla: {
        auto values_buffer = buffer.subspan(chunk.values_offset);
        return decoder::UniversalDecodeIterator(std::in_place_type<decoder::GorillaDecodeIterator>,
                                                encoder::BitSequenceWithItemsCount::count(values_buffer.data()),
                                                encoder::BitSequenceWithItemsCount::reader(values_buffer));
      }

      default: {
        throw BareBones::Exception(0xb4474b73b71c449f, "Unsupported data chunk encoding type %d", static_cast<int>(chunk.encoding_type));
      }
    }
  }
  [[nodiscard]] static Iterator chunk_iterator(std::span<const uint8_t> buffer) noexcept { return Iterator(buffer); }

  explicit Deserializer(std::span<const uint8_t> buffer) : buffer_(buffer) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_valid() const noexcept { return is_valid_buffer(buffer_); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE SerializedChunkSpan get_chunks() const noexcept { return get_chunks(buffer_); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE decoder::UniversalDecodeIterator create_decode_iterator(const chunk::SerializedChunk& chunk) const {
    return create_decode_iterator(buffer_, chunk);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE Iterator begin() const noexcept { return Iterator(buffer_); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE static IteratorSentinel end() noexcept { return {}; }

 private:
  const std::span<const uint8_t> buffer_;
};

}  // namespace series_data::serialization