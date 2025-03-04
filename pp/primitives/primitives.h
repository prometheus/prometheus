#pragma once

#include <cstdint>
#include <limits>
#include <string_view>

#include "bare_bones/preprocess.h"

namespace PromPP::Primitives {

using Symbol = std::string;
using SymbolView = std::string_view;

// FIXME Label and LabelView should be convertable
using Label = std::pair<Symbol, Symbol>;
using LabelView = std::pair<SymbolView, SymbolView>;

using Timestamp = int64_t;
constexpr Timestamp kNullTimestamp = std::numeric_limits<int64_t>::min();

struct TimeInterval {
  static constexpr Timestamp kMin = std::numeric_limits<int64_t>::max();
  static constexpr Timestamp kMax = std::numeric_limits<int64_t>::min();

  Timestamp min{kMin};
  Timestamp max{kMax};

  PROMPP_ALWAYS_INLINE void reset(Timestamp min_value = kMin, Timestamp max_value = kMax) noexcept {
    min = min_value;
    max = max_value;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool contains(Timestamp timestamp) const noexcept { return timestamp >= min && timestamp <= max; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool intersect(const TimeInterval& interval) const noexcept {
    return std::max(min, interval.min) <= std::min(max, interval.max);
  }

  bool operator==(const TimeInterval&) const noexcept = default;
};

using LabelSetID = uint32_t;
constexpr LabelSetID kInvalidLabelSetID = std::numeric_limits<LabelSetID>::max();

}  // namespace PromPP::Primitives

namespace std {
PROMPP_ALWAYS_INLINE constexpr bool operator==(const PromPP::Primitives::LabelView& label_view, const PromPP::Primitives::Label& label) noexcept {
  return label_view.first == label.first && label_view.second == label.second;
}

PROMPP_ALWAYS_INLINE constexpr bool operator==(const PromPP::Primitives::Label& label, const PromPP::Primitives::LabelView& label_view) noexcept {
  return label_view == label;
}

PROMPP_ALWAYS_INLINE constexpr bool operator!=(const PromPP::Primitives::LabelView& label_view, const PromPP::Primitives::Label& label) noexcept {
  return !(label_view == label);
}

PROMPP_ALWAYS_INLINE constexpr bool operator!=(const PromPP::Primitives::Label& label, const PromPP::Primitives::LabelView& label_view) noexcept {
  return !(label_view == label);
}

PROMPP_ALWAYS_INLINE constexpr bool operator<(const PromPP::Primitives::Label& label, const PromPP::Primitives::LabelView& label_view) noexcept {
  return label.first < label_view.first && label.second < label_view.second;
}

PROMPP_ALWAYS_INLINE constexpr bool operator<(const PromPP::Primitives::LabelView& label_view, const PromPP::Primitives::Label& label) noexcept {
  return label_view.first < label.first && label_view.second < label.second;
}
}  // namespace std
