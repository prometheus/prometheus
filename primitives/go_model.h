#pragma once

#include <string_view>

#include "bare_bones/algorithm.h"
#include "primitives/go_slice.h"
#include "primitives/primitives.h"

namespace PromPP::Primitives::Go {

struct StringView {
  uint32_t begin;
  uint32_t length;
};

struct LabelView {
  StringView name;
  StringView value;
};

struct LabelSet {
  class IteratorSentinel {};

  template <class Derived>
  class PairsIterator {
   public:
    using iterator_category = std::forward_iterator_tag;
    using difference_type = ptrdiff_t;

    explicit PairsIterator(const LabelSet* label_set) : label_set_(label_set), iterator_(label_set->pairs.begin()) {}

    PROMPP_ALWAYS_INLINE Derived& operator++() noexcept {
      ++iterator_;
      return *static_cast<Derived*>(this);
    }

    PROMPP_ALWAYS_INLINE Derived operator++(int) noexcept {
      auto it = *this;
      ++*this;
      return it;
    }

    PROMPP_ALWAYS_INLINE bool operator==(const IteratorSentinel&) const noexcept { return iterator_ == label_set_->pairs.end(); }

   protected:
    const LabelSet* label_set_;
    PromPP::Primitives::Go::SliceView<LabelView>::const_iterator iterator_;
  };

  class Iterator : public PairsIterator<Iterator> {
   public:
    using PairsIterator::difference_type;
    using PairsIterator::iterator_category;
    using PairsIterator::PairsIterator;
    using value_type = std::pair<std::string_view, std::string_view>;
    using pointer = value_type*;
    using reference = value_type&;

    [[nodiscard]] PROMPP_ALWAYS_INLINE const value_type operator*() const noexcept {
      return {{label_set_->data.data() + iterator_->name.begin, iterator_->name.length},
              {label_set_->data.data() + iterator_->value.begin, iterator_->value.length}};
    }
  };

  struct Names {
    class NamesIterator : public PairsIterator<NamesIterator> {
     public:
      using PairsIterator::difference_type;
      using PairsIterator::iterator_category;
      using PairsIterator::PairsIterator;
      using value_type = std::string_view;
      using pointer = value_type*;
      using reference = value_type&;

      [[nodiscard]] PROMPP_ALWAYS_INLINE const value_type operator*() const noexcept {
        return {label_set_->data.data() + iterator_->name.begin, iterator_->name.length};
      }
    };

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t size() const noexcept { return label_set->pairs.size(); }

    [[nodiscard]] PROMPP_ALWAYS_INLINE auto begin() const noexcept { return NamesIterator(label_set); }
    [[nodiscard]] PROMPP_ALWAYS_INLINE static IteratorSentinel end() noexcept { return {}; }

    PROMPP_ALWAYS_INLINE friend size_t hash_value(const Names& label_set_names) noexcept {
      return BareBones::accumulate(label_set_names, 0ULL, [](size_t hash, const auto& label_name) PROMPP_LAMBDA_INLINE {
        return XXH3_64bits_withSeed(label_name.data(), label_name.size(), hash);
      });
    }

    const LabelSet* label_set;
  };

  PromPP::Primitives::Go::SliceView<char> data;
  PromPP::Primitives::Go::SliceView<LabelView> pairs;

  [[nodiscard]] PROMPP_ALWAYS_INLINE auto begin() const noexcept { return Iterator(this); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE static IteratorSentinel end() noexcept { return {}; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE auto names() const noexcept { return Names(this); }

  PROMPP_ALWAYS_INLINE friend size_t hash_value(const LabelSet& label_set) noexcept {
    return BareBones::accumulate(label_set, 0ULL, [](size_t hash, const auto& label) PROMPP_LAMBDA_INLINE {
      return XXH3_64bits_withSeed(label.first.data(), label.first.size(), hash) ^ XXH3_64bits_withSeed(label.second.data(), label.second.size(), hash);
    });
  }
};

struct TimeSeries {
  LabelSet label_set;
  uint64_t timestamp;
  double value;
};

struct LabelSetLimits {
  uint32_t max_label_name_length;
  uint32_t max_label_value_length;
  uint32_t max_label_count;
};

template <class TimeSeriesLabelSet>
void read_label_set(const LabelSet& go_label_set, TimeSeriesLabelSet& time_series_label_set) {
  for (auto& go_label_view : go_label_set.pairs) {
    typename TimeSeriesLabelSet::label_type label;
    auto name = std::string_view(go_label_set.data.data() + go_label_view.name.begin, go_label_view.name.length);
    label.first = name;
    auto value = std::string_view(go_label_set.data.data() + go_label_view.value.begin, go_label_view.value.length);
    label.second = value;
    time_series_label_set.add(label);
  }
}

template <class TimeSeriesLabelSet>
void read_label_set(const LabelSet& go_label_set, TimeSeriesLabelSet& time_series_label_set, LabelSetLimits& limits) {
  if (limits.max_label_count && go_label_set.pairs.size() > limits.max_label_count) {
    throw BareBones::Exception(0x18ffd63c691bdb60, "Max label count exceeded");
  }
  for (auto& go_label_view : go_label_set.pairs) {
    typename TimeSeriesLabelSet::label_type label;
    auto name = std::string_view(go_label_set.data.data() + go_label_view.name.begin, go_label_view.name.length);
    if (limits.max_label_name_length && std::size(name) > limits.max_label_name_length) {
      throw BareBones::Exception(0x91b0aa8a8eb15681, "Label name size (%zd) exceeds the maximum name size limit", std::size(name));
    }
    label.first = name;
    auto value = std::string_view(go_label_set.data.data() + go_label_view.value.begin, go_label_view.value.length);
    if (limits.max_label_value_length && std::size(value) > limits.max_label_value_length) {
      throw BareBones::Exception(0x3214247d751d903e, "Label value size (%zd) exceeds the maximum value size limit", std::size(value));
    }
    label.second = value;
    time_series_label_set.add(label);
  }
  if (time_series_label_set.size() == 0) {
    throw BareBones::Exception(0x2c52a6423c07e065, "Label set is empty");
  }
}

template <class Samples>
void read_samples(const TimeSeries& go_time_series, Samples& samples) {
  typename Samples::value_type sample;
  sample.timestamp() = go_time_series.timestamp;
  sample.value() = go_time_series.value;
  samples.push_back(sample);
}

template <class Timeseries>
void read_timeseries(const TimeSeries& go_time_series, Timeseries& time_series) {
  read_label_set(go_time_series.label_set, time_series.label_set());
  read_samples(go_time_series, time_series.samples());
}

}  // namespace PromPP::Primitives::Go
