#pragma once

#include <cstdint>

#include "bare_bones/preprocess.h"
#include "bare_bones/type_traits.h"
#include "series_data/encoder/numeric.h"

namespace series_data::encoder::value {

class DoubleConstantEncoder {
 public:
  explicit DoubleConstantEncoder(double value) : value_(std::bit_cast<uint64_t>(value)) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_actual(double value) const noexcept { return is_values_strictly_equals(value_, value); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool encode(double value) const noexcept { return is_actual(value); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE double value() const noexcept { return std::bit_cast<double>(value_); }

 private:
  const uint64_t value_;
};

}  // namespace series_data::encoder::value

template <>
struct BareBones::IsTriviallyReallocatable<series_data::encoder::value::DoubleConstantEncoder> : std::true_type {};
