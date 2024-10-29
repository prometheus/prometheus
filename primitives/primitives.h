#pragma once

#include <array>
#include <cstdint>
#include <ranges>
#include <string_view>
#include <vector>

#include <parallel_hashmap/phmap.h>

#include "bare_bones/preprocess.h"
#include "bare_bones/vector.h"
#include "hash.h"

namespace PromPP {
namespace Primitives {

using Symbol = std::string;
using SymbolView = std::string_view;

// FIXME Label and LabelView should be convertable
using Label = std::pair<Symbol, Symbol>;
using LabelView = std::pair<SymbolView, SymbolView>;

using Timestamp = int64_t;

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

template <class LabelType, template <class> class Container = BareBones::Vector>
class BasicLabelSet {
  Container<LabelType> labels_;

 public:
  using label_type = LabelType;

  BasicLabelSet() = default;
  BasicLabelSet(std::initializer_list<LabelType> values) {
    reserve(values.size());

    for (auto& label : values) {
      add(label);
    }
  }

  template <class LabelSet>
  explicit BasicLabelSet(const LabelSet& other) {
    labels_.reserve(other.size());
    for (const auto& label : other) {
      // if label value empty - skip label
      if (label.second.empty()) [[unlikely]] {
        continue;
      }
      labels_.emplace_back(label);
    }
  };

  BasicLabelSet(const BasicLabelSet&) = default;
  BasicLabelSet(BasicLabelSet&&) noexcept = default;

  BasicLabelSet& operator=(const BasicLabelSet&) = default;
  BasicLabelSet& operator=(BasicLabelSet&&) noexcept = default;

  inline __attribute__((always_inline)) void clear() noexcept { labels_.clear(); }

  PROMPP_ALWAYS_INLINE void add(const LabelType& label) noexcept {
    // if label value empty - skip label
    if (label.second.empty()) [[unlikely]] {
      return;
    }

    if (labels_.empty() || label.first > labels_.back().first) {
      [[likely]];
      labels_.emplace_back(label);
    } else if (label.first == labels_.back().first) {
      [[unlikely]];
      labels_.back().second = label.second;
    } else {
      auto i = std::lower_bound(labels_.begin(), labels_.end(), label.first, [](const LabelType& a, const auto& b) { return a.first < b; });
      if (i->first == label.first) {
        [[unlikely]];
        i->second = label.second;
      } else {
        labels_.insert(i, label);
      }
    }
  }

  template <class LabelSet>
  PROMPP_ALWAYS_INLINE void add(const LabelSet& label_set) {
    labels_.reserve(labels_.size() + label_set.size());

    for (auto& label : label_set) {
      add(label);
    }
  }

  template <class SymbolType>
  PROMPP_ALWAYS_INLINE const SymbolType& get(const SymbolType& label_name) noexcept {
    if (auto i = std::lower_bound(labels_.begin(), labels_.end(), label_name, [](const LabelType& a, const auto& b) { return a.first < b; });
        i->first == label_name) [[likely]] {
      return i->second;
    }

    static const SymbolType kEmptySymbol{};
    return kEmptySymbol;
  }

  inline __attribute__((always_inline)) auto size() const noexcept { return labels_.size(); }

  inline __attribute__((always_inline)) void reserve(size_t size) noexcept { labels_.reserve(size); }

  using iterator = typename Container<LabelType>::iterator;
  using const_iterator = typename Container<LabelType>::const_iterator;

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

  PROMPP_ALWAYS_INLINE friend size_t hash_value(const BasicLabelSet& label_set) noexcept { return hash::hash_of_label_set(label_set); }

  class Names {
    const Container<LabelType>& labels_;

    friend class BasicLabelSet;
    inline __attribute__((always_inline)) explicit Names(const BasicLabelSet& label_set) : labels_(label_set.labels_) {}

   public:
    class Iterator {
      typename Container<LabelType>::const_iterator i_;

     public:
      using iterator_category = std::forward_iterator_tag;  // FIXME random_access
      using value_type = typename std::tuple_element<0, LabelType>::type;
      using difference_type = std::ptrdiff_t;

      inline __attribute__((always_inline)) explicit Iterator(typename Container<LabelType>::const_iterator i = {}) noexcept : i_(i) {}

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

      inline __attribute__((always_inline)) const value_type& operator*() const noexcept { return i_->first; }
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

