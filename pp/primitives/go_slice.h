#pragma once

#include <cassert>
#include <cstring>
#include <fstream>
#include <iostream>
#include <span>
#include <type_traits>

#include "bare_bones/preprocess.h"
#include "bare_bones/type_traits.h"
#include "bare_bones/vector.h"

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

  PROMPP_ALWAYS_INLINE explicit SliceView() = default;

  PROMPP_ALWAYS_INLINE void reset_to(const T* data, size_t len) {
    data_ = data;
    len_ = len;
    cap_ = len;
  }

  PROMPP_ALWAYS_INLINE void reset_to(std::span<const T> buffer) { reset_to(buffer.data(), buffer.size()); }

  PROMPP_ALWAYS_INLINE const T* data() const noexcept { return data_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool empty() const noexcept { return !len_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t size() const noexcept { return len_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t capacity() const noexcept { return cap_; }

  PROMPP_ALWAYS_INLINE const_iterator begin() const noexcept { return data_; }
  PROMPP_ALWAYS_INLINE const_iterator end() const noexcept { return data_ + len_; }

  PROMPP_ALWAYS_INLINE const T& operator[](uint32_t i) const {
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

  PROMPP_ALWAYS_INLINE static String allocate_and_copy(std::string_view value) noexcept {
    auto data = reinterpret_cast<char*>(std::malloc(value.size() + 1));
    std::memcpy(data, value.data(), value.size());
    data[value.size()] = '\0';

    return String({data, value.size()});
  }

  PROMPP_ALWAYS_INLINE static void free_str(String& str) noexcept {
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

  PROMPP_ALWAYS_INLINE bool operator==(const String& other) const noexcept { return this->size() == other.size() && std::ranges::equal(*this, other); }

  explicit PROMPP_ALWAYS_INLINE operator std::string_view() const noexcept { return {data_, len_}; }
};

template <class T>
struct SliceControlBlock {
  using SizeType = size_t;

  SliceControlBlock() = default;
  SliceControlBlock(const SliceControlBlock&) = delete;
  SliceControlBlock(SliceControlBlock&& other) noexcept
      : data(std::exchange(other.data, nullptr)), items_count(std::exchange(other.items_count, 0)), data_size(std::exchange(other.data_size, 0)) {}

  SliceControlBlock& operator=(const SliceControlBlock&) = delete;
  PROMPP_ALWAYS_INLINE SliceControlBlock& operator=(SliceControlBlock&& other) noexcept {
    if (this != &other) [[likely]] {
      data = std::exchange(other.data, nullptr);
      data_size = std::exchange(other.data_size, 0);
      items_count = std::exchange(other.items_count, 0);
    }

    return *this;
  }

  T* data{};
  union {
    SizeType items_count{};
    SizeType length_;
  };
  union {
    SizeType data_size{};
    SizeType capacity_;
  };
};

template <class T>
using Slice = BareBones::MemoryBasedVector<SliceControlBlock, T>;

class BytesStream : public std::ostream {
 public:
  explicit BytesStream(Slice<char>* s) : std::ostream(&buffer_), buffer_(s) {}

  PROMPP_ALWAYS_INLINE void reserve(size_t size) noexcept { buffer_.reserve(size); }

 private:
  class output_buffer : public std::streambuf {
   public:
    explicit output_buffer(Slice<char>* s) : slice_(s) {}

    PROMPP_ALWAYS_INLINE void reserve(size_t size) noexcept { slice_->reserve(size); }

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