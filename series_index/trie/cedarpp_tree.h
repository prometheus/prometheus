#pragma once

#include <iostream>
#include <memory>
#include <optional>
#include <string>
#include <vector>

#include "bare_bones/preprocess.h"
#include "cedar/cedarpp.h"

namespace series_index::trie {

struct Utf8CharTraits {
  static constexpr uint8_t kUnusedUtf8Char = 0xC0;

  static cedar::uchar replace(cedar::uchar c) noexcept { return c == '\0' ? kUnusedUtf8Char : c; }
  static cedar::uchar restore(cedar::uchar c) noexcept { return c == kUnusedUtf8Char ? '\0' : c; }
};

using Trie = cedar::da<uint32_t, Utf8CharTraits>;

class CedarEnumerativeIterator {
 public:
  CedarEnumerativeIterator() = default;

  CedarEnumerativeIterator(Trie* trie, size_t from, size_t length, bool next)
      : trie_(trie), from_(from), root_(from_), root_length_(length), length_(length), value_(trie_->begin(from_, length_)) {
    if (next) {
      value_ = trie->next(from_, length_, from_);
    }
  }
  CedarEnumerativeIterator(Trie* trie, size_t from, size_t length, const std::string_view& prefix)
      : trie_(trie), from_(from), root_(from_), length_(length), value_(trie_->traverse(prefix.data(), from_, length_, prefix.length())) {
    root_ = from_;
    root_length_ = length_;
    if (is_valid()) {
      value_ = trie->begin(from_, length_);
    }
  }

  PROMPP_ALWAYS_INLINE bool next() noexcept {
    next_value();
    return is_valid();
  }

  PROMPP_ALWAYS_INLINE CedarEnumerativeIterator& operator++() noexcept {
    next_value();
    return *this;
  }
  PROMPP_ALWAYS_INLINE uint32_t operator*() const noexcept { return value_; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_valid() const noexcept { return value_ != static_cast<uint32_t>(Trie::error_code::CEDAR_NO_PATH); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t value() const noexcept { return value_; }

  [[nodiscard]] std::string_view tail_from_root() { return trie_->restore_key(from_, length_ - root_length_, restored_key_); }
  [[nodiscard]] std::string_view key() { return trie_->restore_key(from_, length_, restored_key_); }

 private:
  std::string restored_key_;
  Trie* trie_{};
  size_t from_{};
  size_t root_{};
  size_t root_length_{};
  size_t length_{};
  uint32_t value_{static_cast<uint32_t>(Trie::error_code::CEDAR_NO_PATH)};

  PROMPP_ALWAYS_INLINE void next_value() noexcept { value_ = trie_->next(from_, length_, root_); }
};

class CedarTraversal {
 public:
  explicit CedarTraversal(Trie* trie) : trie_(trie) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE CedarEnumerativeIterator make_enumerative_iterator() const noexcept { return {trie_, from_, length_, false}; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE CedarEnumerativeIterator make_enumerative_iterator_to_subnodes() const noexcept {
    return {trie_, from_, length_, value_ != static_cast<uint32_t>(Trie::error_code::CEDAR_NO_VALUE)};
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE CedarEnumerativeIterator make_enumerative_iterator(const std::string_view& prefix) const noexcept {
    return {trie_, from_, 0, prefix};
  }

  [[nodiscard]] bool traverse(const std::string_view& prefix) {
    size_t length = 0;
    if (value_ = trie_->traverse(prefix.data(), from_, length, prefix.length()); value_ == static_cast<uint32_t>(Trie::error_code::CEDAR_NO_PATH)) {
      return false;
    }
    length_ += length;

    return true;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_leaf() noexcept {
    if (value_ == static_cast<uint32_t>(Trie::error_code::CEDAR_NO_VALUE)) {
      return false;
    }

    size_t length = length_;
    size_t from = from_;
    auto value = trie_->begin(from, length);
    if (value == Trie::error_code::CEDAR_NO_PATH) {
      [[unlikely]];
      return true;
    }

    return trie_->next(from, length, from) == Trie::error_code::CEDAR_NO_PATH;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t lookup(std::string_view key) const noexcept {
    return trie_->exactMatchSearch<uint32_t>(key.data(), key.length(), from_);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static std::string_view tail() noexcept { return {}; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t value() const noexcept { return value_; }

 private:
  Trie* trie_;
  size_t from_{};
  size_t length_{};
  uint32_t value_{static_cast<uint32_t>(Trie::error_code::CEDAR_NO_PATH)};
};

class CedarTrie {
 public:
  using Traversal = CedarTraversal;
  using EnumerativeIterator = CedarEnumerativeIterator;
  using Value = uint32_t;

  PROMPP_ALWAYS_INLINE void insert(std::string_view key, uint32_t value) noexcept { trie_.update(key.data(), key.length(), 0) = value; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE std::optional<uint32_t> lookup(std::string_view key) const noexcept {
    if (auto value = trie_.exactMatchSearch<uint32_t>(key.data(), key.length(), 0); value != static_cast<uint32_t>(Trie::error_code::CEDAR_NO_VALUE)) {
      return value;
    }

    return std::nullopt;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE CedarEnumerativeIterator make_enumerative_iterator() const noexcept { return {const_cast<Trie*>(&trie_), 0, 0, false}; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE CedarTraversal make_traversal() const { return CedarTraversal{const_cast<Trie*>(&trie_)}; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return trie_.allocated_memory(); }

 private:
  Trie trie_;
};

class CedarMatchesList {
 public:
  using SeriesIdList = std::vector<uint32_t>;

  explicit CedarMatchesList(SeriesIdList& matches) : matches_(matches) {}

  PROMPP_ALWAYS_INLINE void clear() { matches_.clear(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t count() const noexcept { return matches_.size(); }

  PROMPP_ALWAYS_INLINE void add_node(const CedarTraversal& traversal) {
    for (auto iterator = traversal.make_enumerative_iterator(); iterator.is_valid(); ++iterator) {
      matches_.push_back(*iterator);
    }
  }

  template <class Condition>
  PROMPP_ALWAYS_INLINE void add_node(const CedarTraversal& traversal, Condition&& condition) {
    for (auto iterator = traversal.make_enumerative_iterator(); iterator.is_valid(); ++iterator) {
      if (condition(iterator.tail_from_root())) {
        matches_.push_back(*iterator);
      }
    }
  }

  PROMPP_ALWAYS_INLINE void add_subnodes(const CedarTraversal& traversal) {
    for (auto iterator = traversal.make_enumerative_iterator_to_subnodes(); iterator.is_valid(); ++iterator) {
      matches_.push_back(*iterator);
    }
  }

  PROMPP_ALWAYS_INLINE void add_leaf(const CedarTraversal& traversal) { matches_.push_back(traversal.value()); }
  PROMPP_ALWAYS_INLINE void add_leaf(const CedarTraversal& traversal, const std::string_view& prefix) {
    if (auto value = traversal.lookup(prefix); value != static_cast<uint32_t>(cedar::da<uint32_t>::error_code::CEDAR_NO_VALUE)) {
      matches_.push_back(value);
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const SeriesIdList& matches() const noexcept { return matches_; }

 private:
  SeriesIdList& matches_;
};

}  // namespace series_index::trie
