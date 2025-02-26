#pragma once

#include <algorithm>
#include <cstring>
#include <ranges>
#include <vector>

#include "bare_bones/preprocess.h"

namespace series_index::querier {

struct SeriesSlice {
  uint32_t begin;
  uint32_t end;

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t count() const noexcept { return end - begin; }
};
using SeriesSliceList = BareBones::Vector<SeriesSlice>;
using SeriesIdSpan = std::span<uint32_t>;

class SetMerger {
 public:
  static SeriesIdSpan merge(SeriesSliceList& slices, uint32_t*& memory, uint32_t*& temp_memory) {
    while (slices.size() > 1) {
      const auto merge_count = slices.size() / 2;
      size_t slices_new_size = 0;
      while (slices_new_size < merge_count) {
        const auto first = &slices[slices_new_size * 2];
        const auto second = first + 1;
        merge(*first, *second, memory, temp_memory);

        slices[slices_new_size++] = SeriesSlice{.begin = first->begin, .end = second->end};
      }

      if (slices.size() % 2 == 1) {
        auto& back = slices.back();
        std::memcpy(temp_memory + back.begin, memory + back.begin, back.count() * sizeof(uint32_t));
        slices[slices_new_size++] = SeriesSlice{.begin = back.begin, .end = back.end};
      }

      slices.resize(slices_new_size);
      std::swap(memory, temp_memory);
    }

    return {memory, slices.empty() ? 0 : slices.begin()->count()};
  }

 private:
  PROMPP_ALWAYS_INLINE static void merge(const SeriesSlice& first, const SeriesSlice& second, uint32_t* memory, uint32_t* temp_memory) {
    std::set_union(memory + first.begin, memory + first.end, memory + second.begin, memory + second.end, temp_memory + first.begin);
  }
};

class SetIntersecter {
 public:
  PROMPP_ALWAYS_INLINE static SeriesIdSpan intersect(SeriesIdSpan set1, SeriesIdSpan set2) {
    return {set1.begin(), std::ranges::set_intersection(set1, set2, set1.begin()).out};
  }
};

class SetSubstractor {
 public:
  template <class Set2>
  PROMPP_ALWAYS_INLINE static SeriesIdSpan substract(SeriesIdSpan set1, const Set2& set2) {
    return {set1.begin(), std::ranges::set_difference(set1, set2, set1.begin()).out};
  }
};

}  // namespace series_index::querier