#pragma once

#include <cstdint>

#include "bare_bones/bitset.h"
#include "bare_bones/preprocess.h"

namespace series_index {

class QueriedSeries {
 public:
  enum class Source : uint32_t {
    kRule = 0,
    kFederate,
    kOther,
  };

  PROMPP_ALWAYS_INLINE void set_series_count(uint32_t count) {
    for (auto& queried_series : queried_series_) {
      queried_series.resize(count);
    }
  }

  template <class SeriesIdContainer>
  void set(Source source, const SeriesIdContainer& ids) noexcept {
    for (auto id : ids) {
      queried_series_[static_cast<uint8_t>(source)].set(id);
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t count(Source source) const noexcept { return queried_series_[static_cast<uint8_t>(source)].popcount(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t allocated_memory() const noexcept { return queried_series_[0].allocated_memory() * queried_series_.size(); }

 private:
  std::array<BareBones::Bitset, 3> queried_series_;
};

}  // namespace series_index