#pragma once

#include <bit>
#include <cstddef>
#include <cstdint>

#include "bit"
#include "bit_sequence.h"
#include "preprocess.h"

namespace BareBones::Encoding {

class VarInt {
 public:
  static constexpr size_t kMaxVarIntLength = 10;

  PROMPP_ALWAYS_INLINE static size_t write(uint8_t* data, int64_t value) noexcept {
    auto uint_value = std::bit_cast<uint64_t>(value) << 1;
    if (value < 0) {
      uint_value = ~uint_value;
    }

    return write(data, uint_value);
  }

  PROMPP_ALWAYS_INLINE static size_t write(uint8_t* data, uint64_t value) noexcept {
    auto p = data;
    while (value >= 128) {
      *p++ = 0x80 | (value & 0x7f);
      value >>= 7;
    }
    *p++ = static_cast<uint8_t>(value);
    return p - data;
  }

  PROMPP_ALWAYS_INLINE static int64_t read(BitSequenceReader& reader) {
    auto value = read_uint(reader);
    auto result = std::bit_cast<int64_t>(value >> 1);
    if ((value & 1) != 0) {
      result = ~result;
    }

    return result;
  }

  PROMPP_ALWAYS_INLINE static uint64_t read_uint(BitSequenceReader& reader) {
    uint64_t result = 0;
    uint8_t shift = 0;

    for (size_t i = 0; i < kMaxVarIntLength; ++i) {
      auto byte = static_cast<uint64_t>(reader.consume_bits_u32(BareBones::Bit::kByteBits));
      if (byte < 0x80) {
        return result | static_cast<uint64_t>(byte) << shift;
      }

      result |= (byte & 0x7F) << shift;
      shift += 7;
    }

    return result;
  }

  PROMPP_ALWAYS_INLINE static size_t length(uint64_t value) noexcept {
    return value < (1ULL << 7)    ? 1
           : value < (1ULL << 14) ? 2
           : value < (1ULL << 21) ? 3
           : value < (1ULL << 28) ? 4
           : value < (1ULL << 35) ? 5
           : value < (1ULL << 42) ? 6
           : value < (1ULL << 49) ? 7
           : value < (1ULL << 56) ? 8
           : value < (1ULL << 63) ? 9
                                  : 10;
  }
};

}  // namespace BareBones::Encoding