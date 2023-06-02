#pragma once

#include <array>
#include <bit>
#include <bitset>
#include <random>
#include <ranges>
#include <thread>

#include "bitset.h"
#include "type_traits.h"
#include "vector.h"

namespace BareBones {
template <class T>
class SparseVector {
  Bitset element_exists_;
  Vector<uint32_t> element_positions_;
  typename std::tuple_element<static_cast<size_t>(IsTriviallyReallocatable<T>::value), std::tuple<std::vector<T>, Vector<T>>>::type data_;

 public:
  inline __attribute__((always_inline)) bool empty() const noexcept { return !element_exists_.size(); }

  inline __attribute__((always_inline)) size_t size() const noexcept { return element_exists_.size(); }

  inline __attribute__((always_inline)) void resize(size_t size) noexcept {
    element_exists_.resize(size);
    element_positions_.resize(size);
  }

  inline __attribute__((always_inline)) void clear() noexcept {
    element_exists_.clear();
    element_positions_.clear();
    data_.clear();
  }

  inline __attribute__((always_inline)) size_t count(uint32_t key) const noexcept { return element_exists_.count(key); }

  inline __attribute__((always_inline)) T& operator[](uint32_t key) noexcept {
    assert(key < element_exists_.size());

    if (__builtin_expect(element_exists_.count(key), false)) {
      return data_[element_positions_[key]];
    } else {
      element_exists_.set(key);
      element_positions_[key] = data_.size();
      data_.resize(data_.size() + 1);
      return data_.back();
    }
  }

  inline __attribute__((always_inline)) const T& operator[](uint32_t key) const noexcept {
    assert(key < element_exists_.size());

    return data_[element_positions_[key]];
  }

  class IteratorSentinel {};

  class Iterator {
    const SparseVector* vec_;
    Bitset::const_iterator i_;

   public:
    using iterator_category = std::input_iterator_tag;
    using value_type = std::pair<uint32_t, const T&>;
    using difference_type = std::ptrdiff_t;

    inline __attribute__((always_inline)) Iterator() noexcept = default;
    inline __attribute__((always_inline)) explicit Iterator(const SparseVector* vec) noexcept : vec_(vec), i_(vec_->element_exists_.begin()) {}

    inline __attribute__((always_inline)) auto operator*() noexcept { return std::pair<uint32_t, const T&>(*i_, vec_->data_[vec_->element_positions_[*i_]]); }
    inline __attribute__((always_inline)) Iterator& operator++() noexcept {
      ++i_;
      return *this;
    }
    inline __attribute__((always_inline)) Iterator operator++(int) noexcept {
      Iterator retval = *this;
      ++i_;
      return retval;
    }
    inline __attribute__((always_inline)) bool operator==(const Iterator& o) const noexcept { return i_ == o.i_; }
    inline __attribute__((always_inline)) bool operator==(const IteratorSentinel& o) const noexcept { return i_ == Bitset::IteratorSentinel(); }
  };

  using const_iterator = Iterator;

  inline __attribute__((always_inline)) auto begin() const noexcept { return Iterator(this); }
  inline __attribute__((always_inline)) auto end() const noexcept { return IteratorSentinel(); }
};

template <class T>
struct IsTriviallyReallocatable<SparseVector<T>> : std::true_type {};

template <class T>
struct IsZeroInitializable<SparseVector<T>> : std::true_type {};

}  // namespace BareBones
