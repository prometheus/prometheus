#pragma once

#include <parallel_hashmap/phmap.h>

#include "bare_bones/preprocess.h"
#include "bare_bones/snug_composite.h"
#include "bare_bones/vector.h"
#include "primitives/snug_composites_filaments.h"

namespace series_index {

template <class Set, uint32_t kMaxIndexValue = std::numeric_limits<uint32_t>::max()>
class SortingIndex {
 public:
  explicit SortingIndex(const Set& ls_id_set) : ls_id_set_(ls_id_set) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool empty() const noexcept { return sorting_index_.empty(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return sorting_index_.allocated_memory(); }

  void build() {
    sorting_index_.resize(ls_id_set_.size());

    uint32_t step = kMaxIndexValue / (ls_id_set_.size() + 1);
    uint32_t index_value = 0;
    for (auto ls_id : ls_id_set_) {
      index_value += step;
      sorting_index_[ls_id] = index_value;
    }
  }

  PROMPP_ALWAYS_INLINE void update(Set::const_iterator ls_id_iterator) {
    uint64_t previous = get_previous(ls_id_iterator);
    uint64_t next = get_next(ls_id_iterator);
    uint32_t value = (previous + next) / 2;
    if (value > previous) {
      sorting_index_.emplace_back(value);
    } else {
      build();
    }
  }

  template <class Iterator>
  void sort(Iterator begin, Iterator end) const noexcept {
    std::sort(begin, end, [this](uint32_t a, uint32_t b) PROMPP_LAMBDA_INLINE { return sorting_index_[a] < sorting_index_[b]; });
  }

 private:
  const Set& ls_id_set_;
  BareBones::Vector<uint32_t> sorting_index_;

  PROMPP_ALWAYS_INLINE uint32_t get_previous(Set::const_iterator ls_id_iterator) const noexcept {
    if (ls_id_iterator != ls_id_set_.begin()) {
      return sorting_index_[*--ls_id_iterator];
    }

    return 0;
  }

  PROMPP_ALWAYS_INLINE uint32_t get_next(Set::const_iterator ls_id_iterator) const noexcept {
    if (++ls_id_iterator != ls_id_set_.end()) {
      return sorting_index_[*ls_id_iterator];
    }

    return kMaxIndexValue;
  }
};

}  // namespace series_index