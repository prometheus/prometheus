#pragma once

#include <cstddef>
#include <memory>
#include <numeric>
#include <vector>

#include "bare_bones/preprocess.h"

namespace series_index {

template <class TrieType, class TrieRegexpMatchesList>
class TrieIndex {
 public:
  using Trie = TrieType;
  using RegexpMatchesList = TrieRegexpMatchesList;

  [[nodiscard]] PROMPP_ALWAYS_INLINE Trie& names_trie() noexcept { return names_trie_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const Trie& names_trie() const noexcept { return names_trie_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE Trie* values_trie(size_t index) noexcept { return values_trie_exists(index) ? values_trie_list_[index].get() : nullptr; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const Trie* values_trie(size_t index) const noexcept {
    return values_trie_exists(index) ? values_trie_list_[index].get() : nullptr;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool values_trie_exists(size_t index) const noexcept { return index < values_trie_list_.size(); }

  PROMPP_ALWAYS_INLINE Trie* insert_values_trie() { return values_trie_list_.emplace_back(std::make_unique<Trie>()).get(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept {
    return names_trie_.allocated_memory() + values_trie_list_.capacity() * sizeof(values_trie_list_[0]) +
           std::accumulate(values_trie_list_.begin(), values_trie_list_.end(), 0ULL,
                           [](size_t sum, const auto& value) { return sum + value->allocated_memory(); });
  }

 private:
  Trie names_trie_;
  std::vector<std::unique_ptr<Trie>> values_trie_list_;
};

}  // namespace series_index