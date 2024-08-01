#pragma once

#include "series_data/encoder/value/values_gorilla.h"
#include "traits.h"

namespace series_data::decoder {

class ValuesGorillaDecodeIterator : public SeparatedTimestampValueDecodeIteratorTrait {
 public:
  ValuesGorillaDecodeIterator(const encoder::BitSequenceWithItemsCount& timestamp_stream, const BareBones::BitSequenceReader& reader)
      : ValuesGorillaDecodeIterator(timestamp_stream.count(), timestamp_stream.reader(), reader) {}
  ValuesGorillaDecodeIterator(uint8_t samples_count, const BareBones::BitSequenceReader& timestamp_reader, const BareBones::BitSequenceReader& values_reader)
      : SeparatedTimestampValueDecodeIteratorTrait(samples_count, timestamp_reader, 0.0), reader_(values_reader) {
    if (remaining_samples_ > 0) {
      decode_value<true>();
    }
  }

  PROMPP_ALWAYS_INLINE ValuesGorillaDecodeIterator& operator++() noexcept {
    if (decode_timestamp()) {
      decode_value<false>();
    }
    return *this;
  }

  PROMPP_ALWAYS_INLINE ValuesGorillaDecodeIterator operator++(int) noexcept {
    auto result = *this;
    ++*this;
    return result;
  }

 private:
  using Decoder = BareBones::Encoding::Gorilla::ValuesDecoder;

  BareBones::BitSequenceReader reader_;
  BareBones::Encoding::Gorilla::ValuesDecoderState state_;

  template <bool first>
  void decode_value() noexcept {
    if constexpr (first) {
      Decoder::decode_first(state_, reader_);
    } else {
      Decoder::decode(state_, reader_);
    }

    sample_.value = state_.last_v;
  }
};

}  // namespace series_data::decoder
