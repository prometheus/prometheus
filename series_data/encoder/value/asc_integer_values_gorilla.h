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
    static constexpr auto is_int_and_ge_than = [](double value2, double value1) PROMPP_LAMBDA_INLINE { return is_int(value2) && value2 >= value1; };

    if (!is_int(value1)) {
      return false;
    }

    if (value1_count > 1) {
      if (BareBones::Encoding::Gorilla::isstalenan(value2)) {
        [[unlikely]];
        return is_int_and_ge_than(value3, value1);
      }
    }

    return is_int_and_ge_than(value2, value1) && (is_int_and_ge_than(value3, value2) || BareBones::Encoding::Gorilla::isstalenan(value3));
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
      if (!is_int(value) || static_cast<int64_t>(value) < state_.last_ts) {
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

 private:
  using Encoder = BareBones::Encoding::Gorilla::ZigZagTimestampEncoder<kDodSignificantLengths>;
  using ValueType = BareBones::Encoding::Gorilla::ValueType;

  BareBones::Encoding::Gorilla::TimestampEncoderState state_;
  CompactBitSequence stream_;
  ValueType last_value_type_{ValueType::kValue};
};

class AscIntegerValuesGorillaDecoder {
 public:
  explicit AscIntegerValuesGorillaDecoder(BareBones::BitSequenceReader reader) : reader_(reader) {}

  PROMPP_ALWAYS_INLINE double decode() noexcept {
    if (gorilla_state_ == GorillaState::kFirstPoint) {
      [[unlikely]];

      Decoder::decode(state_, reader_);
      gorilla_state_ = GorillaState::kSecondPoint;
    } else if (gorilla_state_ == GorillaState::kSecondPoint) {
      [[unlikely]];

      Decoder::decode_delta(state_, reader_);
      gorilla_state_ = GorillaState::kOtherPoint;
    } else {
      if (auto type = Decoder::decode_delta_of_delta_with_stale_nan(state_, reader_); type == ValueType::kStaleNan) {
        [[unlikely]];
        return BareBones::Encoding::Gorilla::STALE_NAN;
      }
    }

    return static_cast<double>(state_.last_ts);
  }

  [[nodiscard]] static BareBones::Vector<double> decode_all(BareBones::BitSequenceReader reader) noexcept {
    BareBones::Vector<double> values;

    AscIntegerValuesGorillaDecoder decoder(reader);
    while (!decoder.reader_.eof()) {
      values.emplace_back(decoder.decode());
    }

    return values;
  }

 private:
  using GorillaState = BareBones::Encoding::Gorilla::GorillaState;
  using Decoder = BareBones::Encoding::Gorilla::ZigZagTimestampDecoder<kDodSignificantLengths>;
  using ValueType = BareBones::Encoding::Gorilla::ValueType;

  BareBones::Encoding::Gorilla::TimestampEncoderState state_;
  BareBones::BitSequenceReader reader_;
  BareBones::Encoding::Gorilla::GorillaState gorilla_state_{GorillaState::kFirstPoint};
};

}  // namespace series_data::encoder::value

template <>
struct BareBones::IsTriviallyReallocatable<series_data::encoder::value::AscIntegerValuesGorillaEncoder> : std::true_type {};
