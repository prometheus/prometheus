#pragma once

#include "bitset.h"
#include "type_traits.h"
#include "vector.h"

namespace BareBones {

template <class T, template <class> class Container>
class SparseVector {
  Bitset element_exists_;
  Vector<uint32_t> element_positions_;
  Container<T> data_;

 public:
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool empty() const noexcept { return !element_exists_.size(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t size() const noexcept { return element_exists_.size(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t items_count() const noexcept { return data_.size(); }

  PROMPP_ALWAYS_INLINE void resize(size_t size) noexcept {
    element_exists_.resize(size);
    element_positions_.resize(size);
  }

  PROMPP_ALWAYS_INLINE void clear() noexcept {
    element_exists_.clear();
    element_positions_.clear();
    data_.clear();
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE T& operator[](uint32_t key) noexcept {
    assert(key < element_exists_.size());

    if (element_exists_.is_set(key)) [[unlikely]] {
      return data_[element_positions_[key]];
    }

    element_exists_.set(key);
    element_positions_[key] = data_.size();
    return data_.emplace_back();
  }

  PROMPP_ALWAYS_INLINE const T& operator[](uint32_t key) const noexcept {
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

    PROMPP_ALWAYS_INLINE Iterator() noexcept = default;
    PROMPP_ALWAYS_INLINE explicit Iterator(const SparseVector* vec) noexcept : vec_(vec), i_(vec_->element_exists_.begin()) {}

    PROMPP_ALWAYS_INLINE auto operator*() noexcept { return std::pair<uint32_t, const T&>(*i_, vec_->data_[vec_->element_positions_[*i_]]); }
    PROMPP_ALWAYS_INLINE Iterator& operator++() noexcept {
      ++i_;
      return *this;
    }
    PROMPP_ALWAYS_INLINE Iterator operator++(int) noexcept {
      Iterator retval = *this;
      ++i_;
      return retval;
    }
    PROMPP_ALWAYS_INLINE bool operator==(const Iterator& o) const noexcept { return i_ == o.i_; }
    PROMPP_ALWAYS_INLINE bool operator==(const IteratorSentinel&) const noexcept { return i_ == Bitset::IteratorSentinel(); }
  };

  using const_iterator = Iterator;

  PROMPP_ALWAYS_INLINE auto begin() const noexcept { return Iterator(this); }
  PROMPP_ALWAYS_INLINE auto end() const noexcept { return IteratorSentinel(); }
};

template <class T, template <class> class Container>
struct IsTriviallyReallocatable<SparseVector<T, Container>> : std::true_type {};

template <class T, template <class> class Container>
struct IsZeroInitializable<SparseVector<T, Container>> : std::true_type {};

}  // namespace BareBones
