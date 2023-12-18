#pragma once

#include <assert.h>
#include <algorithm>
#include <bit>
#include <cstring>
#include <fstream>
#include <iostream>
#include <type_traits>

#include "bare_bones/type_traits.h"

namespace PromPP::Primitives::Go {

template <class T>
class SliceView {
  const T* data_;
  size_t len_;
  size_t cap_;

 public:
  using iterator_category = std::contiguous_iterator_tag;
  using value_type = const T;
  using const_iterator = const T*;

  inline __attribute__((always_inline)) explicit SliceView() = default;

  inline __attribute__((always_inline)) void reset_to(const T* data, size_t len) {
    data_ = data;
    len_ = len;
    cap_ = len;
  }

  inline __attribute__((always_inline)) const T* data() const noexcept { return data_; }
  inline __attribute__((always_inline)) bool empty() const noexcept { return !len_; }
  inline __attribute__((always_inline)) size_t size() const noexcept { return len_; }
  inline __attribute__((always_inline)) size_t capacity() const noexcept { return cap_; }

  inline __attribute__((always_inline)) const_iterator begin() const noexcept { return data_; }
  inline __attribute__((always_inline)) const_iterator end() const noexcept { return data_ + len_; }

  inline __attribute__((always_inline)) const T& operator[](uint32_t i) const {
    assert(i < len_);
    return data_[i];
  }
};  // class SliceView

class String {
  const char* data_;
  size_t len_;

 public:
  using iterator_category = std::contiguous_iterator_tag;
  using value_type = const char;
  using const_iterator = const char*;

  inline __attribute__((always_inline)) explicit String() = default;

  inline __attribute__((always_inline)) void reset_to(const char* data, size_t len) {
    data_ = data;
    len_ = len;
  }

  inline __attribute__((always_inline)) const char* data() const noexcept { return data_; }
  inline __attribute__((always_inline)) bool empty() const noexcept { return !len_; }
  inline __attribute__((always_inline)) size_t size() const noexcept { return len_; }

  inline __attribute__((always_inline)) const_iterator begin() const noexcept { return data_; }
  inline __attribute__((always_inline)) const_iterator end() const noexcept { return data_ + len_; }

  inline __attribute__((always_inline)) const char& operator[](uint32_t i) const {
    assert(i < len_);
    return data_[i];
  }
};  // class String

template <class T>
class Slice {
  static_assert(BareBones::IsTriviallyReallocatable<T>::value, "type parameter of this class should be trivially reallocatable");

  T* data_;
  size_t len_;
  size_t cap_;

 public:
  using iterator_category = std::contiguous_iterator_tag;
  using value_type = T;
  using const_iterator = const T*;
  using iterator = T*;

  Slice() : data_(nullptr), len_(0), cap_(0) {}

  inline __attribute__((always_inline)) void reserve(size_t size) {
    if (size <= cap_) {
      return;
    }
    cap_ = std::max(size, cap_ * 2);
#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Wclass-memaccess"
    data_ = reinterpret_cast<T*>(std::realloc(data_, cap_ * sizeof(T)));
#pragma GCC diagnostic pop
    if (__builtin_expect(data_ == nullptr, false)) {
      std::abort();
    }
  }

  inline __attribute__((always_inline)) void shrink_to_fit() noexcept {
    if (len_ == cap_) {
      return;
    }
    cap_ = len_;
#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Wclass-memaccess"
    data_ = reinterpret_cast<T*>(std::realloc(data_, cap_ * sizeof(T)));
#pragma GCC diagnostic pop
    if (__builtin_expect(data_ == nullptr, false)) {
      std::abort();
    }
  }

  inline __attribute__((always_inline)) void resize(size_t size) noexcept {
    reserve(size);

    if constexpr (!std::is_trivial<T>::value) {
      if constexpr (BareBones::IsZeroInitializable<T>::value) {
        if constexpr (BareBones::IsTriviallyDestructible<T>::value) {
          if (size > len_) {
#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Wclass-memaccess"
            std::memset(data_ + len_, 0, (size - len_) * sizeof(T));
#pragma GCC diagnostic pop
          } else {
#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Wclass-memaccess"
            std::memset(data_ + size, 0, (len_ - size) * sizeof(T));
#pragma GCC diagnostic pop
          }
        } else {
          if (size > len_) {
#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Wclass-memaccess"
            std::memset(data_ + len_, 0, (size - len_) * sizeof(T));
#pragma GCC diagnostic pop
          } else {
            for (uint32_t i = size; i != len_; ++i) {
              (data_ + i)->~T();
            }
          }
        }
      } else {
        if (size > len_) {
          for (uint32_t i = len_; i != size; ++i) {
            new (data_ + i) T();
          }
        } else {
          for (uint32_t i = size; i != len_; ++i) {
            (data_ + i)->~T();
          }
        }
      }
    }

    len_ = size;
  }

