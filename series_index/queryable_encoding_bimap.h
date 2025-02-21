#pragma once

#include "bare_bones/allocator.h"
#include "bare_bones/preprocess.h"
#include "bare_bones/snug_composite.h"
#include "queried_series.h"
#include "reverse_index.h"
#include "sorting_index.h"
#include "trie_index.h"

namespace series_index {

template <template <template <class> class> class Filament, template <class> class Vector, class TrieIndex>
class QueryableEncodingBimap final : public BareBones::SnugComposite::DecodingTable<Filament, Vector> {
 public:
  using Base = BareBones::SnugComposite::DecodingTable<Filament, Vector>;
  using LsIdSet = phmap::btree_set<typename Base::Proxy, typename Base::LessComparator, BareBones::Allocator<typename Base::Proxy>>;
  using HashSet =
      phmap::flat_hash_set<typename Base::Proxy, typename Base::Hasher, typename Base::EqualityComparator, BareBones::Allocator<typename Base::Proxy>>;
  using LsIdSetIterator = typename LsIdSet::const_iterator;
  using TrieIndexIterator = typename TrieIndex::Iterator;

  [[nodiscard]] PROMPP_ALWAYS_INLINE const TrieIndex& trie_index() const noexcept { return trie_index_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const SeriesReverseIndex& reverse_index() const noexcept { return reverse_index_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const LsIdSet& ls_id_set() const noexcept { return ls_id_set_; }

  template <class Iterator>
  PROMPP_ALWAYS_INLINE void sort_series_ids(Iterator begin, Iterator end) noexcept {
    if (sorting_index_.empty()) [[unlikely]] {
      sorting_index_.build();
    }

    sorting_index_.sort(begin, end);
  }

  template <class Container>
  PROMPP_ALWAYS_INLINE void sort_series_ids(Container& container) noexcept {
    sort_series_ids(container.begin(), container.end());
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept {
    return trie_index_.allocated_memory() + reverse_index_.allocated_memory() + ls_id_set_allocated_memory_ + ls_id_hash_set_allocated_memory_ +
           sorting_index_.allocated_memory() + queried_series_.allocated_memory() + Base::allocated_memory();
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
    queried_series_.set_series_count(Base::items_.size());
    return ls_id;
  }

  template <class Class>
  PROMPP_ALWAYS_INLINE std::optional<uint32_t> find(const Class& c) const noexcept {
    if (auto i = ls_id_hash_set_.find(c); i != ls_id_hash_set_.end()) {
      return *i;
    }
    return {};
  }

  template <class Class>
  PROMPP_ALWAYS_INLINE std::optional<uint32_t> find(const Class& c, size_t hashval) const noexcept {
    if (auto i = ls_id_hash_set_.find(c, phmap_hash(hashval)); i != ls_id_hash_set_.end()) {
      return *i;
    }
    return {};
  }

  template <class SeriesIdContainer>
  PROMPP_ALWAYS_INLINE void set_queried_series(QueriedSeries::Source source, const SeriesIdContainer& ids) noexcept {
    queried_series_.set(source, ids);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t queried_series_count(QueriedSeries::Source source) const noexcept { return queried_series_.count(source); }

 private:
  using LabelSet = typename Base::value_type;

  TrieIndex trie_index_;
  SeriesReverseIndex reverse_index_;

  size_t ls_id_set_allocated_memory_{};
  LsIdSet ls_id_set_{{}, Base::less_comparator_, BareBones::Allocator<typename Base::Proxy>{ls_id_set_allocated_memory_}};

  size_t ls_id_hash_set_allocated_memory_{};
  HashSet ls_id_hash_set_{0, Base::hasher_, Base::equality_comparator_, BareBones::Allocator<typename Base::Proxy>{ls_id_hash_set_allocated_memory_}};

  SortingIndex<LsIdSet> sorting_index_{ls_id_set_};

  QueriedSeries queried_series_;

  PROMPP_ALWAYS_INLINE void after_items_load(uint32_t first_loaded_id) noexcept override {
    ls_id_hash_set_.reserve(Base::items_.size());
    queried_series_.set_series_count(Base::items_.size());

    for (auto ls_id = first_loaded_id; ls_id < Base::items_.size(); ++ls_id) {
      auto label_set = this->operator[](ls_id);
      update_indexes(ls_id, label_set, phmap_hash(Base::hasher_(label_set)));
    }
  }

  void update_indexes(uint32_t ls_id, const LabelSet& label_set, size_t label_set_phmap_hash) {
    ls_id_hash_set_.emplace_with_hash(label_set_phmap_hash, typename Base::Proxy(ls_id));
    auto ls_id_set_iterator = ls_id_set_.emplace(ls_id).first;

    for (auto label = label_set.begin(); label != label_set.end(); ++label) {
      if (!is_valid_label((*label).second)) [[unlikely]] {
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
