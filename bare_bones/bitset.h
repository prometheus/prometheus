#pragma once

#include <assert.h>
#ifdef __x86_64__
#include <x86intrin.h>
#endif
#ifdef __ARM_FEATURE_CRC32
#include <arm_acle.h>
#endif

#include <algorithm>
#include <bitset>
#include <ostream>

#include "memory.h"
#include "type_traits.h"

namespace BareBones {
class Bitset {
  /**
   * Why??? Why another bitset??? Why no std::bitset?
   *
   * I've tested std::vector<bool> and roaring bitset, they both are significantly
   * slower:
   * - std::vector<bool> has no way of quickly iterating through set items
   * - roaring bitmap is not that quick if you can afford to hold the whole
   *   bitset in memory (including unset parts), which is the case
   */
  Memory<uint64_t> data_;
  uint32_t size_ = 0;

 public:
  void reserve(size_t size) noexcept {
    if (__builtin_expect(size > std::numeric_limits<uint32_t>::max(), false))
      std::abort();

    uint64_t size_in_uint64_elements = (size + 63) >> 6;

    if (size_in_uint64_elements <= data_.size()) {
      return;
    }

    data_.grow_to_fit_at_least_and_fill_with_zeros(size_in_uint64_elements);
  }

  void resize(size_t size) noexcept {
    reserve(size);

    // unset on downsize
    if (size < size_) {
      uint64_t new_size_in_uint64_elements = (size + 63) >> 6;
      uint64_t original_size_in_uint64_elements = (size_ + 63) >> 6;
      std::memset(data_ + new_size_in_uint64_elements, 0, (original_size_in_uint64_elements - new_size_in_uint64_elements) << 3);
      data_[size >> 6] &= ~(0xFFFFFFFFFFFFFFFF << (size & 0x3F));
    }

    size_ = static_cast<uint32_t>(size);
  }

  // TODO shrink_to_fit

  inline __attribute__((always_inline)) size_t size() const noexcept { return size_; }

  inline __attribute__((always_inline)) size_t capacity() const noexcept { return data_.size() * 64; }

  size_t count(uint32_t v) const noexcept { return v < size_ && (data_[v >> 6] & (1ull << (v & 0x3F))) != 0; }

  void set(uint32_t v) noexcept {
    assert(v < size_);
    data_[v >> 6] |= (1ull << (v & 0x3F));
  }

  inline __attribute__((always_inline)) const bool operator[](uint32_t v) const noexcept {
    assert(v < size_);
    return (data_[v >> 6] & (1ull << (v & 0x3F))) > 0;
  }

  void clear() noexcept {
    if (size_ != 0) {
      uint64_t size_in_uint64_elements = (size_ + 63) >> 6;
      assert(size_in_uint64_elements <= data_.size());
      std::memset(data_, 0, size_in_uint64_elements << 3);
    }
    size_ = 0;
  }

  class IteratorSentinel {};

  class Iterator {
    const uint64_t* data_;

    uint32_t last_block_n_;
    uint32_t block_n_;
    uint64_t block_;
    uint32_t j_;

    inline __attribute__((always_inline)) void next() noexcept {
      if (!block_ && block_n_ != last_block_n_) {
        while (++block_n_ != last_block_n_ && !data_[block_n_]) {
        }
        block_ = data_[block_n_];
      }

      j_ = std::countr_zero(block_);
      block_ &= ~(1ull << j_);
    }

   public:
    using iterator_category = std::input_iterator_tag;
    using value_type = uint32_t;
    using difference_type = std::ptrdiff_t;

    inline __attribute__((always_inline)) explicit Iterator(const uint64_t* data = nullptr, uint32_t size = 0, uint32_t i = 0) noexcept
        : data_(data), last_block_n_(size ? ((size - 1) >> 6) : 0), block_n_(i >> 6), j_(i & 0x3F) {
      block_ = (data_ && size) ? data_[block_n_] : 0;
      next();
    }

    inline __attribute__((always_inline)) uint32_t operator*() noexcept { return (block_n_ << 6) | j_; }
    inline __attribute__((always_inline)) Iterator& operator++() noexcept {
      next();
      return *this;
    }
    inline __attribute__((always_inline)) Iterator operator++(int) noexcept {
      Iterator retval = *this;
      next();
      return retval;
    }
    inline __attribute__((always_inline)) bool operator==(const Iterator& other) const noexcept { return block_n_ == other.block_n_ && j_ == other.j_; }
    inline __attribute__((always_inline)) bool operator==(const IteratorSentinel& other) const noexcept { return block_n_ == last_block_n_ && j_ == 64; }
  };

  using const_iterator = Iterator;

  inline __attribute__((always_inline)) auto begin() const noexcept { return Iterator(data_, size_); }
  inline __attribute__((always_inline)) auto end() const noexcept { return IteratorSentinel(); }
};

template <>
struct IsTriviallyReallocatable<Bitset> : std::true_type {};

template <>
struct IsZeroInitializable<Bitset> : std::true_type {};

}  // namespace BareBones
