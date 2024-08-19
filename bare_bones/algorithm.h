#pragma once

namespace BareBones {

// implementation for std::ranges::accumulate
// TODO: remove this function on C++23 and use std::ranges::fold_left instead
template <class Range, class ValueType, class Method>
ValueType accumulate(const Range& range, ValueType initial_value, Method&& method) {
  for (auto& item : range) {
    initial_value = method(initial_value, item);
  }

  return initial_value;
}

};  // namespace BareBones