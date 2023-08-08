#pragma once

#include <bit>
#include <cstdint>
#include <fstream>
#include <string_view>
#ifdef __x86_64__
#include <x86intrin.h>
#endif
#ifdef __ARM_FEATURE_CRC32
#include <arm_acle.h>
#endif

#include <scope_exit.h>

#include "bit.h"
#include "exception.h"
#include "memory.h"
#include "streams.h"
#include "type_traits.h"

namespace BareBones {
class BitSequence {
  size_t size_ = 0;

  size_t max_size_for_current_data_size_ = 0;
  Memory<uint8_t> data_;

  inline __attribute__((always_inline)) void reserve_enough_memory() noexcept {
    if (__builtin_expect(size_ >= max_size_for_current_data_size_, false)) {
      // always reserve at least 28 extra bytes at the tail, because read/write can touch
      // up to 12 bytes and we allow "from" to be up to 16 bytes (128 bit) ahead
      data_.grow_to_fit_at_least_and_fill_with_zeros((size_ + 28 * 8 + 7) << 3);
      max_size_for_current_data_size_ = (data_.size() << 3) - 28 * 8;
    }
  }

 public:
  inline __attribute__((always_inline)) size_t size() const noexcept { return size_; }

  inline __attribute__((always_inline)) bool empty() const noexcept { return !size_; }

  inline __attribute__((always_inline)) void clear() noexcept {
    if (size_ != 0 && data_.size() != 0)
      std::memset(data_, 0, (size_ + 7) >> 3);
    size_ = 0;
  }

  inline __attribute__((always_inline)) void push_back_bits_u64(uint8_t size, uint64_t data) noexcept {
    assert(size <= 64);

    reserve_enough_memory();

    *reinterpret_cast<uint64_t*>(data_ + (size_ >> 3)) |= data << (size_ & 0b111u);
    *reinterpret_cast<uint64_t*>(data_ + (size_ >> 3) + 4) |= data >> (32 - (size_ & 0b111u));

    size_ += size;
  }

  inline __attribute__((always_inline)) void push_back_bits_u32(uint8_t size, uint32_t data) noexcept {
    assert(size <= 32);

    reserve_enough_memory();

    *reinterpret_cast<uint64_t*>(data_ + (size_ >> 3)) |= data << (size_ & 0b111u);

    size_ += size;
  }

  inline __attribute__((always_inline)) void push_back_u64(uint64_t data) noexcept { push_back_bits_u64(64, data); }

  inline __attribute__((always_inline)) void push_back_u32(uint64_t data) noexcept { push_back_bits_u32(32, data); }

  inline __attribute__((always_inline)) void push_back_single_zero_bit() noexcept {
    reserve_enough_memory();
    ++size_;
  }