    PROMPP_ALWAYS_INLINE friend size_t hash_value(const Names& label_set_names) noexcept { return hash::hash_of_string_list(label_set_names); }
  };

  inline __attribute__((always_inline)) Names names() const noexcept { return Names(*this); }
};

using LabelSet = BasicLabelSet<Label, std::vector>;
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

  bool operator==(const Sample& other) const noexcept {
    return timestamp_ == other.timestamp_ && std::bit_cast<uint64_t>(value_) == std::bit_cast<uint64_t>(other.value_);
  }

  template <class T>
  // TODO requires is_sample
  PROMPP_ALWAYS_INLINE bool operator==(const T& s) const noexcept {
    return timestamp_ == s.timestamp() && std::bit_cast<uint64_t>(value_) == std::bit_cast<uint64_t>(s.value());
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

  BasicTimeseries(const LabelSetType& label_set, SamplesType samples) noexcept : label_set_(label_set), samples_(std::move(samples)) {}

  template <class LabelSet>
  BasicTimeseries(const LabelSet& label_set, SamplesType samples) noexcept : label_set_(label_set), samples_(std::move(samples)) {}

  PROMPP_ALWAYS_INLINE bool operator==(const BasicTimeseries&) const noexcept = default;

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

class LabelsBuilderStateMap {
  PromPP::Primitives::LabelViewSet building_buf_view_;
  PromPP::Primitives::LabelSet building_buf_;
  phmap::flat_hash_map<Symbol, Symbol> buffer_;

  template <class Labels>
  void sort_labels(Labels& labels) {
    std::ranges::sort(labels, [](const auto& a, const auto& b) {
      if (a.first == b.first) {
        return a.second < b.second;
      }
      return a.first < b.first;
    });
  }

 public:
  // del add label name to remove from label set.
  template <class LNameType>
  PROMPP_ALWAYS_INLINE void del(const LNameType& lname) {
    buffer_.erase(lname);
  }

  // extract we extract(move) the lebel from the builder.
  PROMPP_ALWAYS_INLINE Label extract(const std::string_view& lname) {
    if (auto it = buffer_.find(lname); it != buffer_.end()) {
      auto node = buffer_.extract(it);
      return {std::move(const_cast<Symbol&>(node.key())), std::move(node.mapped())};
    }

    return {};
  }

  // get returns the value for the label with the given name. Returns an empty string if the label doesn't exist.
  PROMPP_ALWAYS_INLINE std::string_view get(const std::string_view lname) {
    if (auto it = buffer_.find(lname); it != buffer_.end()) {
      return (*it).second;
    }

    return "";
  }

  // contains check the given name if exist.
  PROMPP_ALWAYS_INLINE bool contains(const std::string_view lname) const noexcept {
    if (auto it = buffer_.find(lname); it != buffer_.end()) {
      return true;
    }

    return false;
  }

  // set - the name/value pair as a label. A value of "" means delete that label.
  template <class LNameType, class LValueType>
  PROMPP_ALWAYS_INLINE void set(const LNameType& lname, const LValueType& lvalue) {
    if (lvalue.size() == 0) [[unlikely]] {
      del(lname);
      return;
    }

    if (auto it = buffer_.find(lname); it != buffer_.end()) {
      (*it).second = lvalue;
      return;
    }

    buffer_[lname] = lvalue;
  }

  // returns size of building labels.
  PROMPP_ALWAYS_INLINE size_t size() { return buffer_.size(); }

  // returns true if ls represents an empty set of labels.
  PROMPP_ALWAYS_INLINE bool is_empty() { return buffer_.size() == 0; }

  // label_view_set - returns the label_view set from the builder. If no modifications were made, the original labels are returned.
  PROMPP_ALWAYS_INLINE const LabelViewSet& label_view_set() {
    building_buf_view_.clear();
    for (const auto& it : buffer_) {
      if (it.second == "") [[unlikely]] {
        continue;
      }

      building_buf_view_.add(LabelView{it.first, it.second});
    }

    if (building_buf_view_.size() != 0) {
      sort_labels(building_buf_view_);
    }

    return building_buf_view_;
  }

  // label_set - returns the label set from the builder. If no modifications were made, the original labels are returned.
  PROMPP_ALWAYS_INLINE const LabelSet& label_set() {
    building_buf_.clear();

    for (const auto& it : buffer_) {
      if (it.second == "") [[unlikely]] {
        continue;
      }

      building_buf_.add(Label{it.first, it.second});
    }

    if (building_buf_.size() != 0) {
      sort_labels(building_buf_);
    }

    return building_buf_;
  }

  // range - calls f on each label in the builder.
  // TODO without copy buffer_, all changes in a another cycle.
  template <class Callback>
  PROMPP_ALWAYS_INLINE void range(Callback func) {
    // take a copy of add and del, so they are unaffected by calls to set() or del().
    phmap::flat_hash_map<Symbol, Symbol> cbuffer_ = buffer_;

    for (const auto& it : cbuffer_) {
      if (it.second == "") [[unlikely]] {
        continue;
      }

      if (bool ok = func(it.first, it.second); !ok) {
        return;
      }
    }
  }

  // reset - clears all current state for the builder.
  template <class SomeLabelSet>
  PROMPP_ALWAYS_INLINE void reset(SomeLabelSet* base) {
    building_buf_view_.clear();
    building_buf_.clear();
    buffer_.clear();

    if (base == nullptr) [[unlikely]] {
      return;
    }

    for (const auto& [lname, lvalue] : *base) {
      if (lvalue == "") {
        continue;
      }

      buffer_[lname] = lvalue;
    }
  }
};

// LabelsBuilder - builder for label set.
template <class BuilderState>
class LabelsBuilder {
  BuilderState& state_;

 public:
  PROMPP_ALWAYS_INLINE explicit LabelsBuilder(BuilderState& state) : state_(state) {}

  template <class SomeLabelSet>
  PROMPP_ALWAYS_INLINE explicit LabelsBuilder(BuilderState& state, SomeLabelSet* ls) : state_(state) {
    reset(ls);
  }

  // del - add label name to remove from label set.
  template <class LNameType>
  PROMPP_ALWAYS_INLINE void del(const LNameType& lname) {
    state_.del(lname);
  }

  // extract we extract(move) the lebel from the builder.
  PROMPP_ALWAYS_INLINE Label extract(const std::string_view& lname) { return state_.extract(lname); }

  // get - returns the value for the label with the given name. Returns an empty string if the label doesn't exist.
  PROMPP_ALWAYS_INLINE std::string_view get(const std::string_view lname) { return state_.get(lname); }

  // contains check the given name if exist.
  PROMPP_ALWAYS_INLINE bool contains(const std::string_view lname) const noexcept { return state_.contains(lname); }

  // returns size of building labels.
  PROMPP_ALWAYS_INLINE size_t size() { return state_.size(); }

  // returns true if ls represents an empty set of labels.
  PROMPP_ALWAYS_INLINE bool is_empty() { return state_.is_empty(); }

  // label_view_set - returns the label_view set from the builder. If no modifications were made, the original labels are returned.
  PROMPP_ALWAYS_INLINE const PromPP::Primitives::LabelViewSet& label_view_set() { return state_.label_view_set(); }

  // label_set - returns the label set from the builder. If no modifications were made, the original labels are returned.
  PROMPP_ALWAYS_INLINE const PromPP::Primitives::LabelSet& label_set() { return state_.label_set(); }

  // range - calls f on each label in the builder.
  template <class Callback>
  PROMPP_ALWAYS_INLINE void range(Callback func) {
    state_.range(func);
  }

  // reset - clears all current state for the builder.
  template <class SomeLabelSet>
  PROMPP_ALWAYS_INLINE void reset() {
    state_.reset(static_cast<SomeLabelSet*>(nullptr));
  }

  // reset - clears all current state for the builder and init from LabelSet.
  template <class SomeLabelSet>
  PROMPP_ALWAYS_INLINE void reset(SomeLabelSet* ls) {
    state_.reset(ls);
  }

  // set - the name/value pair as a label. A value of "" means delete that label.
  template <class LNameType, class LValueType>
  PROMPP_ALWAYS_INLINE void set(const LNameType& lname, const LValueType& lvalue) {
    state_.set(lname, lvalue);
  }

  PROMPP_ALWAYS_INLINE LabelsBuilder(LabelsBuilder&&) noexcept = default;
  PROMPP_ALWAYS_INLINE ~LabelsBuilder() = default;
};

}  // namespace Primitives
}  // namespace PromPP

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

// namespace BareBones {
// template <>
// struct IsTriviallyReallocatable<PromPP::Primitives::String> : std::true_type {};
// }  // namespace BareBones
