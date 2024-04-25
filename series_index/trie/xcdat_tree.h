#pragma once

#include <iostream>
#include <memory>
#include <string>
#include <vector>

#include "bare_bones/preprocess.h"
#include "xcdat/xcdat.hpp"

namespace series_index::trie {

class XcdatMatchesList {
 public:
  using SeriesIdList = std::vector<uint32_t>;

  explicit XcdatMatchesList(SeriesIdList& matches) : matches_(matches) {}

  PROMPP_ALWAYS_INLINE void clear() { matches_.clear(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t count() const noexcept { return matches_.size(); }

  template <class TraversalIterator>
  PROMPP_ALWAYS_INLINE void add_node(const TraversalIterator& traversal) {
    append_match(traversal.get_identifiers_of_lowest_and_highest_keys_with_common_prefix());
  }

  template <class TraversalIterator, class Condition>
  PROMPP_ALWAYS_INLINE void add_node(const TraversalIterator& traversal, Condition&& condition) {
    for (auto enumerative_iterator = traversal.make_enumerative_iterator(); enumerative_iterator.next();) {
      if (condition(enumerative_iterator.decoded_view())) {
        append_match(enumerative_iterator.id());
      }
    }
  }

  template <class TraversalIterator>
  PROMPP_ALWAYS_INLINE void add_subnodes(const TraversalIterator& traversal) {
    append_match(traversal.get_identifiers_of_lowest_and_highest_keys_with_common_prefix_excluding_prefix());
  }

  template <class TraversalIterator>
  PROMPP_ALWAYS_INLINE void add_leaf(const TraversalIterator& traversal) {
    append_match(traversal.id().value());
  }

  template <class TraversalIterator>
  PROMPP_ALWAYS_INLINE void add_leaf(const TraversalIterator& traversal, const std::string_view& prefix) {
    if (auto id = traversal.lookup(prefix); id) {
      append_match(id.value());
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const SeriesIdList& matches() const noexcept { return matches_; }

 private:
  SeriesIdList& matches_;

  PROMPP_ALWAYS_INLINE void append_match(uint64_t value) { matches_.emplace_back(value); }

  PROMPP_ALWAYS_INLINE void append_match(std::pair<uint64_t, uint64_t> range) {
    matches_.reserve(matches_.size() + range.second - range.first + 1);
    while (range.first <= range.second) {
      matches_.emplace_back(range.first++);
    }
  }
};

}  // namespace series_index::trie
