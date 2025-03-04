#pragma once

#include <cstdint>
#include <numeric>

namespace BareBones::numeric {
template <class T>
consteval auto select_integral_type() {
  if constexpr (sizeof(T) <= sizeof(std::uint8_t)) {
    return std::uint8_t{};
  } else if constexpr (sizeof(T) <= sizeof(std::uint16_t)) {
    return std::uint16_t{};
  } else if constexpr (sizeof(T) <= sizeof(std::uint32_t)) {
    return std::uint32_t{};
  } else {
    static_assert(sizeof(T) <= sizeof(std::uint64_t), "T is too big to be covered by an integral type!");
    return std::uint64_t{};
  }
}
template <class T>
using integral_type_for = decltype(select_integral_type<T>());
}  // namespace BareBones::numeric