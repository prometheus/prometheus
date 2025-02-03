#pragma once

#include "bare_bones/preprocess.h"
#include "label_set.h"
#include "sample.h"

namespace PromPP::Primitives {
template <class LabelSetType, class SamplesType = BareBones::Vector<Sample>>
class BasicTimeseries {
  LabelSetType label_set_;
  SamplesType samples_;

 public:
  using label_set_type = LabelSetType;
  using samples_type = SamplesType;

  BasicTimeseries() noexcept = default;
  BasicTimeseries(const BasicTimeseries&) noexcept = default;
  BasicTimeseries& operator=(const BasicTimeseries&) noexcept = default;
  BasicTimeseries(BasicTimeseries&&) noexcept = default;
  BasicTimeseries& operator=(BasicTimeseries&&) noexcept = default;

  BasicTimeseries(const LabelSetType& label_set, SamplesType samples) noexcept : label_set_(label_set), samples_(std::move(samples)) {}

  template <class LabelSet>
  BasicTimeseries(const LabelSet& label_set, SamplesType samples) noexcept : label_set_(label_set), samples_(std::move(samples)) {}

  PROMPP_ALWAYS_INLINE bool operator==(const BasicTimeseries&) const noexcept = default;

  PROMPP_ALWAYS_INLINE auto& label_set() noexcept {
    if constexpr (std::is_pointer_v<LabelSetType>) {
      return *label_set_;
    } else {
      return label_set_;
    }
  }

  PROMPP_ALWAYS_INLINE const auto& label_set() const noexcept {
    if constexpr (std::is_pointer_v<LabelSetType>) {
      return *label_set_;
    } else {
      return label_set_;
    }
  }

  PROMPP_ALWAYS_INLINE void set_label_set(LabelSetType label_set) noexcept {
    static_assert(std::is_pointer_v<LabelSetType>, "this functions can be used only if LabelSetType is a pointer");
    label_set_ = label_set;
  }

  PROMPP_ALWAYS_INLINE const auto& samples() const noexcept {
    if constexpr (std::is_pointer_v<SamplesType>) {
      return *samples_;
    } else {
      return samples_;
    }
  }

  PROMPP_ALWAYS_INLINE auto& samples() noexcept {
    if constexpr (std::is_pointer_v<SamplesType>) {
      return *samples_;
    } else {
      return samples_;
    }
  }

  PROMPP_LAMBDA_INLINE void set_samples(const SamplesType& samples) noexcept {
    static_assert(std::is_pointer_v<SamplesType>, "this functions can be used only if SamplesType is a pointer");
    samples_ = samples;
  }

  PROMPP_ALWAYS_INLINE void clear() noexcept {
    label_set().clear();
    samples().clear();
  }
};

using Timeseries = BasicTimeseries<LabelSet, BareBones::Vector<Sample>>;
using TimeseriesSemiview = BasicTimeseries<LabelViewSet, BareBones::Vector<Sample>>;

}  // namespace PromPP::Primitives