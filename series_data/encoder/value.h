#pragma once

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

class PROMPP_ATTRIBUTE_PACKED GorillaEncoder {
 public:
  explicit GorillaEncoder(double value) { BareBones::Encoding::Gorilla::ValuesEncoder::encode_first(state_, value, stream_); }

  PROMPP_ALWAYS_INLINE void encode(double value) { BareBones::Encoding::Gorilla::ValuesEncoder::encode(state_, value, stream_); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return stream_.allocated_memory(); }

 private:
  BareBones::Encoding::Gorilla::ValuesEncoderState state_;
  BareBones::CompactBitSequence stream_;
};

enum class EncodingType : uint8_t {
  kUnknown,
  kUint32Constant,
  kDoubleConstant,
  kTwoDoubleConstant,
  kGorilla,
};

}  // namespace series_data::encoder::value

namespace BareBones {

template <>
struct IsTriviallyReallocatable<series_data::encoder::value::DoubleConstantEncoder> : std::true_type {};

template <>
struct IsTriviallyReallocatable<series_data::encoder::value::GorillaEncoder> : std::true_type {};

template <>
struct IsTriviallyReallocatable<series_data::encoder::value::TwoDoubleConstantEncoder> : std::true_type {};

}  // namespace BareBones
