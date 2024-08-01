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
  [[nodiscard]] PROMPP_ALWAYS_INLINE const TrieIndex& trie_index() const noexcept { return trie_index_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const SeriesReverseIndex& reverse_index() const noexcept { return reverse_index_; }
  PROMPP_ALWAYS_INLINE void build_sorting_index() {
    if (sorting_index_.empty()) {
      sorting_index_.build();
    }
  }
  PROMPP_ALWAYS_INLINE void sort_series_ids(std::span<uint32_t> series_ids) const noexcept { sorting_index_.sort(series_ids.begin(), series_ids.end()); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept {
    return trie_index_.allocated_memory() + reverse_index_.allocated_memory() + ls_id_set_allocated_memory_ + sorting_index_.allocated_memory() +
           Base::allocated_memory();
  }

  template <class LabelSet>
  PROMPP_ALWAYS_INLINE uint32_t find_or_emplace(const LabelSet& label_set) noexcept {
    auto iterator = ls_id_set_.lower_bound(label_set);
    if (iterator != ls_id_set_.end() && (*this)[*iterator] == label_set) {
      return *iterator;
    }

    auto ls_id = Base::items_.size();
    Base::items_.emplace_back(Base::data_, label_set);
    iterator = ls_id_set_.emplace_hint(iterator, ls_id);

    update_indexes(ls_id, iterator);
    return ls_id;
  }

  template <class LabelSet>
  PROMPP_ALWAYS_INLINE uint32_t find_or_emplace(const LabelSet& label_set, [[maybe_unused]] size_t hash) noexcept {
    return find_or_emplace(label_set);
  }

 protected:
  PROMPP_ALWAYS_INLINE void after_items_load(uint32_t first_loaded_id) noexcept override {
    for (auto id = first_loaded_id; id < Base::items_.size(); ++id) {
      update_indexes(id, ls_id_set_.emplace(id).first);
    }
  }

 private:
  using Base = BareBones::SnugComposite::DecodingTable<Filament>;
  using Set = phmap::btree_set<typename Base::Proxy, typename Base::LessComparator, BareBones::Allocator<typename Base::Proxy>>;

  TrieIndex trie_index_;
  SeriesReverseIndex reverse_index_;
  size_t ls_id_set_allocated_memory_{};
  Set ls_id_set_{{}, Base::less_comparator_, BareBones::Allocator<typename Base::Proxy>{ls_id_set_allocated_memory_}};
  SortingIndex<Set> sorting_index_{ls_id_set_};

  void update_indexes(uint32_t ls_id, Set::const_iterator ls_id_set_iterator) {
    auto label_set = this->operator[](ls_id);
    for (auto label = label_set.begin(); label != label_set.end(); ++label) {
      if (!is_valid_label((*label).second)) {
        continue;
      }

      reverse_index_.add(label, ls_id);
      trie_index_.names_trie().insert((*label).first, label.name_id());

      if (auto values_trie = trie_index_.values_trie(label.name_id()); values_trie) {
        values_trie->insert((*label).second, label.value_id());
      } else {
        trie_index_.insert_values_trie()->insert((*label).second, label.value_id());
      }
    }

    if (!sorting_index_.empty()) {
      sorting_index_.update(ls_id_set_iterator);
    }
  }

  PROMPP_ALWAYS_INLINE static bool is_valid_label(std::string_view value) noexcept { return !value.empty(); }
};

}  // namespace series_index