#pragma once

#include <assert.h>
#include <algorithm>
#include <bit>
#include <cstring>
#include <fstream>

#include <scope_exit.h>

#include "memory.h"
#include "streams.h"
#include "type_traits.h"

namespace BareBones {

template <class T>
class Vector {
  /**
   * Why??? Why another vector??? Why no std::vector?
   *
   * There are two main reasons for Vector:
   * - it is more compact, because of +10% growth policy (instead of x2),
   * - it uses std::realloc for allocation and reallocation.
   *
   * More information on reasons and motivation can be found in the folly/FBVector documentations.
   * Original implementation (folly/FBVector) is not used because folly is too cumbersome to install.
   */

  static_assert(IsTriviallyReallocatable<T>::value, "type parameter of this class should be trivially reallocatable");

  Memory<T> data_;
  uint32_t size_ = 0;

 public:
  using iterator_category = std::contiguous_iterator_tag;
  using value_type = T;
  using iterator = T*;
  using const_iterator = const T*;

  Vector() noexcept = default;
  Vector(Vector&& o) noexcept : data_(std::move(o.data_)), size_(o.size_) { o.size_ = 0; }
  Vector(const Vector& o) noexcept : size_(o.size_) {
    if constexpr (IsTriviallyCopyable<T>::value) {
      data_ = o.data_;
    } else {
      reserve(size_);
      for (uint32_t i = 0; i != size_; ++i) {
        new (data_ + i) T(o[i]);
      }
    }
  }

  Vector& operator=(Vector&& o) noexcept {
    data_ = std::move(o.data_);
    size_ = o.size_;
    o.size_ = 0;
    return *this;
  }
  Vector& operator=(const Vector& o) noexcept {
    if constexpr (IsTriviallyCopyable<T>::value) {
      size_ = o.size_;
      data_ = o.data_;
    } else {
      if (this != &o) {
        size_ = o.size_;
        reserve(size_);
        for (uint32_t i = 0; i != size_; ++i) {
          new (data_ + i) T(o[i]);
        }
      }
    }

    return *this;
  }

  inline __attribute__((always_inline)) void reserve(size_t size) noexcept { data_.grow_to_fit_at_least(size); }

  inline __attribute__((always_inline)) void shrink_to_fit() noexcept { data_.resize_to_fit_at_least(size_); }

  inline __attribute__((always_inline)) void resize(size_t size) noexcept {
    reserve(size);

    if constexpr (!std::is_trivial<T>::value) {
      if constexpr (IsZeroInitializable<T>::value) {
        if constexpr (IsTriviallyDestructible<T>::value) {
          if (size > size_) {
#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Wclass-memaccess"
            std::memset(data_ + size_, 0, (size - size_) * sizeof(T));
#pragma GCC diagnostic pop
          } else {
#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Wclass-memaccess"
            std::memset(data_ + size, 0, (size_ - size) * sizeof(T));
#pragma GCC diagnostic pop
          }
        } else {
          if (size > size_) {
#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Wclass-memaccess"
            std::memset(data_ + size_, 0, (size - size_) * sizeof(T));
#pragma GCC diagnostic pop
          } else {
            for (uint32_t i = size; i != size_; ++i) {
              (data_ + i)->~T();
            }
          }
        }
      } else {
        if (size > size_) {
          for (uint32_t i = size_; i != size; ++i) {
            new (data_ + i) T();
          }
        } else {
          for (uint32_t i = size; i != size_; ++i) {
            (data_ + i)->~T();
          }
        }
      }
    }

    size_ = static_cast<uint32_t>(size);
  }

  inline __attribute__((always_inline)) ~Vector() noexcept { clear(); }

  inline __attribute__((always_inline)) void clear() noexcept {
    if constexpr (!std::is_trivial<T>::value) {
      if constexpr (IsTriviallyDestructible<T>::value) {
        std::memset(data_, 0, size_ * sizeof(T));
      } else {
        for (uint32_t i = 0; i != size_; ++i) {
          (data_ + i)->~T();
        }
      }
    }

    size_ = 0;
  }

  inline __attribute__((always_inline)) const T* data() const noexcept { return data_; }

  inline __attribute__((always_inline)) T* data() noexcept { return data_; }

  inline __attribute__((always_inline)) bool empty() const noexcept { return !size_; }

  inline __attribute__((always_inline)) size_t size() const noexcept { return size_; }

  inline __attribute__((always_inline)) size_t capacity() const noexcept { return data_.size(); }

  inline __attribute__((always_inline)) void push_back(const T& item) noexcept {
    auto pos = size_;
    resize(static_cast<size_t>(size_) + 1);
    *(data_ + pos) = item;
  }

  inline __attribute__((always_inline)) void push_back(T&& item) noexcept {
    auto pos = size_;
    resize(static_cast<size_t>(size_) + 1);
    *(data_ + pos) = std::move(item);
  }

