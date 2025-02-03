#pragma once

#include <cmath>

#include "bare_bones/numeric.h"
#include "bare_bones/preprocess.h"

namespace series_data::encoder::value {

PROMPP_ALWAYS_INLINE bool is_int(double value) noexcept {
  return static_cast<double>(static_cast<int64_t>(value)) == value;
}

constexpr PROMPP_ALWAYS_INLINE bool is_positive_int(double value) noexcept {
  return value >= 0.0 && is_int(value);
}

template <class Number1, class Number2>
constexpr PROMPP_ALWAYS_INLINE bool is_values_strictly_equal(Number1 a, Number2 b) noexcept {
  if constexpr (sizeof(Number1) == sizeof(Number2)) {
    using unsigned_integral = BareBones::numeric::integral_type_for<Number1>;
    return std::bit_cast<unsigned_integral>(a) == std::bit_cast<unsigned_integral>(b);
  }
  return false;
}

constexpr PROMPP_ALWAYS_INLINE bool is_float(double value) noexcept {
  return is_values_strictly_equal(value, static_cast<double>(static_cast<float>(value)));
}

template <class Number1, class Number2>
constexpr PROMPP_ALWAYS_INLINE bool is_in_bounds(double value, Number1 min, Number2 max) noexcept {
  return value >= static_cast<double>(min) && value <= static_cast<double>(max);
}

}  // namespace series_data::encoder::value
