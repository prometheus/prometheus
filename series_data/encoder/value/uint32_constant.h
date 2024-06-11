#pragma once

#include <cstdint>

#include "bare_bones/preprocess.h"
#include "bare_bones/type_traits.h"
#include "series_data/encoder/numeric.h"

namespace series_data::encoder::value {

class PROMPP_ATTRIBUTE_PACKED Uint32ConstantEncoder {
 public:
  explicit Uint32ConstantEncoder(double value) : value_(uint32_value(value)) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE static bool can_be_encoded(double value) noexcept {
    return is_positive_int(value) && static_cast<uint64_t>(value) <= std::numeric_limits<uint32_t>::max();
    // return is_positive_int(value) && static_cast<uint64_t>(uint32_value(value)) == static_cast<uint64_t>(value);
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_actual(double value) const noexcept { return can_be_encoded(value) && value_ == uint32_value(value); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool encode(double value) const noexcept { return is_actual(value); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE double value() const noexcept { return static_cast<double>(static_cast<uint64_t>(value_)); }

 private:
  const uint32_t value_;

  [[nodiscard]] PROMPP_ALWAYS_INLINE static uint32_t uint32_value(double value) noexcept { return static_cast<uint32_t>(value); }
};

}  // namespace series_data::encoder::value

template <>
struct BareBones::IsTriviallyReallocatable<series_data::encoder::value::Uint32ConstantEncoder> : std::true_type {};
