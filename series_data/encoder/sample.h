#pragma once

#include "bare_bones/vector.h"
#include "numeric.h"

namespace series_data::encoder {

struct Sample {
  int64_t timestamp{};
  double value{};

  PROMPP_ALWAYS_INLINE bool operator==(const Sample& other) const noexcept {
    return timestamp == other.timestamp && value::is_values_strictly_equals(value, other.value);
  }
  PROMPP_ALWAYS_INLINE bool operator<(const Sample& other) const noexcept { return timestamp < other.timestamp; }
  PROMPP_ALWAYS_INLINE bool operator<=(const Sample& other) const noexcept { return *this < other || *this == other; }
  PROMPP_ALWAYS_INLINE bool operator>(const Sample& other) const noexcept { return timestamp > other.timestamp; }
  PROMPP_ALWAYS_INLINE bool operator>=(const Sample& other) const noexcept { return *this > other || *this == other; }
};

using SampleList = BareBones::Vector<Sample>;

}  // namespace series_data::encoder