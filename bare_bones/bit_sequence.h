#pragma once

#include <bit>
#include <cstdint>
#include <fstream>

#ifdef __x86_64__
#include <x86intrin.h>
#endif
#ifdef __ARM_FEATURE_CRC32
#include <arm_acle.h>
#endif

#include <scope_exit.h>
#include <span>

#include "bit.h"
#include "exception.h"
#include "memory.h"
#include "preprocess.h"
#include "streams.h"
#include "type_traits.h"

namespace BareBones {

struct AllocationSize {
  uint32_t bits;

  explicit constexpr AllocationSize(uint32_t size_in_bytes) : bits(Bit::to_bits(size_in_bytes)) {}

  PROMPP_ALWAYS_INLINE friend bool operator>(AllocationSize a, uint32_t b) noexcept { return a.bits > b; }
  PROMPP_ALWAYS_INLINE friend bool operator>(uint32_t a, AllocationSize b) noexcept { return a > b.bits; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE constexpr uint32_t bytes() const noexcept { return Bit::to_bytes(bits); }
};

class BitSequenceReader {
 public:
  BitSequenceReader(const Memory<uint8_t>::const_iterator begin, uint64_t size) noexcept : begin_(begin), size_(size) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t read_bits_u32(uint8_t size) const noexcept {
    assert(size <= 32);
    return Bit::bextr(*reinterpret_cast<const uint64_t*>(begin_ + (i_ >> 3)), i_ & 0b111, size);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint64_t read_bits_u56(uint8_t size) const noexcept {
    assert(size <= 56);
    return Bit::bextr(*reinterpret_cast<const uint64_t*>(begin_ + (i_ >> 3)), i_ & 0b111, size);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint64_t read_bits_u64(uint8_t size) const noexcept {
    assert(size <= 64);
    return Bit::bextr(Bit::bextr(*reinterpret_cast<const uint64_t*>(begin_ + (i_ >> 3)), i_ & 0b111, 8 - (i_ & 0b111)) |
                          (Bit::bextr(*reinterpret_cast<const uint64_t*>(begin_ + (i_ >> 3) + 1), 0, 64) << (8 - (i_ & 0b111))),
                      0, size);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t read_u32() const noexcept { return *reinterpret_cast<const uint64_t*>(begin_ + (i_ >> 3)) >> (i_ & 0b111); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint64_t read_u56() const noexcept {
    return Bit::bextr(*reinterpret_cast<const uint64_t*>(begin_ + (i_ >> 3)), i_ & 0b111, 56);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint64_t read_u64() const noexcept {
    return Bit::bextr(*reinterpret_cast<const uint64_t*>(begin_ + (i_ >> 3)), i_ & 0b111, 8 - (i_ & 0b111)) |
           (Bit::bextr(*reinterpret_cast<const uint64_t*>(begin_ + (i_ >> 3) + 1), 0, 64) << (8 - (i_ & 0b111)));
  }

  PROMPP_ALWAYS_INLINE void ff(uint64_t size) noexcept {
    // it is sufficient to check range only here, because it is not allowed to read more than 28 bytes ahead of
    // i_ before calling ff and function reserve_enough_memory always reserve extra 28 bytes on the tail
    assert(i_ + size <= size_);

    i_ += size;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint64_t left() const noexcept { return i_ <= size_ ? size_ - i_ : 0; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool eof() const noexcept { return i_ == size_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint64_t position() const noexcept { return i_; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t consume_bits_u32(uint8_t size) noexcept {
    uint32_t res = read_bits_u32(size);
    ff(size);
    return res;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint64_t consume_bits_u56(uint8_t size) noexcept {
    uint64_t res = read_bits_u56(size);
    ff(size);
    return res;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint64_t consume_bits_u64(uint8_t size) noexcept {
    uint64_t res = read_bits_u64(size);
    ff(size);
    return res;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t consume_u32() noexcept {
    uint32_t res = read_u32();
    ff(32);
    return res;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint64_t consume_u56() noexcept {
    uint64_t res = read_u56();
    ff(56);
    return res;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint64_t consume_u64() noexcept {
    uint64_t res = read_u64();
    ff(64);
    return res;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint64_t consume_u64_svbyte_2468() noexcept {
    const uint8_t code = consume_bits_u32(2);
    const uint8_t size_in_bits = (code + 1) << (1 + 3);

    return consume_bits_u64(size_in_bits);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint64_t consume_u64_svbyte_0248() noexcept {
    const uint8_t code = consume_bits_u32(2);
    const uint8_t size_in_bits = (code + (code == 3)) << (1 + 3);

    return consume_bits_u64(size_in_bits);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE double consume_d64_svbyte_0468() noexcept {
    const uint8_t code = consume_bits_u32(2);
    const uint8_t size_in_bits = (code + (code != 0)) << (1 + 3);

    uint64_t res = consume_bits_u64(size_in_bits) << (64 - size_in_bits);
    return std::bit_cast<double>(res);
  }

 private:
  const uint8_t* begin_;
  uint64_t i_ = 0;
  uint64_t size_;
};

template <std::array kAllocationSizesTable, uint32_t kReservedSizeBits>
  requires std::is_same_v<typename decltype(kAllocationSizesTable)::value_type, AllocationSize>
class PROMPP_ATTRIBUTE_PACKED CompactBitSequenceBase {
 public:
  CompactBitSequenceBase() = default;
  CompactBitSequenceBase(const CompactBitSequenceBase& other)
      : memory_(reinterpret_cast<uint8_t*>(std::malloc(other.allocated_memory()))),
        size_in_bits_(other.size_in_bits_),
        allocation_size_index_(other.allocation_size_index_) {
    std::memcpy(memory_, other.memory_, other.allocated_memory());
  }
  PROMPP_ALWAYS_INLINE CompactBitSequenceBase(CompactBitSequenceBase&& other) noexcept
      : memory_(other.memory_), size_in_bits_(other.size_in_bits_), allocation_size_index_(std::exchange(other.allocation_size_index_, 0)) {
    other.memory_ = nullptr;
    other.size_in_bits_ = 0;
  }

  CompactBitSequenceBase& operator=(const CompactBitSequenceBase& other) {
    if (this != &other) {
      std::free(memory_);

      memory_ = reinterpret_cast<uint8_t*>(std::malloc(other.allocated_memory()));
      std::memcpy(memory_, other.memory_, other.allocated_memory());
      size_in_bits_ = other.size_in_bits_;
      allocation_size_index_ = other.allocation_size_index_;
    }

    return *this;
  }

  CompactBitSequenceBase& operator=(CompactBitSequenceBase&& other) noexcept {
    if (this != &other) {
      std::free(memory_);

      memory_ = other.memory_;
      other.memory_ = nullptr;

      size_in_bits_ = other.size_in_bits_;
      other.size_in_bits_ = 0;

      allocation_size_index_ = std::exchange(other.allocation_size_index_, 0);
    }

    return *this;
  }

  ~CompactBitSequenceBase() {
    std::free(memory_);
    memory_ = nullptr;
  }

  PROMPP_ALWAYS_INLINE bool operator==(const CompactBitSequenceBase& other) const noexcept {
    return size_in_bits_ == other.size_in_bits_ && memcmp(memory_, other.memory_, size_in_bytes()) == 0;
  }

  PROMPP_ALWAYS_INLINE void clear() noexcept {
    size_in_bits_ = 0;
    allocation_size_index_ = 0;
    std::free(memory_);
    memory_ = nullptr;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t size_in_bits() const noexcept { return size_in_bits_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t size_in_bytes() const noexcept { return Bit::to_bytes(size_in_bits_ + 7); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t allocated_memory() const noexcept {
    return is_read_only() ? (size_in_bytes() + Bit::to_bytes(kReservedSizeBits)) : kAllocationSizesTable[allocation_size_index_].bytes();
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint8_t allocation_size_index() const noexcept { return allocation_size_index_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_read_only() const noexcept { return allocation_size_index_ == kNoAllocationIndex; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE std::span<const uint8_t> filled_bytes() const noexcept { return {memory_, Bit::to_bytes(size_in_bits_)}; }
  template <class T>
  [[nodiscard]] PROMPP_ALWAYS_INLINE std::span<const T> bytes() const noexcept {
    return {reinterpret_cast<T*>(memory_), size_in_bytes() / sizeof(T)};
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE std::span<const uint8_t> bytes() const noexcept { return bytes<uint8_t>(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const uint8_t* raw_bytes() const noexcept { return memory_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint8_t* raw_bytes() noexcept { return memory_; }

  PROMPP_ALWAYS_INLINE void shrink_to_fit() noexcept {
    memory_ = reinterpret_cast<uint8_t*>(std::realloc(memory_, size_in_bytes() + Bit::to_bytes(kReservedSizeBits)));
    allocation_size_index_ = kNoAllocationIndex;
  }

 protected:
  static constexpr uint32_t kNoAllocationIndex = std::numeric_limits<uint8_t>::max();

  uint8_t* memory_{};
  uint32_t size_in_bits_{};
  uint8_t allocation_size_index_{};

  void reserve_enough_memory_if_needed() noexcept {
    assert(!is_read_only());

    auto old_size = kAllocationSizesTable[allocation_size_index_];
    if (size_in_bits_ + kReservedSizeBits > old_size.bits) {
      [[unlikely]];
      ++allocation_size_index_;
      assert(allocation_size_index_ < std::size(kAllocationSizesTable));

      auto new_size = kAllocationSizesTable[allocation_size_index_].bytes();
      memory_ = reinterpret_cast<uint8_t*>(std::realloc(memory_, new_size));
      std::memset(memory_ + old_size.bytes(), 0, new_size - old_size.bytes());
    }
  }

  void reserve_enough_memory_if_needed(uint32_t needed_size) noexcept {
    assert(!is_read_only());

    needed_size += size_in_bits_ + kReservedSizeBits;
    auto new_allocation_size_index = allocation_size_index_;
    while (needed_size > kAllocationSizesTable[new_allocation_size_index]) {
      ++new_allocation_size_index;
    }

    if (new_allocation_size_index > allocation_size_index_) {
      auto old_size = kAllocationSizesTable[allocation_size_index_];
      allocation_size_index_ = new_allocation_size_index;
      assert(new_allocation_size_index < std::size(kAllocationSizesTable));

      auto new_size = kAllocationSizesTable[allocation_size_index_].bytes();
      memory_ = reinterpret_cast<uint8_t*>(std::realloc(memory_, new_size));
      std::memset(memory_ + old_size.bytes(), 0, new_size - old_size.bytes());
    }
  }

  template <class T>
  [[nodiscard]] PROMPP_ALWAYS_INLINE T* unfilled_memory() const noexcept {
    return reinterpret_cast<T*>(memory_ + Bit::to_bytes(size_in_bits_));
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t unfilled_bits_in_byte() const noexcept { return size_in_bits_ % 8; }
};

template <std::array kAllocationSizesTable>
  requires std::is_same_v<typename decltype(kAllocationSizesTable)::value_type, AllocationSize>
class PROMPP_ATTRIBUTE_PACKED CompactBitSequence : public CompactBitSequenceBase<kAllocationSizesTable, Bit::to_bits(sizeof(uint64_t) + 1)> {
 public:
  [[nodiscard]] PROMPP_ALWAYS_INLINE BitSequenceReader reader() const noexcept { return {Base::memory_, size_in_bits_}; };

  PROMPP_ALWAYS_INLINE void push_back_single_zero_bit() noexcept {
    reserve_enough_memory_if_needed();
    ++size_in_bits_;
  }
  PROMPP_ALWAYS_INLINE void push_back_single_zero_bit(uint32_t count) noexcept {
    reserve_enough_memory_if_needed(count);
    size_in_bits_ += count;
  }
  PROMPP_ALWAYS_INLINE void push_back_single_one_bit() noexcept {
    reserve_enough_memory_if_needed();
    *Base::template unfilled_memory<uint32_t>() |= 0b1u << unfilled_bits_in_byte();
    ++size_in_bits_;
  }
  PROMPP_ALWAYS_INLINE void push_back_bits_u32(uint32_t size, uint32_t data) noexcept {
    assert(size <= Bit::to_bits(sizeof(uint32_t)));

    reserve_enough_memory_if_needed();
    *Base::template unfilled_memory<uint64_t>() |= static_cast<uint64_t>(data) << unfilled_bits_in_byte();
    size_in_bits_ += size;
  }
  PROMPP_ALWAYS_INLINE void push_back_u64(uint64_t data) noexcept { push_back_bits_u64(64, data); }
  PROMPP_ALWAYS_INLINE void push_back_bits_u64(uint32_t size, uint64_t data) noexcept {
    assert(size <= Bit::to_bits(sizeof(uint64_t)));

    reserve_enough_memory_if_needed();

    auto* memory = Base::template unfilled_memory<uint8_t>();
    *reinterpret_cast<uint64_t*>(memory) |= data << unfilled_bits_in_byte();
    *reinterpret_cast<uint64_t*>(memory + 1) |= data >> (8 - unfilled_bits_in_byte());

    size_in_bits_ += size;
  }
  PROMPP_ALWAYS_INLINE void push_back_d64_svbyte_0468(double val) noexcept {
    // for double skip trail z instead of lead z

    uint8_t size_in_bytes = Bit::to_bytes(64 + 15 - std::countr_zero(std::bit_cast<uint64_t>(val))) & 0b1110;
    size_in_bytes += static_cast<bool>(size_in_bytes & 0b111) << 1;
    const uint8_t code = (size_in_bytes >> 1) - (size_in_bytes != 0);

    push_back_bits_u32(2, code);
    push_back_bits_u64(Bit::to_bits(size_in_bytes), (std::bit_cast<uint64_t>(val)) >> (64 - Bit::to_bits(size_in_bytes)));
  }
  PROMPP_ALWAYS_INLINE void push_back_u64_svbyte_2468(uint64_t val) noexcept {
    uint8_t size_in_bytes = ((64 + 15 - std::countl_zero(val)) >> 3) & 0b1110;

    size_in_bytes += (size_in_bytes == 0) << 1;
    const uint8_t code = (size_in_bytes >> 1) - 1;

    push_back_bits_u32(2, code);
    push_back_bits_u64(size_in_bytes << 3, val);
  }
  PROMPP_ALWAYS_INLINE void push_back_u64_svbyte_0248(uint64_t val) noexcept {
    uint8_t size_in_bytes = ((64 + 15 - std::countl_zero(val)) >> 3) & 0b1110;

    size_in_bytes += (size_in_bytes == 6) << 1;
    const uint8_t code = (size_in_bytes >> 1) - (size_in_bytes == 8);

    push_back_bits_u32(2, code);
    push_back_bits_u64(size_in_bytes << 3, val);
  }

 private:
  using Base = CompactBitSequenceBase<kAllocationSizesTable, Bit::to_bits(sizeof(uint64_t) + 1)>;

  using Base::reserve_enough_memory_if_needed;
  using Base::size_in_bits_;
  using Base::unfilled_bits_in_byte;
  using Base::unfilled_memory;
};

class BitSequence {
  size_t size_ = 0;

  size_t max_size_for_current_data_size_ = 0;
  Memory<uint8_t> data_;

  PROMPP_ALWAYS_INLINE void reserve_enough_memory_if_needed() noexcept {
    if (size_ >= max_size_for_current_data_size_) {
      [[unlikely]];
      reserve_memory(size_);
    }
  }

  PROMPP_ALWAYS_INLINE void reserve_memory(size_t bits_count) noexcept {
    // always reserve at least 12 extra bytes at the tail, because read/write can touch up to 12 bytes
    data_.grow_to_fit_at_least_and_fill_with_zeros((bits_count + 12 * 8 + 7) >> 3);
    max_size_for_current_data_size_ = (data_.size() << 3) - 12 * 8;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint8_t unfilled_bits_count() const noexcept { return 8 - (size_ % 8); }

  PROMPP_ALWAYS_INLINE static void zero_bits(uint8_t& byte, uint8_t bits_count) noexcept {
    static constexpr uint8_t kFilledByte = 0b11111111;

    if (bits_count < 8) {
      byte &= kFilledByte >> bits_count;
    }
  }

 public:
  BitSequence() = default;
  BitSequence(const BitSequence&) = default;
  BitSequence(BitSequence&&) noexcept = default;

  PROMPP_ALWAYS_INLINE BitSequence(const BitSequence& other, size_t bits_count) : size_(bits_count) {
    if (!other.empty()) {
      reserve_memory(other.size_);
    }

    if (!empty()) {
      auto bytes = size_in_bytes();
      std::memcpy(data_, other.data_, bytes);
      zero_bits(data_[bytes - 1], unfilled_bits_count());
    }
  }

  BitSequence& operator=(const BitSequence&) = default;
  BitSequence& operator=(BitSequence&&) noexcept = default;

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t size() const noexcept { return size_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t size_in_bytes() const noexcept { return (size_ + 7) >> 3; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE std::span<const uint8_t> filled_bytes() const noexcept { return {data_.operator const uint8_t*(), size_ / 8}; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE std::span<const uint8_t> bytes() const noexcept { return {data_.operator const uint8_t*(), data_.size()}; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return data_.allocated_memory(); }

  inline __attribute__((always_inline)) bool empty() const noexcept { return !size_; }

  inline __attribute__((always_inline)) void clear() noexcept {
    if (size_ != 0 && data_.size() != 0)
      std::memset(data_, 0, (size_ + 7) >> 3);
    size_ = 0;
  }

  inline __attribute__((always_inline)) void push_back_bits_u64(uint8_t size, uint64_t data) noexcept {
    assert(size <= 64);

    reserve_enough_memory_if_needed();

    *reinterpret_cast<uint64_t*>(data_ + (size_ >> 3)) |= data << (size_ & 0b111u);
    *reinterpret_cast<uint64_t*>(data_ + (size_ >> 3) + 4) |= data >> (32 - (size_ & 0b111u));

    size_ += size;
  }

  inline __attribute__((always_inline)) void push_back_bits_u32(uint8_t size, uint32_t data) noexcept {
    assert(size <= 32);

    reserve_enough_memory_if_needed();

    *reinterpret_cast<uint64_t*>(data_ + (size_ >> 3)) |= data << (size_ & 0b111u);

    size_ += size;
  }

  inline __attribute__((always_inline)) void push_back_u64(uint64_t data) noexcept { push_back_bits_u64(64, data); }

  inline __attribute__((always_inline)) void push_back_u32(uint64_t data) noexcept { push_back_bits_u32(32, data); }

  inline __attribute__((always_inline)) void push_back_single_zero_bit() noexcept {
    reserve_enough_memory_if_needed();
    ++size_;
  }

  inline __attribute__((always_inline)) void push_back_single_one_bit() noexcept {
    reserve_enough_memory_if_needed();
    *reinterpret_cast<uint32_t*>(data_ + (size_ >> 3)) |= 0b1u << (size_ & 0b111u);
    ++size_;
  }

  inline __attribute__((always_inline)) void push_back_u64_svbyte_2468(uint64_t val) noexcept {
    uint8_t size_in_bytes = ((64 + 15 - std::countl_zero(val)) >> 3) & 0b1110;

    size_in_bytes += (size_in_bytes == 0) << 1;
    const uint8_t code = (size_in_bytes >> 1) - 1;

    push_back_bits_u32(2, code);
    push_back_bits_u64(size_in_bytes << 3, val);
  }

  inline __attribute__((always_inline)) void push_back_u64_svbyte_0248(uint64_t val) noexcept {
    uint8_t size_in_bytes = ((64 + 15 - std::countl_zero(val)) >> 3) & 0b1110;

    size_in_bytes += (size_in_bytes == 6) << 1;
    const uint8_t code = (size_in_bytes >> 1) - (size_in_bytes == 8);

    push_back_bits_u32(2, code);
    push_back_bits_u64(size_in_bytes << 3, val);
  }

  inline __attribute__((always_inline)) void push_back_d64_svbyte_0468(double val) noexcept {
    // for double skip trail z instead of lead z

    uint8_t size_in_bytes = ((64 + 15 - std::countr_zero(std::bit_cast<uint64_t>(val))) >> 3) & 0b1110;
    size_in_bytes += static_cast<bool>(size_in_bytes & 0b111) << 1;
    const uint8_t code = (size_in_bytes >> 1) - (size_in_bytes != 0);

    push_back_bits_u32(2, code);
    push_back_bits_u64(size_in_bytes << 3, (std::bit_cast<uint64_t>(val)) >> (64 - (size_in_bytes << 3)));
  }

  inline __attribute__((always_inline)) BitSequenceReader reader() const noexcept { return {data_, size_}; }

  inline __attribute__((always_inline)) size_t save_size() const noexcept {
    // version is written and read by methods put() and get() and they write and read 1 byte
    return 1 + sizeof(size_) + size_in_bytes();
  }

  template <OutputStream S>
  friend S& operator<<(S& out, const BitSequence& seq) {
    auto original_exceptions = out.exceptions();
    auto sg1 = std::experimental::scope_exit([&]() { out.exceptions(original_exceptions); });
    out.exceptions(std::ifstream::failbit | std::ifstream::badbit);

    // write version
    out.put(1);

    // write size
    out.write(reinterpret_cast<const char*>(&seq.size_), sizeof(seq.size_));

    // if there is nothing to write, we finish here
    if (!seq.size_) {
      return out;
    }

    // write data
    out.write(reinterpret_cast<const char*>(static_cast<const uint8_t*>(seq.data_)), seq.size_in_bytes());

    return out;
  }

  template <InputStream S>
  friend S& operator>>(S& in, BitSequence& seq) {
    assert(seq.empty());
    auto sg1 = std::experimental::scope_fail([&]() { seq.clear(); });

    // read version
    uint8_t version = in.get();

    // return successfully, if stream is empty
    if (in.eof())
      return in;

    // check version
    if (version != 1) {
      throw BareBones::Exception(0x0c91c5ebad288f61, "Invalid BitSequence version %d got from input, only version 1 is supported", version);
    }

    auto original_exceptions = in.exceptions();
    auto sg2 = std::experimental::scope_exit([&]() { in.exceptions(original_exceptions); });
    in.exceptions(std::ifstream::failbit | std::ifstream::badbit | std::ifstream::eofbit);

    // read size
    in.read(reinterpret_cast<char*>(&seq.size_), sizeof(seq.size_));

    // read is completed, if there are no items
    if (!seq.size_) {
      return in;
    }

    // read data
    seq.reserve_enough_memory_if_needed();
    in.read(reinterpret_cast<char*>(static_cast<uint8_t*>(seq.data_)), seq.size_in_bytes());

    return in;
  }
};

template <>
struct IsTriviallyReallocatable<BitSequence> : std::true_type {};

template <>
struct IsZeroInitializable<BitSequence> : std::true_type {};

}  // namespace BareBones
