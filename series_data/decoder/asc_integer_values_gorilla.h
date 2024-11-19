#pragma once

#include "series_data/encoder/value/asc_integer_values_gorilla.h"
#include "traits.h"

namespace series_data::decoder {

class AscIntegerValuesGorillaDecodeIterator : public SeparatedTimestampValueDecodeIteratorTrait {
 public:
  AscIntegerValuesGorillaDecodeIterator(const encoder::BitSequenceWithItemsCount& timestamp_stream, const BareBones::BitSequenceReader& reader)
      : AscIntegerValuesGorillaDecodeIterator(timestamp_stream.count(), timestamp_stream.reader(), reader) {}
  AscIntegerValuesGorillaDecodeIterator(uint8_t samples_count,
                                        const BareBones::BitSequenceReader& timestamp_reader,
                                        const BareBones::BitSequenceReader& values_reader)
      : SeparatedTimestampValueDecodeIteratorTrait(samples_count, timestamp_reader, 0.0), reader_(values_reader) {
    if (remaining_samples_ > 0) {
      decode_value();
    }
  }

  PROMPP_ALWAYS_INLINE AscIntegerValuesGorillaDecodeIterator& operator++() noexcept {
    if (decode_timestamp()) {
      decode_value();
    }
    return *this;
  }

  PROMPP_ALWAYS_INLINE AscIntegerValuesGorillaDecodeIterator operator++(int) noexcept {
    const auto result = *this;
    ++*this;
    return result;
  }

 private:
  using GorillaState = BareBones::Encoding::Gorilla::GorillaState;
  using Decoder = BareBones::Encoding::Gorilla::ZigZagTimestampDecoder<encoder::value::kAscIntegerDodSignificantLengths>;
  using ValueType = BareBones::Encoding::Gorilla::ValueType;

  Decoder decoder_;
  BareBones::BitSequenceReader reader_;
  BareBones::Encoding::Gorilla::GorillaState gorilla_state_{GorillaState::kFirstPoint};

  void decode_value() noexcept {
    if (gorilla_state_ == GorillaState::kFirstPoint) [[unlikely]] {
      decoder_.decode(reader_);
      gorilla_state_ = GorillaState::kSecondPoint;
    } else if (gorilla_state_ == GorillaState::kSecondPoint) [[unlikely]] {
      decoder_.decode_delta(reader_);
      gorilla_state_ = GorillaState::kOtherPoint;
    } else {
      if (const auto type = decoder_.decode_delta_of_delta_with_stale_nan(reader_); type == ValueType::kStaleNan) [[unlikely]] {
        sample_.value = BareBones::Encoding::Gorilla::STALE_NAN;
        return;
      }
    }

    sample_.value = static_cast<double>(decoder_.timestamp());
  }
};

}  // namespace series_data::decoder
