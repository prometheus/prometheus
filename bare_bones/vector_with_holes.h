#pragma once

#include "bitset.h"
#include "preprocess.h"
#include "vector.h"

namespace BareBones {

namespace VectorWithHolesImpl {

template <class T>
union ItemOrHole {
  template <class... Args>
  PROMPP_ALWAYS_INLINE explicit ItemOrHole(Args&&... args) : value{std::forward<Args>(args)...} {}

  PROMPP_ALWAYS_INLINE ~ItemOrHole() {}

  PROMPP_ALWAYS_INLINE void destructor() { value.~T(); }
  PROMPP_ALWAYS_INLINE void destructor(uint32_t next_hole_value) {
    value.~T();
    next_hole = next_hole_value;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept {
    if constexpr (BareBones::concepts::has_allocated_memory<T>) {
      return value.allocated_memory();
    } else if constexpr (BareBones::concepts::dereferenceable_has_allocated_memory<T>) {
      value->allocated_memory();
    } else {
      return 0;
    }
  }

  T value;
  uint32_t next_hole;
};

}  // namespace VectorWithHolesImpl

template <class T>
struct IsTriviallyReallocatable<VectorWithHolesImpl::ItemOrHole<T>> : std::true_type {};

template <class T>
class VectorWithHoles {
 public:
  ~VectorWithHoles() {
    if constexpr (!IsTriviallyDestructible<T>::value) {
      for (auto item_id : item_index_set_) {
        vector_[item_id].destructor();
      }
    }
  }

  template <class... Args>
  PROMPP_ALWAYS_INLINE T& emplace_back(Args&&... args) noexcept {
    if (next_hole_ == std::numeric_limits<uint32_t>::max()) {
      item_index_set_.resize(vector_.size() + 1);
      item_index_set_.set(vector_.size());
      return vector_.emplace_back(std::forward<Args>(args)...).value;
    }

    auto hole_id = next_hole_;
    next_hole_ = vector_[next_hole_].next_hole;

    item_index_set_.set(hole_id);

    auto& item = vector_[hole_id];
    new (&item.value) T(std::forward<Args>(args)...);

    return item.value;
  }

  PROMPP_ALWAYS_INLINE void erase(uint32_t index) {
    assert(item_index_set_[index]);

    vector_[index].destructor(next_hole_);
    next_hole_ = index;
    item_index_set_.reset(index);
  }

  PROMPP_ALWAYS_INLINE uint32_t index_of(const T& item) const noexcept { return reinterpret_cast<const Item*>(&item) - vector_.begin(); }

  PROMPP_ALWAYS_INLINE const T& operator[](uint32_t i) const noexcept { return vector_[i].value; }
  PROMPP_ALWAYS_INLINE T& operator[](uint32_t i) noexcept { return vector_[i].value; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t size() const noexcept { return vector_.size(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept {
    size_t allocated_memory = vector_.capacity() * sizeof(Item) + item_index_set_.allocated_memory();

    if constexpr (BareBones::concepts::has_allocated_memory<T> || BareBones::concepts::dereferenceable_has_allocated_memory<T>) {
      for (auto item_id : item_index_set_) {
        allocated_memory += vector_[item_id].allocated_memory();
      }
    }

    return allocated_memory;
  }

 private:
  using Item = VectorWithHolesImpl::ItemOrHole<T>;

  BareBones::Vector<Item> vector_;
  BareBones::Bitset item_index_set_;
  uint32_t next_hole_{std::numeric_limits<uint32_t>::max()};
};

}  // namespace BareBones