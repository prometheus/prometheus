#pragma once

#include <array>
#include <bit>
#include <cassert>
#include <cstdint>
#ifdef __x86_64__
#include <x86intrin.h>
#endif

namespace BareBones::Bit {
static_assert(std::endian::native == std::endian::little, "big endian arch is not yet supported");

inline __attribute__((always_inline)) uint32_t bextr(uint32_t src, uint32_t start, uint32_t len) noexcept {
#ifdef __BMI__
  return _bextr_u32(src, start, len);
#else
  assert(start < 32);
  assert(len <= 32);
  return (static_cast<uint64_t>(src) >> start) & ((static_cast<uint64_t>(1) << len) - 1);
#endif
}

inline __attribute__((always_inline)) uint64_t bextr(uint64_t src, uint32_t start, uint32_t len) noexcept {
#ifdef __BMI__
  return _bextr_u64(src, start, len);
#else
  assert(start < 64);
  assert(len <= 64);

  static constexpr auto lut = []() {
    std::array<uint64_t, 65> mask;

    for (uint8_t i = 0; i != 65; ++i) {
      mask[i] = ((static_cast<uint64_t>(i < 64) << (i & 63)) - 1u);
    }

    return mask;
  }();
  return (src >> start) & lut[len];
#endif
}
}  // namespace BareBones::Bit
