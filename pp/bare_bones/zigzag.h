#pragma once

#include <cstdint>

namespace BareBones {
namespace Encoding {

namespace ZigZag {
inline __attribute__((always_inline)) uint64_t encode(int64_t val) noexcept {
  // cppcheck-suppress shiftTooManyBitsSigned
  return (static_cast<uint64_t>(val) + static_cast<uint64_t>(val)) ^ (val >> 63);
}

inline __attribute__((always_inline)) uint32_t encode(int32_t val) noexcept {
  // cppcheck-suppress shiftTooManyBitsSigned
  return (static_cast<uint32_t>(val) + static_cast<uint32_t>(val)) ^ (val >> 31);
}

inline __attribute__((always_inline)) int64_t decode(uint64_t val) noexcept {
  return (val >> 1) ^ (0 - (val & 1));
}

inline __attribute__((always_inline)) int32_t decode(uint32_t val) noexcept {
  return (val >> 1) ^ (0 - (val & 1));
}
}  // namespace ZigZag

}  // namespace Encoding
}  // namespace BareBones
