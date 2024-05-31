#pragma once

#include <cmath>

#include "bare_bones/preprocess.h"

namespace series_data::encoder::value {

PROMPP_ALWAYS_INLINE bool is_int(double value) noexcept {
  return __builtin_trunc(value) == value;
}

constexpr PROMPP_ALWAYS_INLINE bool is_positive_int(double value) noexcept {
  return value >= 0.0 && is_int(value);
}

template <class Number1, class Number2>
constexpr PROMPP_ALWAYS_INLINE bool is_values_strictly_equals(Number1 a, Number2 b) noexcept {
  return std::bit_cast<uint64_t>(a) == std::bit_cast<uint64_t>(b);
}

}  // namespace series_data::encoder::value
