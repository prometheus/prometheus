#pragma once

#include <assert.h>
#include <bit>
#include <cstring>
#include <numeric>

#include "type_traits.h"

namespace BareBones {
template <class T>
class Memory {
  static_assert(IsTriviallyReallocatable<T>::value, "type parameter of this class should be trivially reallocatable");

  T* data_ = nullptr;
  uint32_t size_ = 0;

 public:
  using value_type = T;
  using iterator = T*;
  using const_iterator = const T*;

  inline __attribute__((always_inline)) void resize_to_fit_at_least(size_t size) noexcept {
    if (__builtin_expect(size > std::numeric_limits<uint32_t>::max(), false))
      std::abort();

    size_t new_size;
    if (sizeof(T) < 8 && size * 1.5 * sizeof(T) < 256) {
      // grow 50%, round up to 32b
      new_size = ((static_cast<size_t>(size * sizeof(T) * 1.5) & 0xFFFFFFFFFFFFFFE0) + 32) / sizeof(T);
    } else if (size * 1.5 * sizeof(T) < 4096) {
      // grow 50%, round up to 256b
      new_size = ((static_cast<size_t>(size * sizeof(T) * 1.5) & 0xFFFFFFFFFFFFFF00) + 256) / sizeof(T);
    } else {
      // grow 10%, round up to 4096b
      new_size = ((static_cast<size_t>(size * sizeof(T) * 1.1) & 0xFFFFFFFFFFFFF000) + 4096) / sizeof(T);
      new_size = std::min(new_size, static_cast<size_t>(std::numeric_limits<uint32_t>::max()));
    }

#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Wclass-memaccess"
    data_ = reinterpret_cast<T*>(std::realloc(data_, new_size * sizeof(T)));
#pragma GCC diagnostic pop
    if (__builtin_expect(data_ == nullptr, false)) {
      std::abort();
    }
    size_ = static_cast<uint32_t>(new_size);
  }

  inline __attribute__((always_inline)) void grow_to_fit_at_least(size_t size) noexcept {
    if (size > size_)
      resize_to_fit_at_least(size);
  }

  inline __attribute__((always_inline)) void grow_to_fit_at_least_and_fill_with_zeros(size_t size) noexcept {
    if (size > size_) {
      size_t old_size = size_;
      resize_to_fit_at_least(size);
      std::memset(data_ + old_size, 0, (size_ - old_size) * sizeof(T));
    }
  }

  inline __attribute__((always_inline)) Memory() noexcept = default;

  inline __attribute__((always_inline)) Memory(const Memory& o) noexcept : size_(o.size_) {
    static_assert(IsTriviallyCopyable<T>::value, "it's not allowed to copy memory for non trivially copyable types");

    data_ = reinterpret_cast<T*>(std::realloc(data_, size_ * sizeof(T)));

    if (__builtin_expect(data_ == nullptr, false)) {
      size_ = 0;
      std::abort();
    }

    std::memcpy(data_, o.data_, size_ * sizeof(T));
  }

  inline __attribute__((always_inline)) Memory(Memory&& o) noexcept : size_(o.size_) {
    if (data_)
      std::free(data_);

    data_ = o.data_;

    o.size_ = 0;
    o.data_ = nullptr;
  }

  inline __attribute__((always_inline)) Memory& operator=(const Memory& o) noexcept {
    static_assert(IsTriviallyCopyable<T>::value, "it's not allowed to copy memory for non trivially copyable types");

    if (this != &o) {
      size_ = o.size_;
#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Wclass-memaccess"
      data_ = reinterpret_cast<T*>(std::realloc(data_, size_ * sizeof(T)));
#pragma GCC diagnostic pop
      if (__builtin_expect(data_ == nullptr, false)) {
        size_ = 0;
        std::abort();
      }

#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Wclass-memaccess"
      std::memcpy(data_, o.data_, size_ * sizeof(T));
#pragma GCC diagnostic pop
    }

    return *this;
  }

  inline __attribute__((always_inline)) Memory& operator=(Memory&& o) noexcept {
    if (data_)
      std::free(data_);

    size_ = o.size_;
    data_ = o.data_;

    o.size_ = 0;
    o.data_ = nullptr;

    return *this;
  }

  inline __attribute__((always_inline)) ~Memory() noexcept {
    if (data_)
      std::free(data_);
  }

  // NOLINTNEXTLINE(google-explicit-constructor)
  inline __attribute__((always_inline)) operator const T*() const noexcept { return data_; }

  // NOLINTNEXTLINE(google-explicit-constructor)
  inline __attribute__((always_inline)) operator T*() noexcept { return data_; }

  inline __attribute__((always_inline)) bool empty() const noexcept { return !size_; }

  inline __attribute__((always_inline)) size_t size() const noexcept { return size_; }
};

template <class T>
struct IsTriviallyReallocatable<Memory<T>> : std::true_type {};

template <class T>
struct IsZeroInitializable<Memory<T>> : std::true_type {};
}  // namespace BareBones
