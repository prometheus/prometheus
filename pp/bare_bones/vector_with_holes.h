#pragma once

#include "allocated_memory.h"
#include "preprocess.h"
#include "vector.h"

#include "bitset.h"

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

template <class T>
class VectorWithHoles {
 public:
  ~VectorWithHoles() {
    if constexpr (VectorWithHolesImpl::holes_need_bitset<T>) {
      for_each_item([&](auto& item) { item.destroy_item(); });
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
    }
    return vector_.emplace_back(std::forward<Args>(args)...).value;
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
      allocated_memory += holes_index_set_.allocated_memory();
      for_each_item([&](const auto& item) { allocated_memory += BareBones::mem::allocated_memory(item); });
    }

    return allocated_memory;
  }

 private:
  using Item = VectorWithHolesImpl::ItemOrHole<T>;
  struct nothing_t {};
  // The bitset was chosen as the main data structure based on series_data_encoder_benchmark
  // between Roaring::roaring and BareBones::Bitset
  using BitsetOrEmpty = std::conditional_t<VectorWithHolesImpl::holes_need_bitset<T>, BareBones::Bitset, nothing_t>;

  BareBones::Vector<Item> vector_;
  [[no_unique_address]] BitsetOrEmpty holes_index_set_;
  uint32_t next_hole_{std::numeric_limits<uint32_t>::max()};

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool has_holes() const noexcept { return next_hole_ != std::numeric_limits<uint32_t>::max(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_hole(uint32_t index) const noexcept {
    if constexpr (VectorWithHolesImpl::holes_need_bitset<T>) {
      return holes_index_set_.is_set(index);
    } else {
      if (index >= vector_.size()) {
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

  PROMPP_ALWAYS_INLINE void mark_hole(uint32_t index) noexcept {
    if constexpr (VectorWithHolesImpl::holes_need_bitset<T>) {
      if (index >= holes_index_set_.size()) [[unlikely]] {
        holes_index_set_.resize(index + 1);
      }
      holes_index_set_.set(index);
    }
  }
  PROMPP_ALWAYS_INLINE void unmark_hole(uint32_t index) noexcept {
    if constexpr (VectorWithHolesImpl::holes_need_bitset<T>) {
      holes_index_set_.reset(index);
    }
  }

  // skips holes and applies f for each item
  template <class UnaryFunc>
  PROMPP_ALWAYS_INLINE void for_each_item(UnaryFunc&& f) {
    for_each_item_impl(std::forward<UnaryFunc>(f), *this);
  }

  // const variant
  template <class UnaryFunc>
  PROMPP_ALWAYS_INLINE void for_each_item(UnaryFunc&& f) const {
    for_each_item_impl(std::forward<UnaryFunc>(f), *this);
  }

  template <class UnaryFunc, class Self>
  static PROMPP_ALWAYS_INLINE void for_each_item_impl(UnaryFunc&& f, Self&& self) {
    size_t index_global = 0;
    const auto& holes_index_set = self.holes_index_set_;
    auto& vector = self.vector_;

    for (auto hole_it = holes_index_set.begin(); hole_it != holes_index_set.end(); ++hole_it) {
      for (size_t index = index_global, index_end = *hole_it; index < index_end; ++index) {
        f(vector[index]);
      }
      index_global = (*hole_it) + 1;
    }

    for (size_t index = index_global; index < vector.size(); ++index) {
      f(vector[index]);
    }
  }
};

}  // namespace BareBones