  inline __attribute__((always_inline)) void push_back_single_one_bit() noexcept {
    reserve_enough_memory();
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

  class Reader {
    const Memory<uint8_t>::const_iterator begin_;
    uint64_t i_ = 0;
    const uint64_t size_;

   public:
    inline __attribute__((always_inline)) uint32_t read_bits_u32(uint8_t size) const noexcept {
      assert(size <= 32);
      return Bit::bextr(*reinterpret_cast<const uint64_t*>(begin_ + (i_ >> 3)), i_ & 0b111, size);
    }

    inline __attribute__((always_inline)) uint32_t read_bits_u32(uint64_t from, uint8_t size) const noexcept {
      assert(size <= 32);
      assert(from <= 128);
      return Bit::bextr(*reinterpret_cast<const uint64_t*>(begin_ + ((i_ + from) >> 3)), (i_ + from) & 0b111, size);
    }

    inline __attribute__((always_inline)) uint64_t read_bits_u56(uint8_t size) const noexcept {
      assert(size <= 56);
      return Bit::bextr(*reinterpret_cast<const uint64_t*>(begin_ + (i_ >> 3)), i_ & 0b111, size);
    }

    inline __attribute__((always_inline)) uint64_t read_bits_u56(uint64_t from, uint8_t size) const noexcept {
      assert(size <= 56);
      assert(from <= 128);
      return Bit::bextr(*reinterpret_cast<const uint64_t*>(begin_ + ((i_ + from) >> 3)), (i_ + from) & 0b111, size);
    }

    inline __attribute__((always_inline)) uint64_t read_bits_u64(uint8_t size) const noexcept {
      assert(size <= 64);
      return Bit::bextr(Bit::bextr(*reinterpret_cast<const uint64_t*>(begin_ + (i_ >> 3)), i_ & 0b111, 32) |
                            (Bit::bextr(*reinterpret_cast<const uint64_t*>(begin_ + (i_ >> 3) + 4), i_ & 0b111, 32) << 32),
                        0, size);
    }

    inline __attribute__((always_inline)) uint64_t read_bits_u64(uint64_t from, uint8_t size) const noexcept {
      assert(size <= 64);
      assert(from <= 128);
      return Bit::bextr(Bit::bextr(*reinterpret_cast<const uint64_t*>(begin_ + ((i_ + from) >> 3)), (i_ + from) & 0b111, 32) |
                            (Bit::bextr(*reinterpret_cast<const uint64_t*>(begin_ + ((i_ + from + 32) >> 3)), (i_ + from) & 0b111, 32) << 32),
                        0, size);
    }

    inline __attribute__((always_inline)) uint32_t read_u32() const noexcept { return *reinterpret_cast<const uint64_t*>(begin_ + (i_ >> 3)) >> (i_ & 0b111); }

    inline __attribute__((always_inline)) uint64_t read_u56() const noexcept {
      return Bit::bextr(*reinterpret_cast<const uint64_t*>(begin_ + (i_ >> 3)), i_ & 0b111, 56);
    }

    inline __attribute__((always_inline)) uint64_t read_u64() const noexcept {
      return Bit::bextr(*reinterpret_cast<const uint64_t*>(begin_ + (i_ >> 3)), i_ & 0b111, 32) |
             (Bit::bextr(*reinterpret_cast<const uint64_t*>(begin_ + (i_ >> 3) + 4), i_ & 0b111, 32) << 32);
    }

    inline __attribute__((always_inline)) uint32_t read_u32(uint8_t from) const noexcept {
      assert(from <= 128);
      return *reinterpret_cast<const uint64_t*>(begin_ + ((i_ + from) >> 3)) >> ((i_ + from) & 0b111);
    }

    inline __attribute__((always_inline)) uint64_t read_u56(uint8_t from) const noexcept {
      assert(from <= 128);
      return Bit::bextr(*reinterpret_cast<const uint64_t*>(begin_ + ((i_ + from) >> 3)), (i_ + from) & 0b111, 56);
    }

    inline __attribute__((always_inline)) uint64_t read_u64(uint8_t from) const noexcept {
      assert(from <= 128);
      return Bit::bextr(*reinterpret_cast<const uint64_t*>(begin_ + ((i_ + from) >> 3)), (i_ + from) & 0b111, 32) |
             (Bit::bextr(*reinterpret_cast<const uint64_t*>(begin_ + ((i_ + from + 32) >> 3)), (i_ + from) & 0b111, 32) << 32);
    }

    inline __attribute__((always_inline)) void ff(uint64_t size) noexcept {
      // it is sufficient to check range only here, because it is not allowed to read more than 28 bytes ahead of
      // i_ before calling ff and function reserve_enough_memory always reserve extra 28 bytes on the tail
      assert(i_ + size <= size_);

      i_ += size;
    }

    inline __attribute__((always_inline)) uint64_t left() const noexcept { return i_ <= size_ ? size_ - i_ : 0; }

    inline __attribute__((always_inline)) uint32_t consume_bits_u32(uint8_t size) noexcept {
      uint32_t res = read_bits_u32(size);
      ff(size);
      return res;
    }

    inline __attribute__((always_inline)) uint64_t consume_bits_u56(uint8_t size) noexcept {
      uint64_t res = read_bits_u56(size);
      ff(size);
      return res;
    }

    inline __attribute__((always_inline)) uint64_t consume_bits_u64(uint8_t size) noexcept {
      uint64_t res = read_bits_u64(size);
      ff(size);
      return res;
    }

    inline __attribute__((always_inline)) uint32_t consume_u32() noexcept {
      uint32_t res = read_u32();
      ff(32);
      return res;
    }

    inline __attribute__((always_inline)) uint64_t consume_u56() noexcept {
      uint64_t res = read_u56();
      ff(56);
      return res;
    }

    inline __attribute__((always_inline)) uint64_t consume_u64() noexcept {
      uint64_t res = read_u64();
      ff(64);
      return res;
    }

    inline __attribute__((always_inline)) uint64_t consume_u64_svbyte_2468() noexcept {
      const uint8_t code = consume_bits_u32(2);
      const uint8_t size_in_bits = (code + 1) << (1 + 3);

      return consume_bits_u64(size_in_bits);
    }

    inline __attribute__((always_inline)) uint64_t consume_u64_svbyte_0248() noexcept {
      const uint8_t code = consume_bits_u32(2);
      const uint8_t size_in_bits = (code + (code == 3)) << (1 + 3);

      return consume_bits_u64(size_in_bits);
    }

    inline __attribute__((always_inline)) double consume_d64_svbyte_0468() noexcept {
      const uint8_t code = consume_bits_u32(2);
      const uint8_t size_in_bits = (code + (code != 0)) << (1 + 3);

      uint64_t res = consume_bits_u64(size_in_bits) << (64 - size_in_bits);
      return std::bit_cast<double>(res);
    }

    Reader(const Memory<uint8_t>::const_iterator begin, uint64_t size) noexcept : begin_(begin), size_(size) {}
  };

  inline __attribute__((always_inline)) Reader reader() const noexcept { return Reader(data_, size_); }

  inline __attribute__((always_inline)) size_t save_size() const noexcept {
    // version is written and read by methods put() and get() and they write and read 1 byte
    return 1 + sizeof(size_) + ((size_ + 7) >> 3);
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
    out.write(reinterpret_cast<const char*>(static_cast<const uint8_t*>(seq.data_)), (seq.size_ + 7) >> 3);

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
      char buf[100];
      size_t l = std::snprintf(buf, sizeof(buf), "Invalid BitSequence version %d got from input, only version 1 is supported", version);
      throw BareBones::Exception(0x0c91c5ebad288f61, std::string_view(buf, l));
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
    seq.reserve_enough_memory();
    in.read(reinterpret_cast<char*>(static_cast<uint8_t*>(seq.data_)), (seq.size_ + 7) >> 3);

    return in;
  }
};

template <>
struct IsTriviallyReallocatable<BitSequence> : std::true_type {};

template <>
struct IsZeroInitializable<BitSequence> : std::true_type {};

}  // namespace BareBones
