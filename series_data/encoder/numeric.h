#pragma once

#include <cmath>

#include "bare_bones/preprocess.h"

namespace series_data::encoder::value {

PROMPP_ALWAYS_INLINE bool is_int(double value) noexcept {
  return static_cast<double>(static_cast<int64_t>(value)) == value;
}

constexpr PROMPP_ALWAYS_INLINE bool is_positive_int(double value) noexcept {
  return value >= 0.0 && is_int(value);
}

template <class Number1, class Number2>
constexpr PROMPP_ALWAYS_INLINE bool is_values_strictly_equals(Number1 a, Number2 b) noexcept {
  return std::bit_cast<uint64_t>(a) == std::bit_cast<uint64_t>(b);
}

template <class Number1, class Number2>
constexpr PROMPP_ALWAYS_INLINE bool is_in_bounds(double value, Number1 min, Number2 max) noexcept {
  return value >= static_cast<double>(min) && value <= static_cast<double>(max);
}

}  // namespace series_data::encoder::value
