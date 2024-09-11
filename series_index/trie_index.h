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

  class IteratorSentinel {};

  class Iterator {
   public:
    class Data {
     public:
      [[nodiscard]] PROMPP_ALWAYS_INLINE std::string_view name() const noexcept { return names_iterator_.key(); }
      [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t name_id() const noexcept { return names_iterator_.value(); }
      [[nodiscard]] PROMPP_ALWAYS_INLINE std::string_view value() const noexcept { return values_iterator_.key(); }
      [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t value_id() const noexcept { return values_iterator_.value(); }

     private:
      const TrieIndex* index_;
      mutable Trie::EnumerativeIterator names_iterator_;
      mutable Trie::EnumerativeIterator values_iterator_;

      friend class TrieIndex;

      explicit Data(const TrieIndex* index) : index_(index), names_iterator_(index_->names_trie().make_enumerative_iterator()) {
        if (names_iterator_.is_valid()) {
          values_iterator_ = index_->values_trie(names_iterator_.value())->make_enumerative_iterator();
        }
      }

      void next_value() noexcept {
        if (values_iterator_.is_valid()) {
          if (++values_iterator_; values_iterator_.is_valid()) {
            return;
          }
        }

        if (++names_iterator_; names_iterator_.is_valid()) {
          values_iterator_ = index_->values_trie(names_iterator_.value())->make_enumerative_iterator();
        }
      }

      [[nodiscard]] PROMPP_ALWAYS_INLINE bool has_value() const noexcept { return values_iterator_.is_valid(); }
    };

    using iterator_category = std::forward_iterator_tag;
    using value_type = Data;
    using difference_type = ptrdiff_t;
    using pointer = Data*;
    using reference = Data&;

    explicit Iterator(const TrieIndex* index) : data_(index) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE const Data& operator*() const noexcept { return data_; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE const Data* operator->() const noexcept { return &data_; }

    PROMPP_ALWAYS_INLINE Iterator& operator++() noexcept {
      data_.next_value();
      return *this;
    }

    PROMPP_ALWAYS_INLINE Iterator operator++(int) noexcept {
      auto it = *this;
      ++*this;
      return it;
    }

    PROMPP_ALWAYS_INLINE bool operator==(const IteratorSentinel&) const noexcept { return !data_.has_value(); }

   private:
    Data data_;
  };

  [[nodiscard]] PROMPP_ALWAYS_INLINE Iterator begin() const noexcept { return Iterator(this); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE IteratorSentinel end() const noexcept { return {}; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE Trie& names_trie() noexcept { return names_trie_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const Trie& names_trie() const noexcept { return names_trie_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE Trie* values_trie(size_t index) noexcept { return values_trie_exists(index) ? values_trie_list_[index].get() : nullptr; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const Trie* values_trie(size_t index) const noexcept {
    return values_trie_exists(index) ? values_trie_list_[index].get() : nullptr;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool values_trie_exists(size_t index) const noexcept { return index < values_trie_list_.size(); }

  void insert(std::string_view name, uint32_t name_id, std::string_view value, uint32_t value_id) {
    names_trie_.insert(name, name_id);

    if (!values_trie_exists(name_id)) {
      values_trie_list_.resize(name_id + 1);
    }

    auto& trie = values_trie_list_[name_id];
    if (!trie) {
      trie = std::make_unique<Trie>();
    }

    trie->insert(value, value_id);
  }

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