  inline __attribute__((always_inline)) iterator erase(iterator first, iterator last) noexcept {
    assert(first >= begin());
    assert(last <= end());
    assert(last >= first);
    if (first == last)
      return end();
    if constexpr (!BareBones::IsTriviallyDestructible<T>::value) {
      for (auto i = first; i != last; ++i) {
        (data_ + i)->~T();
      }
    }
#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Wclass-memaccess"
    std::ranges::move(last, end(), first);
#pragma GCC diagnostic pop
    resize(len_ - (last - first));
    return first;
  }

  inline __attribute__((always_inline)) void clear() noexcept {
    if constexpr (!std::is_trivial<T>::value) {
      if constexpr (BareBones::IsTriviallyDestructible<T>::value) {
        std::memset(data_, 0, len_ * sizeof(T));
      } else {
        for (size_t i = 0; i != len_; ++i) {
          (data_ + i)->~T();
        }
      }
    }

    len_ = 0;
  }

  inline __attribute__((always_inline)) void free() noexcept {
    clear();
    if (data_) {
      std::free(data_);
      len_ = 0;
      cap_ = 0;
    }
  }

  inline __attribute__((always_inline)) ~Slice() noexcept { free(); }

  inline __attribute__((always_inline)) const T* data() const noexcept { return data_; }
  inline __attribute__((always_inline)) T* data() noexcept { return data_; }
  inline __attribute__((always_inline)) bool empty() const noexcept { return !len_; }
  inline __attribute__((always_inline)) size_t size() const noexcept { return len_; }
  inline __attribute__((always_inline)) size_t capacity() const noexcept { return cap_; }

  inline __attribute__((always_inline)) void push_back(const T& item) noexcept {
    auto pos = len_;
    resize(len_ + 1);
    *(data_ + pos) = item;
  }

  inline __attribute__((always_inline)) void push_back(T&& item) noexcept {
    auto pos = len_;
    resize(len_ + 1);
    *(data_ + pos) = std::move(item);
  }

  template <std::random_access_iterator IteratorType, class IteratorSentinelType>
    requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, T>::value && std::sentinel_for<IteratorSentinelType, IteratorType>
  inline __attribute__((always_inline)) void push_back(IteratorType begin, IteratorSentinelType end) noexcept {
    auto pos = len_;
    resize(len_ + (end - begin));
    std::ranges::copy(begin, end, data_ + pos);
  }

  inline __attribute__((always_inline)) void push_back(size_t count, T item) noexcept {
    uint32_t size = len_ + count;
    reserve(size);
    if constexpr (!std::is_trivial<T>::value) {
      if constexpr (BareBones::IsZeroInitializable<T>::value) {
#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Wclass-memaccess"
        std::fill_n(data_ + len_, count, item);
#pragma GCC diagnostic pop
      } else {
        for (uint32_t i = len_; i != size; ++i) {
          new (data_ + i) T(item);
        }
      }
    }

    len_ = size;
  }

  inline __attribute__((always_inline)) const_iterator begin() const noexcept { return data_; }
  inline __attribute__((always_inline)) iterator begin() noexcept { return data_; }
  inline __attribute__((always_inline)) const_iterator end() const noexcept { return data_ + len_; }
  inline __attribute__((always_inline)) iterator end() noexcept { return data_ + len_; }

  inline __attribute__((always_inline)) const T& operator[](uint32_t i) const {
    assert(i < len_);
    return data_[i];
  }

  inline __attribute__((always_inline)) T& operator[](uint32_t i) {
    assert(i < len_);
    return data_[i];
  }
};  // class Slice

class BytesStream {
  Slice<char>* slice_;
  std::ios_base::iostate exceptions_;

 public:
  BytesStream(Slice<char>* s) : slice_(s), exceptions_(std::ifstream::goodbit) {}

  std::ios_base::iostate exceptions() const { return exceptions_; }
  void exceptions(std::ios_base::iostate except) { exceptions_ = except; }

  bool good() const { return true; }
  bool eof() const { return false; }
  bool fail() const { return false; }
  bool bad() const { return false; }

  BytesStream& put(char c) {
    slice_->push_back(c);
    return *this;
  }

  BytesStream& write(const char* s, std::streamsize count) {
    slice_->push_back(s, s + count);
    return *this;
  }

  size_t tellp() const noexcept { return slice_->size(); }

  BytesStream& flush() { return *this; }
};  // class BytesStream

}  // namespace PromPP::Primitives::Go
