#pragma once

#include "series_data/encoder/gorilla.h"
#include "traits.h"

namespace series_data::decoder {

class GorillaDecodeIterator : public DecodeIteratorTrait {
 public:
  explicit GorillaDecodeIterator(const encoder::CompactBitSequence& stream)
      : GorillaDecodeIterator(encoder::BitSequenceWithItemsCount::count(stream), encoder::BitSequenceWithItemsCount::reader(stream)) {}
  GorillaDecodeIterator(uint8_t samples_count, const BareBones::BitSequenceReader& reader) : DecodeIteratorTrait(samples_count), reader_(reader) { decode(); }

  PROMPP_ALWAYS_INLINE GorillaDecodeIterator& operator++() noexcept {
    --remaining_samples_;
    decode();
    return *this;
  }

  PROMPP_ALWAYS_INLINE GorillaDecodeIterator operator++(int) noexcept {
    auto result = *this;
    ++*this;
    return result;
  }

 private:
  using Decoder = BareBones::Encoding::Gorilla::ValuesDecoder;

  BareBones::BitSequenceReader reader_;
  BareBones::Encoding::Gorilla::StreamDecoder<BareBones::Encoding::Gorilla::ZigZagTimestampDecoder<>, BareBones::Encoding::Gorilla::ValuesDecoder> decoder_;

  PROMPP_ALWAYS_INLINE void decode() noexcept {
    if (remaining_samples_ > 0) {
      decoder_.decode(reader_, reader_);
      sample_.value = decoder_.last_value();
      sample_.timestamp = decoder_.last_timestamp();
    }
  }
};

}  // namespace series_data::decoder
