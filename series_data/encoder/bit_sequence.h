#pragma once

#include <array>

#include "bare_bones/bit_sequence.h"

namespace series_data::encoder {

static inline constexpr std::array kAllocationSizesTable = {
    BareBones::AllocationSize(0),    BareBones::AllocationSize(32),   BareBones::AllocationSize(64),   BareBones::AllocationSize(96),
    BareBones::AllocationSize(128),  BareBones::AllocationSize(192),  BareBones::AllocationSize(256),  BareBones::AllocationSize(384),
    BareBones::AllocationSize(512),  BareBones::AllocationSize(640),  BareBones::AllocationSize(768),  BareBones::AllocationSize(1024),
    BareBones::AllocationSize(1152), BareBones::AllocationSize(1280), BareBones::AllocationSize(1408), BareBones::AllocationSize(1536),
    BareBones::AllocationSize(2048), BareBones::AllocationSize(2176), BareBones::AllocationSize(2304), BareBones::AllocationSize(2432),
    BareBones::AllocationSize(2560), BareBones::AllocationSize(3076), BareBones::AllocationSize(3584), BareBones::AllocationSize(4096),
    BareBones::AllocationSize(4608), BareBones::AllocationSize(5120), BareBones::AllocationSize(5632), BareBones::AllocationSize(6144),
    BareBones::AllocationSize(6656), BareBones::AllocationSize(7168), BareBones::AllocationSize(7680), BareBones::AllocationSize(8192),
};

using CompactBitSequence = BareBones::CompactBitSequence<kAllocationSizesTable>;

struct PROMPP_ATTRIBUTE_PACKED BitSequenceWithItemsCount {
  CompactBitSequence stream;

  BitSequenceWithItemsCount() { stream.push_back_bits_u32(BareBones::Bit::to_bits(sizeof(uint8_t)), 1U); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint8_t count() const noexcept { return *reinterpret_cast<const uint8_t*>(stream.raw_bytes()); }
  PROMPP_ALWAYS_INLINE uint8_t inc_count() noexcept { return (*reinterpret_cast<uint8_t*>(stream.raw_bytes()))++; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE BareBones::BitSequenceReader reader() const noexcept { return reader(stream); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static uint8_t count(const CompactBitSequence& stream) noexcept { return count(stream.raw_bytes()); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE static uint8_t count(const uint8_t* buffer) noexcept { return buffer[0]; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static BareBones::BitSequenceReader reader(const CompactBitSequence& stream) noexcept { return reader(stream.bytes()); }

  template <class Buffer>
  [[nodiscard]] PROMPP_ALWAYS_INLINE static BareBones::BitSequenceReader reader(const Buffer& buffer) noexcept {
    BareBones::BitSequenceReader reader{reinterpret_cast<const uint8_t*>(buffer.data()), BareBones::Bit::to_bits(buffer.size())};
    reader.ff(BareBones::Bit::to_bits(sizeof(uint8_t)));
    return reader;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return stream.allocated_memory(); }
};

struct PROMPP_ATTRIBUTE_PACKED RefCountableBitSequenceWithItemsCount {
  encoder::BitSequenceWithItemsCount stream;
  uint32_t reference_count{1};

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return stream.allocated_memory(); }
};

}  // namespace series_data::encoder

template <>
struct BareBones::IsTriviallyReallocatable<series_data::encoder::BitSequenceWithItemsCount> : std::true_type {};

template <>
struct BareBones::IsTriviallyReallocatable<series_data::encoder::CompactBitSequence> : std::true_type {};

template <>
struct BareBones::IsTriviallyReallocatable<series_data::encoder::RefCountableBitSequenceWithItemsCount> : std::true_type {};