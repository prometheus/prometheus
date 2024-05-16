#pragma once

#include "allocation_sizes_table.h"
#include "bare_bones/gorilla.h"

namespace series_data::encoder::value {

class PROMPP_ATTRIBUTE_PACKED Uint32ConstantEncoder {
 public:
  explicit Uint32ConstantEncoder(double value) : value_(uint32_value(value)) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE static bool can_be_encoded(double value) noexcept {
    return std::bit_cast<uint64_t>(value) <= std::numeric_limits<uint32_t>::max();
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool encode(double value) const noexcept { return value_ == uint32_value(value); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE double value() const noexcept { return std::bit_cast<double>(static_cast<uint64_t>(value_)); }

 private:
  const uint32_t value_;

  [[nodiscard]] PROMPP_ALWAYS_INLINE static uint32_t uint32_value(double value) noexcept { return static_cast<uint32_t>(std::bit_cast<uint64_t>(value)); }
};

class DoubleConstantEncoder {
 public:
  explicit DoubleConstantEncoder(double value) : value_(std::bit_cast<uint64_t>(value)) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool encode(double value) const noexcept { return value_ == std::bit_cast<uint64_t>(value); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE double value() const noexcept { return std::bit_cast<double>(value_); }

 private:
  const uint64_t value_;
};

class PROMPP_ATTRIBUTE_PACKED TwoDoubleConstantEncoder {
 public:
  explicit TwoDoubleConstantEncoder(double value1, double value2, uint32_t value1_count)
      : value1_(std::bit_cast<uint64_t>(value1)), value2_(std::bit_cast<uint64_t>(value2)), value1_count_(value1_count) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool encode(double value) const noexcept { return value2_ == std::bit_cast<uint64_t>(value); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE double value1() const noexcept { return std::bit_cast<double>(value1_); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t value1_count() const noexcept { return value1_count_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE double value2() const noexcept { return std::bit_cast<double>(value2_); }

 private:
  const uint64_t value1_;
  uint64_t value2_;
  uint32_t value1_count_;
};

class PROMPP_ATTRIBUTE_PACKED ValuesGorillaEncoder {
 public:
  PROMPP_ALWAYS_INLINE explicit ValuesGorillaEncoder(double value, uint32_t count) {
    BareBones::Encoding::Gorilla::ValuesEncoder::encode_first(state_, value, count, stream_);
  }
  PROMPP_ALWAYS_INLINE explicit ValuesGorillaEncoder(double value) { BareBones::Encoding::Gorilla::ValuesEncoder::encode_first(state_, value, stream_); }

  PROMPP_ALWAYS_INLINE void encode(double value) { BareBones::Encoding::Gorilla::ValuesEncoder::encode(state_, value, stream_); }

  PROMPP_ALWAYS_INLINE void encode(double value, uint32_t count) { BareBones::Encoding::Gorilla::ValuesEncoder::encode(state_, value, count, stream_); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return stream_.allocated_memory(); }

 private:
  BareBones::Encoding::Gorilla::ValuesEncoderState state_;
  BareBones::CompactBitSequence<kAllocationSizesTable> stream_;
};

class PROMPP_ATTRIBUTE_PACKED GorillaEncoder {
 public:
  PROMPP_ALWAYS_INLINE GorillaEncoder(int64_t timestamp, double value) {
    TimestampEncoder::encode(timestamp_state_, timestamp, stream_);
    ValuesEncoder::encode_first(values_state_, value, stream_);
  }

  PROMPP_ALWAYS_INLINE void encode_second(int64_t timestamp, double value) {
    TimestampEncoder::encode_delta(timestamp_state_, timestamp, stream_);
    ValuesEncoder::encode(values_state_, value, stream_);
  }

  PROMPP_ALWAYS_INLINE void encode(int64_t timestamp, double value) {
    TimestampEncoder::encode_delta_of_delta(timestamp_state_, timestamp, stream_);
    ValuesEncoder::encode(values_state_, value, stream_);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return stream_.allocated_memory(); }

 private:
  using TimestampEncoder = BareBones::Encoding::Gorilla::TimestampEncoder;
  using ValuesEncoder = BareBones::Encoding::Gorilla::ValuesEncoder;

  BareBones::Encoding::Gorilla::TimestampEncoderState timestamp_state_;
  BareBones::Encoding::Gorilla::ValuesEncoderState values_state_;
  BareBones::CompactBitSequence<kAllocationSizesTable> stream_;
};

}  // namespace series_data::encoder::value

template <>
struct BareBones::IsTriviallyReallocatable<series_data::encoder::value::DoubleConstantEncoder> : std::true_type {};

template <>
struct BareBones::IsTriviallyReallocatable<series_data::encoder::value::ValuesGorillaEncoder> : std::true_type {};

template <>
struct BareBones::IsTriviallyReallocatable<series_data::encoder::value::TwoDoubleConstantEncoder> : std::true_type {};

template <>
struct BareBones::IsTriviallyReallocatable<series_data::encoder::value::GorillaEncoder> : std::true_type {};
