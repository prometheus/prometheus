#pragma once

#include <array>
#include <bitset>
#include <cstdint>
#include <ranges>

#define XXH_INLINE_ALL
#include "third_party/xxhash/xxhash.h"

#include "bare_bones/vector.h"

namespace PromPP {
namespace Primitives {

using Symbol = std::string;
using SymbolView = std::string_view;

// FIXME Label and LabelView should be convertable and comparable
using Label = std::pair<Symbol, Symbol>;
using LabelView = std::pair<SymbolView, SymbolView>;

using Timestamp = int64_t;

using LabelSetID = uint32_t;

template <class LabelType>
class BasicLabelSet {
  BareBones::Vector<LabelType> labels_;

 public:
  using label_type = LabelType;

  inline __attribute__((always_inline)) void clear() noexcept { labels_.clear(); }

  inline __attribute__((always_inline)) void add(const LabelType& label) noexcept {
    if (__builtin_expect(labels_.empty() || std::get<0>(label) > std::get<0>(labels_.back()), true)) {
      labels_.emplace_back(label);
    } else if (__builtin_expect(std::get<0>(label) == std::get<0>(labels_.back()), false)) {
      std::get<1>(labels_.back()) = std::get<1>(label);
    } else {
      auto i = std::lower_bound(labels_.begin(), labels_.end(), std::get<0>(label), [](const LabelType& a, const SymbolView& b) { return std::get<0>(a) < b; });
      if (__builtin_expect(std::get<0>(*i) == std::get<0>(label), false)) {
        std::get<1>(*i) = std::get<1>(label);
      } else {
        labels_.insert(i, label);
      }
    }
  }

  inline __attribute__((always_inline)) auto size() const noexcept { return labels_.size(); }

  using iterator = typename BareBones::Vector<LabelType>::iterator;
  using const_iterator = typename BareBones::Vector<LabelType>::const_iterator;

  inline __attribute__((always_inline)) const_iterator begin() const noexcept { return labels_.begin(); }
  inline __attribute__((always_inline)) iterator begin() noexcept { return labels_.begin(); }

  inline __attribute__((always_inline)) const_iterator end() const noexcept { return labels_.end(); }
  inline __attribute__((always_inline)) iterator end() noexcept { return labels_.end(); }

  template <class T>
  bool operator==(const T& o) const noexcept {
    return std::ranges::equal(begin(), end(), o.begin(), o.end());
  }

  template <class T>
  bool operator<(const T& o) const noexcept {
    return std::ranges::lexicographical_compare(begin(), end(), o.begin(), o.end());
  }

  inline __attribute__((always_inline)) friend size_t hash_value(const BasicLabelSet& label_set) noexcept {
    size_t res = 0;
    for (const auto& [label_name, label_value] : label_set) {
      res = XXH3_64bits_withSeed(label_name.data(), label_name.size(), res) ^ XXH3_64bits_withSeed(label_value.data(), label_value.size(), res);
    }
    return res;
  }

  class Names {
    const BareBones::Vector<LabelType>& labels_;

    friend class BasicLabelSet;
    inline __attribute__((always_inline)) explicit Names(const BasicLabelSet& label_set) : labels_(label_set.labels_) {}

   public:
    class Iterator {
      typename BareBones::Vector<LabelType>::const_iterator i_;

     public:
      using iterator_category = std::forward_iterator_tag;  // FIXME random_access
      using value_type = typename std::tuple_element<0, LabelType>::type;
      using difference_type = std::ptrdiff_t;

      inline __attribute__((always_inline)) explicit Iterator(typename BareBones::Vector<LabelType>::const_iterator i = nullptr) noexcept : i_(i) {}

      inline __attribute__((always_inline)) Iterator& operator++() noexcept {
        ++i_;
        return *this;
      }

      inline __attribute__((always_inline)) Iterator operator++(int) noexcept {
        Iterator retval = *this;
        ++(*this);
        return retval;
      }

      inline __attribute__((always_inline)) bool operator==(const Iterator& o) const noexcept { return i_ == o.i_; }

      inline __attribute__((always_inline)) const value_type& operator*() const noexcept { return *reinterpret_cast<const value_type*>(i_); }
    };

    inline __attribute__((always_inline)) auto begin() const noexcept { return Iterator(labels_.begin()); }

    inline __attribute__((always_inline)) auto end() const noexcept { return Iterator(labels_.end()); }

    inline __attribute__((always_inline)) auto size() const noexcept { return labels_.size(); }

