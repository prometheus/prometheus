#pragma once

#include "bare_bones/bit_sequence.h"

namespace PromPP::Prometheus::tsdb::chunkenc {

template <std::array kAllocationSizesTable>
  requires std::is_same_v<typename decltype(kAllocationSizesTable)::value_type, BareBones::AllocationSize>
class BStream : public BareBones::CompactBitSequenceBase<kAllocationSizesTable, BareBones::Bit::to_bits(sizeof(uint8_t))> {
 public:
  void write_bits(uint64_t u, uint8_t nbits) noexcept {
    u <<= kUint64Bits - nbits;
    while (nbits >= kByteBits) {
      auto byt = static_cast<uint8_t>(u >> (kUint64Bits - kByteBits));
      write_byte(byt);
      u <<= kByteBits;
      nbits -= kByteBits;
    }

    while (nbits > 0) {
      write_bit(u >> (kUint64Bits - 1) == 1);
      u <<= 1;
      --nbits;
    }
  }

  PROMPP_ALWAYS_INLINE void write_byte(uint8_t byt) noexcept {
    reserve_enough_memory_if_needed();

    auto memory = Base::template unfilled_memory<uint8_t>();
    *memory++ |= byt >> unfilled_bits_in_byte();
    *memory |= byt << rest_of_bits_in_byte();

    size_in_bits_ += kByteBits;
  }

  PROMPP_ALWAYS_INLINE void write_zero_bit() noexcept {
    reserve_enough_memory_if_needed();
    ++size_in_bits_;
  }

  PROMPP_ALWAYS_INLINE void write_single_bit() noexcept {
    reserve_enough_memory_if_needed();
    *Base::template unfilled_memory<uint8_t>() |= 0b1u << (rest_of_bits_in_byte() - 1);
    ++size_in_bits_;
  }

  PROMPP_ALWAYS_INLINE void write_bit(bool bit) noexcept {
    if (bit) {
      write_single_bit();
    } else {
      write_zero_bit();
    }
  }

 private:
  using Base = BareBones::CompactBitSequenceBase<kAllocationSizesTable, BareBones::Bit::to_bits(sizeof(uint8_t))>;

  static constexpr auto kUint64Bits = BareBones::Bit::kUint64Bits;
  static constexpr auto kByteBits = BareBones::Bit::kByteBits;

  using Base::reserve_enough_memory_if_needed;
  using Base::size_in_bits_;
  using Base::unfilled_bits_in_byte;
  using Base::unfilled_memory;

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t rest_of_bits_in_byte() const noexcept { return kByteBits - unfilled_bits_in_byte(); }
};

}  // namespace PromPP::Prometheus::tsdb::chunkenc