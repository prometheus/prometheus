#pragma once

#include "traits.h"

namespace series_data::decoder {

class ConstantDecodeIterator : public SeparatedTimestampValueDecodeIteratorTrait {
 public:
  ConstantDecodeIterator(const encoder::BitSequenceWithItemsCount& timestamp_stream, double value)
      : SeparatedTimestampValueDecodeIteratorTrait(timestamp_stream, value) {}
  ConstantDecodeIterator(uint8_t samples_count, const BareBones::BitSequenceReader& timestamp_reader, double value)
      : SeparatedTimestampValueDecodeIteratorTrait(samples_count, timestamp_reader, value) {}

  PROMPP_ALWAYS_INLINE ConstantDecodeIterator& operator++() noexcept {
    decode_timestamp();
    return *this;
  }

  PROMPP_ALWAYS_INLINE ConstantDecodeIterator operator++(int) noexcept {
    auto result = *this;
    ++*this;
    return result;
  }
};

}  // namespace series_data::decoder