    template <class T>
    bool operator==(const T& o) const noexcept {
      return std::ranges::equal(begin(), end(), o.begin(), o.end());
    }

    template <class T>
    bool operator<(const T& o) const noexcept {
      return std::ranges::lexicographical_compare(begin(), end(), o.begin(), o.end());
    }

    inline __attribute__((always_inline)) friend size_t hash_value(const Names& label_set_names) noexcept {
      size_t res = 0;
      for (const auto& label_name : label_set_names) {
        res = XXH3_64bits_withSeed(label_name.data(), label_name.size(), res);
      }
      return res;
    }
  };

  inline __attribute__((always_inline)) Names names() const noexcept { return Names(*this); }
};

using LabelSet = BasicLabelSet<Label>;
using LabelViewSet = BasicLabelSet<LabelView>;

class Sample {
 public:
  using timestamp_type = Timestamp;
  using value_type = double;

 private:
  timestamp_type timestamp_;
  value_type value_;

 public:
  timestamp_type timestamp() const noexcept { return timestamp_; }
  timestamp_type& timestamp() noexcept { return timestamp_; }

  value_type value() const noexcept { return value_; }
  value_type& value() noexcept { return value_; }

  template <size_t I>
  inline __attribute__((always_inline)) const auto& get() const noexcept {
    if constexpr (I == 0)
      return timestamp_;
    else if constexpr (I == 1)
      return value_;
    else
      static_assert(I < 2);
  }

  template <size_t I>
  inline __attribute__((always_inline)) auto& get() noexcept {
    if constexpr (I == 0)
      return timestamp_;
    else if constexpr (I == 1)
      return value_;
    else
      static_assert(I < 2);
  }

  template <class T>
  // TODO requires is_sample
  inline __attribute__((always_inline)) Sample& operator=(const T& s) noexcept {
    timestamp_ = s.timestamp();
    value_ = s.value();
    return *this;
  }

  template <class T>
  // TODO requires is_sample
  inline __attribute__((always_inline)) explicit Sample(const T& s) noexcept : timestamp_(s.timestamp()), value_(s.value()) {}

  inline __attribute__((always_inline)) Sample(timestamp_type timestamp, value_type value) noexcept : timestamp_(timestamp), value_(value) {}

  inline __attribute__((always_inline)) Sample() noexcept = default;
};

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

  BasicTimeseries(const LabelSetType& label_set, const SamplesType samples) noexcept : label_set_(label_set), samples_(samples) {}

  inline __attribute__((always_inline)) auto& label_set() noexcept {
    if constexpr (std::is_pointer<LabelSetType>::value) {
      return *label_set_;
    } else {
      return label_set_;
    }
  }

  inline __attribute__((always_inline)) const auto& label_set() const noexcept {
    if constexpr (std::is_pointer<LabelSetType>::value) {
      return *label_set_;
    } else {
      return label_set_;
    }
  }

  inline __attribute__((always_inline)) void set_label_set(LabelSetType label_set) noexcept {
    static_assert(std::is_pointer<LabelSetType>::value, "this functions can be used only if LabelSetType is a pointer");
    label_set_ = label_set;
  }

  inline __attribute__((always_inline)) const auto& samples() const noexcept {
    if constexpr (std::is_pointer<SamplesType>::value) {
      return *samples_;
    } else {
      return samples_;
    }
  }

  inline __attribute__((always_inline)) auto& samples() noexcept {
    if constexpr (std::is_pointer<SamplesType>::value) {
      return *samples_;
    } else {
      return samples_;
    }
  }

  inline __attribute__((always_inline)) void set_label_set(SamplesType samples) noexcept {
    static_assert(std::is_pointer<SamplesType>::value, "this functions can be used only if SamplesType is a pointer");
    samples_ = samples;
  }

  inline __attribute__((always_inline)) void clear() noexcept {
    label_set().clear();
    samples().clear();
  }
};

using Timeseries = BasicTimeseries<LabelSet, BareBones::Vector<Sample>>;
using TimeseriesSemiview = BasicTimeseries<LabelViewSet, BareBones::Vector<Sample>>;

}  // namespace Primitives
}  // namespace PromPP

namespace std {
template <>
struct tuple_size<PromPP::Primitives::Sample> : std::integral_constant<std::size_t, 2> {};

template <>
struct tuple_element<0, PromPP::Primitives::Sample> {
  using type = uint64_t;
};

template <>
struct tuple_element<1, PromPP::Primitives::Sample> {
  using type = double;
};
}  // namespace std
