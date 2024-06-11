#pragma once

#include "bare_bones/vector.h"
#include "numeric.h"

namespace series_data::encoder {

struct Sample {
  int64_t timestamp{};
  double value{};

  bool operator==(const Sample& other) const noexcept { return timestamp == other.timestamp && value::is_values_strictly_equals(value, other.value); }
  bool operator<(const Sample& other) const noexcept { return timestamp < other.timestamp; }
  bool operator<=(const Sample& other) const noexcept { return timestamp < other.timestamp || value::is_values_strictly_equals(value, other.value); }
  bool operator>(const Sample& other) const noexcept { return !(*this <= other); }
  bool operator>=(const Sample& other) const noexcept { return !(*this < other); }
};

using SampleList = BareBones::Vector<Sample>;

}  // namespace series_data::encoder