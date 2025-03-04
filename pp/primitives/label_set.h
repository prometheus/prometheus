#pragma once

#include "bare_bones/preprocess.h"
#include "bare_bones/vector.h"
#include "hash.h"
#include "primitives.h"

namespace PromPP::Primitives {
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

  PROMPP_ALWAYS_INLINE void clear() noexcept { labels_.clear(); }

  PROMPP_ALWAYS_INLINE void add(const LabelType& label) noexcept {
    // if label value empty - skip label
    if (label.second.empty()) [[unlikely]] {
      return;
    }

    if (labels_.empty() || label.first > labels_.back().first) [[likely]] {
      labels_.emplace_back(label);
    } else if (label.first == labels_.back().first) [[unlikely]] {
      labels_.back().second = label.second;
    } else {
      auto i = std::lower_bound(labels_.begin(), labels_.end(), label.first, [](const LabelType& a, const auto& b) { return a.first < b; });
      if (i->first == label.first) [[unlikely]] {
        i->second = label.second;
      } else {
        labels_.insert(i, label);
      }
    }
  }

  PROMPP_ALWAYS_INLINE void append(std::string_view name, std::string_view value) noexcept {
    if (!value.empty()) [[likely]] {
      labels_.emplace_back(name, value);
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

  [[nodiscard]] PROMPP_ALWAYS_INLINE auto size() const noexcept { return labels_.size(); }

  PROMPP_ALWAYS_INLINE void reserve(size_t size) noexcept { labels_.reserve(size); }

  using iterator = typename Container<LabelType>::iterator;
  using const_iterator = typename Container<LabelType>::const_iterator;

  [[nodiscard]] PROMPP_ALWAYS_INLINE const_iterator begin() const noexcept { return labels_.begin(); }
  PROMPP_ALWAYS_INLINE iterator begin() noexcept { return labels_.begin(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const_iterator end() const noexcept { return labels_.end(); }
  PROMPP_ALWAYS_INLINE iterator end() noexcept { return labels_.end(); }

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

      PROMPP_ALWAYS_INLINE explicit Iterator(typename Container<LabelType>::const_iterator i = {}) noexcept : i_(i) {}

      PROMPP_ALWAYS_INLINE Iterator& operator++() noexcept {
        ++i_;
        return *this;
      }

      PROMPP_ALWAYS_INLINE Iterator operator++(int) noexcept {
        Iterator retval = *this;
        ++(*this);
        return retval;
      }

      PROMPP_ALWAYS_INLINE bool operator==(const Iterator& o) const noexcept { return i_ == o.i_; }

      PROMPP_ALWAYS_INLINE const value_type& operator*() const noexcept { return i_->first; }
    };

    PROMPP_ALWAYS_INLINE auto begin() const noexcept { return Iterator(labels_.begin()); }

    PROMPP_ALWAYS_INLINE auto end() const noexcept { return Iterator(labels_.end()); }

    PROMPP_ALWAYS_INLINE auto size() const noexcept { return labels_.size(); }

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

  [[nodiscard]] PROMPP_ALWAYS_INLINE Names names() const noexcept { return Names(*this); }
};

using LabelSet = BasicLabelSet<Label, std::vector>;
using LabelViewSet = BasicLabelSet<LabelView>;
}  // namespace PromPP::Primitives