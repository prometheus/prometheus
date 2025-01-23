#pragma once

#include "allocated_memory.h"
#include "preprocess.h"
#include "vector.h"

#include <roaring/roaring.hh>

namespace BareBones {

namespace VectorWithHolesImpl {

template <class T>
union ItemOrHole {
  template <class... Args>
  PROMPP_ALWAYS_INLINE explicit ItemOrHole(Args&&... args) : value{std::forward<Args>(args)...} {}

  PROMPP_ALWAYS_INLINE ~ItemOrHole() {}

  template <class... Args>
  PROMPP_ALWAYS_INLINE void create_item(Args&&... args) {
    std::construct_at(&value, std::forward<Args>(args)...);
  }
  PROMPP_ALWAYS_INLINE void destroy_item() { std::destroy_at(&value); }
  // destroys current item and links hole into linked list
  PROMPP_ALWAYS_INLINE void make_hole(uint32_t next_hole_index) {
    destroy_item();
    next_hole = next_hole_index;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return mem::allocated_memory(value); }

  T value;
  uint32_t next_hole;
};

template <class T>
concept holes_need_bitset =
    BareBones::concepts::has_allocated_memory<T> || BareBones::concepts::dereferenceable_has_allocated_memory<T> || !IsTriviallyDestructible<T>::value;
}  // namespace VectorWithHolesImpl

template <class T>
struct IsTriviallyReallocatable<VectorWithHolesImpl::ItemOrHole<T>> : std::true_type {};

template <class T, uint32_t CompactingCounter = 255>
class VectorWithHoles {
 public:
  ~VectorWithHoles() {
    if constexpr (VectorWithHolesImpl::holes_need_bitset<T>) {
      for (uint32_t i = 0; i < vector_.size(); ++i) {
        if (is_hole(i)) [[unlikely]] {
          continue;
        }
        vector_[i].destroy_item();
      }
    }
  }

  template <class... Args>
  PROMPP_ALWAYS_INLINE T& emplace_back(Args&&... args) noexcept {
    if (has_holes()) [[unlikely]] {
      auto& item = vector_[next_hole_];

      unmark_hole(next_hole_);
      next_hole_ = vector_[next_hole_].next_hole;

      item.create_item(std::forward<Args>(args)...);
      return item.value;
    } else {
      return vector_.emplace_back(std::forward<Args>(args)...).value;
    }
  }

  PROMPP_ALWAYS_INLINE void erase(uint32_t index) {
    assert(index < vector_.size());
    assert(!is_hole(index));

    vector_[index].make_hole(next_hole_);

    next_hole_ = index;

    mark_hole(index);
  }

  PROMPP_ALWAYS_INLINE uint32_t index_of(const T& item) const noexcept { return reinterpret_cast<const Item*>(&item) - vector_.begin(); }

  PROMPP_ALWAYS_INLINE const T& operator[](uint32_t i) const noexcept { return vector_[i].value; }
  PROMPP_ALWAYS_INLINE T& operator[](uint32_t i) noexcept { return vector_[i].value; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t size() const noexcept { return vector_.size(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept {
    size_t allocated_memory = vector_.capacity() * sizeof(Item);
    if constexpr (VectorWithHolesImpl::holes_need_bitset<T>) {
      allocated_memory += holes_index_set_.getSizeInBytes();
      for (uint32_t i = 0; i < vector_.size(); ++i) {
        if (is_hole(i)) [[unlikely]] {
          continue;
        }
        allocated_memory += vector_[i].allocated_memory();
      }
    }

    return allocated_memory;
  }

 private:
  using Item = VectorWithHolesImpl::ItemOrHole<T>;
  struct nothing_t {};
  using BitsetOrEmpty = std::conditional_t<VectorWithHolesImpl::holes_need_bitset<T>, roaring::Roaring, nothing_t>;
  using counter_t = std::conditional_t<VectorWithHolesImpl::holes_need_bitset<T>, uint32_t, nothing_t>;

  BareBones::Vector<Item> vector_;
  [[no_unique_address]] BitsetOrEmpty holes_index_set_;
  uint32_t next_hole_{std::numeric_limits<uint32_t>::max()};
  [[no_unique_address]] counter_t bitset_update_cnt_{};

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool has_holes() const noexcept { return next_hole_ != std::numeric_limits<uint32_t>::max(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_hole(uint32_t index) const noexcept {
    if constexpr (VectorWithHolesImpl::holes_need_bitset<T>) {
      return holes_index_set_.contains(index);
    } else {
      if (index > vector_.size()) {
        return false;
      }
      for (uint32_t current = next_hole_; current != std::numeric_limits<uint32_t>::max(); current = vector_[current].next_hole) {
        if (current == index) {
          return true;
        }
      }
      return false;
    }
  }
  PROMPP_ALWAYS_INLINE void bitset_update() noexcept {
    if constexpr (VectorWithHolesImpl::holes_need_bitset<T>) {
      ++bitset_update_cnt_;
      if (bitset_update_cnt_ == CompactingCounter) [[unlikely]] {
        holes_index_set_.runOptimize();
        holes_index_set_.shrinkToFit();
        bitset_update_cnt_ = 0;
      }
    }
  }
  PROMPP_ALWAYS_INLINE void mark_hole(uint32_t index) noexcept {
    if constexpr (VectorWithHolesImpl::holes_need_bitset<T>) {
      holes_index_set_.add(index);
      bitset_update();
    }
  }
  PROMPP_ALWAYS_INLINE void unmark_hole(uint32_t index) noexcept {
    if constexpr (VectorWithHolesImpl::holes_need_bitset<T>) {
      holes_index_set_.remove(index);
      bitset_update();
    }
  }
};

}  // namespace BareBones