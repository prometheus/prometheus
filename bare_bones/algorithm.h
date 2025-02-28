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

template <class Value, class... Args>
bool is_in(const Value& value, Args&&... args) {
  return (... || (value == std::forward<Args>(args)));
}

};  // namespace BareBones