#pragma once

#include <cstdint>

#include "bare_bones/preprocess.h"
#include "bare_bones/type_traits.h"
#include "series_data/encoder/numeric.h"

namespace series_data::encoder::value {

class PROMPP_ATTRIBUTE_PACKED TwoDoubleConstantEncoder {
 public:
  explicit TwoDoubleConstantEncoder(double value1, double value2, uint8_t value1_count)
      : value1_(std::bit_cast<uint64_t>(value1)), value2_(std::bit_cast<uint64_t>(value2)), value1_count_(value1_count) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_actual(double value) const noexcept { return is_values_strictly_equals(value2_, value); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool encode(double value) const noexcept { return is_actual(value); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE double value1() const noexcept { return std::bit_cast<double>(value1_); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint8_t value1_count() const noexcept { return value1_count_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE double value2() const noexcept { return std::bit_cast<double>(value2_); }

  bool operator==(const TwoDoubleConstantEncoder&) const noexcept = default;

 private:
  const uint64_t value1_;
  const uint64_t value2_;
  const uint8_t value1_count_;
};

}  // namespace series_data::encoder::value

template <>
struct BareBones::IsTriviallyReallocatable<series_data::encoder::value::TwoDoubleConstantEncoder> : std::true_type {};
