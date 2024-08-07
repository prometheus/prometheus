#pragma once

#include <algorithm>
#include <bit>
#include <cassert>
#include <cstring>
#include <fstream>
#include <iostream>
#include <type_traits>

#include "bare_bones/preprocess.h"
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

  explicit String() = default;
  explicit String(std::string_view value) : data_(value.data()), len_(value.size()) {}

  PROMPP_ALWAYS_INLINE static String allocate(std::string_view value) noexcept {
    auto data = reinterpret_cast<char*>(std::malloc(value.size() + 1));
    std::memcpy(data, value.data(), value.size());
    data[value.size()] = '\0';

    return String({data, value.size()});
  }

  PROMPP_ALWAYS_INLINE static void free(String& str) noexcept {
    std::free(const_cast<char*>(str.data_));
    str.reset_to(nullptr, 0);
  }

  PROMPP_ALWAYS_INLINE void reset_to(const char* data, size_t len) {
    data_ = data;
    len_ = len;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const char* data() const noexcept { return data_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool empty() const noexcept { return !len_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t size() const noexcept { return len_; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const_iterator begin() const noexcept { return data_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const_iterator end() const noexcept { return data_ + len_; }

  PROMPP_ALWAYS_INLINE const char& operator[](uint32_t i) const {
    assert(i < len_);
    return data_[i];
  }

  explicit PROMPP_ALWAYS_INLINE operator std::string_view() const noexcept { return {data_, len_}; }
};

template <class T>
class Slice {
  static_assert(BareBones::IsTriviallyReallocatable<T>::value, "type parameter of this class should be trivially reallocatable");

  T* data_{};
  size_t len_{};
  size_t cap_{};

 public:
  using iterator_category = std::contiguous_iterator_tag;
  using value_type = T;
  using const_iterator = const T*;
  using iterator = T*;

  Slice() = default;
  explicit Slice(size_t size) { resize(size); }
  Slice(const Slice& o) : len_(o.len_) {
    reserve(len_);
    if constexpr (BareBones::IsTriviallyCopyable<T>::value) {
      std::memcpy(data_, o.data_, len_ * sizeof(T));
    } else {
      for (size_t i = 0; i != len_; ++i) {
        new (data_ + i) T(o[i]);
      }
    }
  }
  Slice(Slice&& other) noexcept : data_(std::exchange(other.data_, nullptr)), len_(std::exchange(other.len_, 0ULL)), cap_(std::exchange(other.cap_, 0ULL)) {}

  Slice& operator=(const Slice& o) noexcept {
    len_ = o.len_;
    reserve(len_);

    if constexpr (BareBones::IsTriviallyCopyable<T>::value) {
      std::memcpy(data_, o.data_, len_ * sizeof(T));
    } else {
      for (size_t i = 0; i != len_; ++i) {
        new (data_ + i) T(o[i]);
      }
    }

    return *this;
  }
  Slice& operator=(Slice&& other) noexcept {
    if (this != &other) {
      [[likely]];
      free();

      data_ = std::exchange(other.data_, nullptr);
      len_ = std::exchange(other.len_, 0ULL);
      cap_ = std::exchange(other.cap_, 0ULL);
    }

    return *this;
  }

  inline __attribute__((always_inline)) void reserve(size_t size) {
    if (size <= cap_) {
      return;
    }
    cap_ = std::max(size, cap_ * 2);
    PRAGMA_DIAGNOSTIC(push)
    PRAGMA_DIAGNOSTIC(ignored DIAGNOSTIC_CLASS_MEMACCESS)
    data_ = reinterpret_cast<T*>(std::realloc(data_, cap_ * sizeof(T)));
    PRAGMA_DIAGNOSTIC(pop)
    if (__builtin_expect(data_ == nullptr, false)) {
      std::abort();
    }
  }

  inline __attribute__((always_inline)) void shrink_to_fit() noexcept {
    if (len_ == cap_) {
      return;
    }
    cap_ = len_;
    PRAGMA_DIAGNOSTIC(push)
    PRAGMA_DIAGNOSTIC(ignored DIAGNOSTIC_CLASS_MEMACCESS)
    data_ = reinterpret_cast<T*>(std::realloc(data_, cap_ * sizeof(T)));
    PRAGMA_DIAGNOSTIC(pop)
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
            PRAGMA_DIAGNOSTIC(push)
            PRAGMA_DIAGNOSTIC(ignored DIAGNOSTIC_CLASS_MEMACCESS)
            std::memset(data_ + len_, 0, (size - len_) * sizeof(T));
            PRAGMA_DIAGNOSTIC(pop)
          } else {
            PRAGMA_DIAGNOSTIC(push)
            PRAGMA_DIAGNOSTIC(ignored DIAGNOSTIC_CLASS_MEMACCESS)
            std::memset(data_ + size, 0, (len_ - size) * sizeof(T));
            PRAGMA_DIAGNOSTIC(pop)
          }
        } else {
          if (size > len_) {
            PRAGMA_DIAGNOSTIC(push)
            PRAGMA_DIAGNOSTIC(ignored DIAGNOSTIC_CLASS_MEMACCESS)
            std::memset(data_ + len_, 0, (size - len_) * sizeof(T));
            PRAGMA_DIAGNOSTIC(pop)
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
    PRAGMA_DIAGNOSTIC(push)
    PRAGMA_DIAGNOSTIC(ignored DIAGNOSTIC_CLASS_MEMACCESS)
    std::ranges::move(last, end(), first);
    PRAGMA_DIAGNOSTIC(pop)
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
        PRAGMA_DIAGNOSTIC(push)
        PRAGMA_DIAGNOSTIC(ignored DIAGNOSTIC_CLASS_MEMACCESS)
        std::fill_n(data_ + len_, count, item);
        PRAGMA_DIAGNOSTIC(pop)
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

class BytesStream : public std::ostream {
 public:
  explicit BytesStream(Slice<char>* s) : std::ostream(&buffer_), buffer_(s) {}

 private:
  class output_buffer : public std::streambuf {
   public:
    explicit output_buffer(Slice<char>* s) : slice_(s) {}

   private:
    Slice<char>* slice_;

    int_type overflow(int_type ch) override {
      slice_->push_back(ch);
      return ch;
    }

    std::streamsize xsputn(const char_type* s, std::streamsize count) override {
      slice_->push_back(s, s + count);
      return count;
    }

    pos_type seekoff(off_type, std::ios_base::seekdir, std::ios_base::openmode) override { return slice_->size(); }
  };

  output_buffer buffer_;
};

}  // namespace PromPP::Primitives::Go

template <class T>
struct BareBones::IsTriviallyReallocatable<PromPP::Primitives::Go::Slice<T>> : std::true_type {};