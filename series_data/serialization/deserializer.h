#pragma once

#include <span>

#include "bare_bones/preprocess.h"
#include "series_data/chunk/serialized_chunk.h"
#include "series_data/decoder/asc_integer_values_gorilla.h"
#include "series_data/decoder/constant.h"
#include "series_data/decoder/gorilla.h"
#include "series_data/decoder/two_double_constant.h"
#include "series_data/decoder/values_gorilla.h"

namespace series_data::serialization {

class Deserializer {
 public:
  using SerializedChunkSpan = std::span<const chunk::SerializedChunk>;

  [[nodiscard]] PROMPP_ALWAYS_INLINE static bool is_valid_buffer(std::span<const uint8_t> buffer) noexcept {
    if (buffer.size() < sizeof(uint32_t)) {
      return false;
    }

    uint32_t chunks_count = *reinterpret_cast<const uint32_t*>(buffer.data());
    return buffer.size() >= sizeof(uint32_t) + chunks_count * sizeof(chunk::SerializedChunk);
  }

  explicit Deserializer(std::span<const uint8_t> buffer) : buffer_(buffer) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_valid() const noexcept { return is_valid_buffer(buffer_); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE SerializedChunkSpan get_chunks() const noexcept {
    uint32_t chunks_count = *reinterpret_cast<const uint32_t*>(buffer_.data());
    return {reinterpret_cast<const chunk::SerializedChunk*>(buffer_.data() + sizeof(uint32_t)), chunks_count};
  }

  template <chunk::DataChunk::EncodingType encoding_type>
  [[nodiscard]] PROMPP_ALWAYS_INLINE auto create_decode_iterator(const chunk::SerializedChunk& chunk) const {
    using enum chunk::DataChunk::EncodingType;

    if constexpr (encoding_type == kUint32Constant) {
      auto timestamp_buffer = buffer_.subspan(chunk.timestamps_offset);
      return decoder::ConstantDecodeIterator(encoder::BitSequenceWithItemsCount::count(timestamp_buffer.data()),
                                             encoder::BitSequenceWithItemsCount::reader(timestamp_buffer), chunk.values_offset);
    } else if constexpr (encoding_type == kDoubleConstant) {
      auto timestamp_buffer = buffer_.subspan(chunk.timestamps_offset);
      auto values_buffer = buffer_.subspan(chunk.values_offset);
      assert(values_buffer.size() >= sizeof(double));
      return decoder::ConstantDecodeIterator(encoder::BitSequenceWithItemsCount::count(timestamp_buffer.data()),
                                             encoder::BitSequenceWithItemsCount::reader(timestamp_buffer),
                                             *reinterpret_cast<const double*>(values_buffer.data()));
    } else if constexpr (encoding_type == kTwoDoubleConstant) {
      auto timestamp_buffer = buffer_.subspan(chunk.timestamps_offset);
      auto values_buffer = buffer_.subspan(chunk.values_offset);
      assert(values_buffer.size() >= sizeof(encoder::value::TwoDoubleConstantEncoder));
      return decoder::TwoDoubleConstantDecodeIterator(encoder::BitSequenceWithItemsCount::count(timestamp_buffer.data()),
                                                      encoder::BitSequenceWithItemsCount::reader(timestamp_buffer),
                                                      *reinterpret_cast<const encoder::value::TwoDoubleConstantEncoder*>(values_buffer.data()));
    } else if constexpr (encoding_type == kAscIntegerValuesGorilla) {
      auto timestamp_buffer = buffer_.subspan(chunk.timestamps_offset);
      auto values_buffer = buffer_.subspan(chunk.values_offset);
      return decoder::AscIntegerValuesGorillaDecodeIterator(encoder::BitSequenceWithItemsCount::count(timestamp_buffer.data()),
                                                            encoder::BitSequenceWithItemsCount::reader(timestamp_buffer),
                                                            BareBones::BitSequenceReader(values_buffer.data(), BareBones::Bit::to_bits(values_buffer.size())));
    } else if constexpr (encoding_type == kValuesGorilla) {
      auto timestamp_buffer = buffer_.subspan(chunk.timestamps_offset);
      auto values_buffer = buffer_.subspan(chunk.values_offset);
      return decoder::ValuesGorillaDecodeIterator(encoder::BitSequenceWithItemsCount::count(timestamp_buffer.data()),
                                                  encoder::BitSequenceWithItemsCount::reader(timestamp_buffer),
                                                  BareBones::BitSequenceReader(values_buffer.data(), BareBones::Bit::to_bits(values_buffer.size())));
    } else if constexpr (encoding_type == kGorilla) {
      auto values_buffer = buffer_.subspan(chunk.values_offset);
      return decoder::GorillaDecodeIterator(encoder::BitSequenceWithItemsCount::count(values_buffer.data()),
                                            encoder::BitSequenceWithItemsCount::reader(values_buffer));
    }
  }

 private:
  const std::span<const uint8_t> buffer_;
};

}  // namespace series_data::serialization