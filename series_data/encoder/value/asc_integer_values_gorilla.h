#pragma once

#include "bare_bones/gorilla.h"
#include "series_data/encoder/bit_sequence.h"
#include "series_data/encoder/numeric.h"

namespace series_data::encoder::value {

static constexpr BareBones::Encoding::Gorilla::DodSignificantLengths kDodSignificantLengths = {.first = 4, .second = 12, .third = 21};

class PROMPP_ATTRIBUTE_PACKED AscIntegerValuesGorillaEncoder {
 public:
  PROMPP_ALWAYS_INLINE explicit AscIntegerValuesGorillaEncoder(double value) { Encoder::encode(state_, static_cast<int64_t>(value), stream_); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static bool can_be_encoded(double value1, uint8_t value1_count, double value2, double value3) {
    if (!is_valid_int(value1)) {
      return false;
    }

    if (value1_count > 1) {
      if (BareBones::Encoding::Gorilla::isstalenan(value2)) {
        [[unlikely]];
        return is_valid_int_and_ge_than(value3, value1);
      }
    }

    return is_valid_int_and_ge_than(value2, value1) && (is_valid_int_and_ge_than(value3, value2) || BareBones::Encoding::Gorilla::isstalenan(value3));
  }

  PROMPP_ALWAYS_INLINE void encode_second(double value) {
    Encoder::encode_delta(state_, static_cast<int64_t>(value), stream_);
    last_value_type_ = BareBones::Encoding::Gorilla::get_value_type(value);
  }

  PROMPP_ALWAYS_INLINE bool encode(double value) noexcept {
    if (BareBones::Encoding::Gorilla::isstalenan(value)) {
      [[unlikely]];
      last_value_type_ = ValueType::kStaleNan;
    } else {
      if (!is_valid_int_and_ge_than(value, static_cast<double>(state_.last_ts))) {
        [[unlikely]];
        return false;
      }

      last_value_type_ = ValueType::kValue;
    }

    Encoder::encode_delta_of_delta_with_stale_nan(state_, value, stream_);
    return true;
  }

  PROMPP_ALWAYS_INLINE bool operator==(const AscIntegerValuesGorillaEncoder& other) const noexcept { return stream_ == other.stream_; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return stream_.allocated_memory(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_actual(double value) const noexcept {
    return last_value_type_ == ValueType::kStaleNan ? BareBones::Encoding::Gorilla::isstalenan(value) : is_values_strictly_equals(value, last_value());
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE double last_value() const noexcept { return static_cast<double>(state_.last_ts); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const CompactBitSequence& stream() const noexcept { return stream_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE CompactBitSequence finalize_stream() noexcept {
    auto stream = std::move(stream_);
    stream.shrink_to_fit();
    return stream;
  }

 public:
  using Encoder = BareBones::Encoding::Gorilla::ZigZagTimestampEncoder<kDodSignificantLengths>;
  using ValueType = BareBones::Encoding::Gorilla::ValueType;

  BareBones::Encoding::Gorilla::TimestampEncoderState state_;
  CompactBitSequence stream_;
  ValueType last_value_type_{ValueType::kValue};

  PROMPP_ALWAYS_INLINE static bool is_valid_int(double value) noexcept {
    return is_int(value) && is_in_bounds(value, std::numeric_limits<int32_t>::min(), std::numeric_limits<int32_t>::max());
  }

  PROMPP_ALWAYS_INLINE static bool is_valid_int_and_ge_than(double value2, double value1) noexcept {
    return is_int(value2) && is_in_bounds(value2, value1, std::numeric_limits<int32_t>::max());
  }
};

}  // namespace series_data::encoder::value

template <>
struct BareBones::IsTriviallyReallocatable<series_data::encoder::value::AscIntegerValuesGorillaEncoder> : std::true_type {};
