#pragma once

#include "series_data/encoder/value/two_double_constant.h"
#include "traits.h"

namespace series_data::decoder {

class TwoDoubleConstantDecodeIterator : public SeparatedTimestampValueDecodeIteratorTrait {
 public:
  TwoDoubleConstantDecodeIterator(const encoder::BitSequenceWithItemsCount& timestamp_stream, const encoder::value::TwoDoubleConstantEncoder& encoder)
      : SeparatedTimestampValueDecodeIteratorTrait(timestamp_stream, encoder.value1()), encoder_(&encoder) {}

  PROMPP_ALWAYS_INLINE TwoDoubleConstantDecodeIterator& operator++() noexcept {
    if (decode_timestamp()) {
      ++count_;
      sample_.value = count_ <= encoder_->value1_count() ? encoder_->value1() : encoder_->value2();
    }
    return *this;
  }

  PROMPP_ALWAYS_INLINE TwoDoubleConstantDecodeIterator operator++(int) noexcept {
    auto result = *this;
    ++*this;
    return result;
  }

 private:
  const encoder::value::TwoDoubleConstantEncoder* encoder_;
  uint8_t count_{1};
};

}  // namespace series_data::decoder
