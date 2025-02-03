#pragma once

#include <cstdint>

#include "bare_bones/preprocess.h"
#include "bare_bones/type_traits.h"
#include "series_data/encoder/numeric.h"

namespace series_data::encoder::value {

class PROMPP_ATTRIBUTE_PACKED Float32ConstantEncoder {
 public:
  explicit Float32ConstantEncoder(double value) : value_(float_value(value)) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE static bool can_be_encoded(double value) noexcept { return is_float(value); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_actual(double value) const noexcept { return is_values_strictly_equal(this->value(), value); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool encode(double value) const noexcept { return is_actual(value); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE double value() const noexcept { return value_; }

 private:
  const float value_;

  [[nodiscard]] PROMPP_ALWAYS_INLINE static float float_value(double value) noexcept { return static_cast<float>(value); }
};

}  // namespace series_data::encoder::value

template <>
struct BareBones::IsTriviallyReallocatable<series_data::encoder::value::Float32ConstantEncoder> : std::true_type {};