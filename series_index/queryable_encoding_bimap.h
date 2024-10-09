#pragma once

#include "bare_bones/allocator.h"
#include "bare_bones/preprocess.h"
#include "bare_bones/snug_composite.h"
#include "reverse_index.h"
#include "sorting_index.h"
#include "trie_index.h"

namespace series_index {

template <class Filament, class TrieIndex>
class QueryableEncodingBimap : public BareBones::SnugComposite::DecodingTable<Filament> {
 public:
  using Base = BareBones::SnugComposite::DecodingTable<Filament>;
  using Set = phmap::btree_set<typename Base::Proxy, typename Base::LessComparator, BareBones::Allocator<typename Base::Proxy>>;
  using HashSet =
      phmap::flat_hash_set<typename Base::Proxy, typename Base::Hasher, typename Base::EqualityComparator, BareBones::Allocator<typename Base::Proxy>>;
  using LsIdSetIterator = typename Set::const_iterator;
  using TrieIndexIterator = typename TrieIndex::Iterator;

  [[nodiscard]] PROMPP_ALWAYS_INLINE const TrieIndex& trie_index() const noexcept { return trie_index_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const SeriesReverseIndex& reverse_index() const noexcept { return reverse_index_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const Set& ls_id_set() const noexcept { return ls_id_set_; }

  PROMPP_ALWAYS_INLINE void build_sorting_index() {
    if (sorting_index_.empty()) {
      sorting_index_.build();
    }
  }

  template <class Iterator>
  PROMPP_ALWAYS_INLINE void sort_series_ids(Iterator begin, Iterator end) const noexcept {
    sorting_index_.sort(begin, end);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept {
    return trie_index_.allocated_memory() + reverse_index_.allocated_memory() + ls_id_set_allocated_memory_ + ls_id_hash_set_allocated_memory_ +
           sorting_index_.allocated_memory() + Base::allocated_memory();
  }

  template <class LabelSet>
  PROMPP_ALWAYS_INLINE uint32_t find_or_emplace(const LabelSet& label_set) noexcept {
    return find_or_emplace(label_set, Base::hasher_(label_set));
  }

  template <class LabelSet>
  PROMPP_ALWAYS_INLINE uint32_t find_or_emplace(const LabelSet& label_set, size_t hash) noexcept {
    hash = phmap_hash(hash);
    if (auto it = ls_id_hash_set_.find(label_set, hash); it != ls_id_hash_set_.end()) {
      return *it;
    }

    auto ls_id = Base::items_.size();
    auto composite_label_set = Base::items_.emplace_back(Base::data_, label_set).composite(Base::data());
    update_indexes(ls_id, composite_label_set, hash);
    return ls_id;
  }

 private:
  using LabelSet = typename Base::value_type;

  TrieIndex trie_index_;
  SeriesReverseIndex reverse_index_;

  size_t ls_id_set_allocated_memory_{};
  Set ls_id_set_{{}, Base::less_comparator_, BareBones::Allocator<typename Base::Proxy>{ls_id_set_allocated_memory_}};

  size_t ls_id_hash_set_allocated_memory_{};
  HashSet ls_id_hash_set_{0, Base::hasher_, Base::equality_comparator_, BareBones::Allocator<typename Base::Proxy>{ls_id_hash_set_allocated_memory_}};

  SortingIndex<Set> sorting_index_{ls_id_set_};

  PROMPP_ALWAYS_INLINE void after_items_load(uint32_t first_loaded_id) noexcept override {
    for (auto ls_id = first_loaded_id; ls_id < Base::items_.size(); ++ls_id) {
      auto label_set = this->operator[](ls_id);
      update_indexes(ls_id, label_set, phmap_hash(Base::hasher_(label_set)));
    }
  }

  void update_indexes(uint32_t ls_id, const LabelSet& label_set, size_t label_set_phmap_hash) {
    ls_id_hash_set_.emplace_with_hash(label_set_phmap_hash, typename Base::Proxy(ls_id));
    auto ls_id_set_iterator = ls_id_set_.emplace(ls_id).first;

    for (auto label = label_set.begin(); label != label_set.end(); ++label) {
      if (!is_valid_label((*label).second)) {
        continue;
      }

      reverse_index_.add(label, ls_id);
      trie_index_.insert((*label).first, label.name_id(), (*label).second, label.value_id());
    }

    if (!sorting_index_.empty()) {
      sorting_index_.update(ls_id_set_iterator);
    }
  }

  PROMPP_ALWAYS_INLINE static bool is_valid_label(std::string_view value) noexcept { return !value.empty(); }

  PROMPP_ALWAYS_INLINE static size_t phmap_hash(size_t hash) noexcept { return phmap::phmap_mix<sizeof(size_t)>()(hash); }
};

}  // namespace series_index