  inline __attribute__((always_inline)) iterator insert(const iterator pos, const T& item) noexcept {
    assert(pos >= data_);
    assert(pos <= data_ + size_);
    const auto idx = pos - data_;
    reserve(size_ + 1);
#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Wclass-memaccess"
    std::memmove(data_ + idx + 1, data_ + idx, (size_ - idx) * sizeof(T));
#pragma GCC diagnostic pop
    ++size_;
    new (data_ + idx) T(item);
    return data_ + idx;
  }

  inline __attribute__((always_inline)) iterator insert(const iterator pos, T&& item) noexcept {
    assert(pos >= data_);
    assert(pos <= data_ + size_);
    const auto idx = pos - data_;
    reserve(size_ + 1);
    std::memmove(data_ + idx + 1, data_ + idx, (size_ - idx) * sizeof(T));
    ++size_;
    new (data_ + idx) T(std::move(item));
    return data_ + idx;
  }

  template <class... Args>
  inline __attribute__((always_inline)) T& emplace_back(Args&&... args) noexcept {
    reserve(size_ + 1);

    new (data_ + size_) T(std::forward<Args>(args)...);

    return *(data_ + size_++);
  }

  template <std::random_access_iterator IteratorType, class IteratorSentinelType>
    requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, T>::value && std::sentinel_for<IteratorSentinelType, IteratorType>
  inline __attribute__((always_inline)) void push_back(IteratorType begin, IteratorSentinelType end) noexcept {
    auto pos = size_;
    resize(static_cast<size_t>(size_) + (end - begin));
    std::ranges::copy(begin, end, data_ + pos);
  }

  inline __attribute__((always_inline)) const T& back() const noexcept { return data_[size_ - 1]; }

  inline __attribute__((always_inline)) T& back() noexcept { return data_[size_ - 1]; }

  inline __attribute__((always_inline)) iterator begin() noexcept { return data_; }

  inline __attribute__((always_inline)) const_iterator begin() const noexcept { return data_; }

  inline __attribute__((always_inline)) iterator end() noexcept { return data_ + size_; }

  inline __attribute__((always_inline)) const_iterator end() const noexcept { return data_ + size_; }

  inline __attribute__((always_inline)) const T& operator[](uint32_t i) const noexcept {
    assert(i < size_);
    return data_[i];
  }

  inline __attribute__((always_inline)) T& operator[](uint32_t i) noexcept {
    assert(i < size_);
    return data_[i];
  }

  inline __attribute__((always_inline)) bool operator==(const Vector<T>& vec) const { return this->size() == vec.size() && std::ranges::equal(*this, vec); }

  inline __attribute__((always_inline)) size_t save_size() const noexcept {
    // version is written and read by methods put() and get() and they write and read 1 byte
    return 1 + sizeof(size_) + sizeof(T) * size();
  }

  template <OutputStream S>
  friend S& operator<<(S& out, const Vector& vec) {
    auto original_exceptions = out.exceptions();
    auto sg1 = std::experimental::scope_exit([&]() { out.exceptions(original_exceptions); });
    out.exceptions(std::ifstream::failbit | std::ifstream::badbit);

    // write version
    out.put(1);

    // write size
    out.write(reinterpret_cast<const char*>(&vec.size_), sizeof(vec.size_));

    // if there are no items to write, we finish here
    if (!vec.size_) {
      return out;
    }

    // write data
    out.write(reinterpret_cast<const char*>(static_cast<const T*>(vec.data_)), sizeof(T) * vec.size_);

    return out;
  }

  template <InputStream S>
  friend S& operator>>(S& in, Vector& vec) {
    assert(vec.empty());
    auto sg1 = std::experimental::scope_fail([&]() { vec.clear(); });

    // read version
    uint8_t version = in.get();

    // return successfully, if stream is empty
    if (in.eof())
      return in;

    // check version
    if (version != 1) {
      char buf[100];
      std::snprintf(buf, sizeof(buf), "unknown version %d", version);
      throw std::logic_error(buf);
    }

    auto original_exceptions = in.exceptions();
    auto sg2 = std::experimental::scope_exit([&]() { in.exceptions(original_exceptions); });
    in.exceptions(std::ifstream::failbit | std::ifstream::badbit | std::ifstream::eofbit);

    // read size
    uint32_t size_to_read;
    in.read(reinterpret_cast<char*>(&size_to_read), sizeof(size_to_read));

    // read is completed, if there are no items
    if (!size_to_read) {
      return in;
    }

    // read data
    vec.resize(size_to_read);
    in.read(reinterpret_cast<char*>(static_cast<T*>(vec.data_)), sizeof(T) * size_to_read);

    return in;
  }
};

template <class T>
struct IsTriviallyReallocatable<Vector<T>> : std::true_type {};

template <class T>
struct IsZeroInitializable<Vector<T>> : std::true_type {};

}  // namespace BareBones
