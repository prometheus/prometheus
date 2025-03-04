#pragma once

#include <ranges>

#include <parallel_hashmap/phmap.h>

#include "bare_bones/preprocess.h"
#include "label_set.h"
#include "primitives.h"

namespace PromPP::Primitives {
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
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool contains(const std::string_view lname) const noexcept {
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
  PROMPP_ALWAYS_INLINE void reset() {
    building_buf_view_.clear();
    building_buf_.clear();
    buffer_.clear();
  }

  // reset - clears all current state for the builder.
  template <class SomeLabelSet>
  PROMPP_ALWAYS_INLINE void reset(SomeLabelSet& base) {
    building_buf_view_.clear();
    building_buf_.clear();
    buffer_.clear();

    for (const auto& [lname, lvalue] : base) {
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
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool contains(const std::string_view lname) const noexcept { return state_.contains(lname); }

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
  PROMPP_ALWAYS_INLINE void reset() { state_.reset(); }

  // reset - clears all current state for the builder and init from LabelSet.
  template <class SomeLabelSet>
  PROMPP_ALWAYS_INLINE void reset(const SomeLabelSet& ls) {
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
}  // namespace PromPP::Primitives