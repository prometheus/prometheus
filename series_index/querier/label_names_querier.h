#pragma once

#include <unordered_set>

#include "prometheus/types.h"
#include "querier.h"

namespace series_index::querier {

template <class Index>
class LabelNamesQuerier {
 public:
  explicit LabelNamesQuerier(const Index& index) : index_(index) {}

  template <class LabelMatchers, class NameHandler>
  [[nodiscard]] QuerierStatus query(const LabelMatchers& label_matchers, NameHandler&& name_handler) const {
    if (label_matchers.empty()) {
      return query_all_label_names(std::forward<NameHandler>(name_handler));
    }

    auto result = Querier<Index>{index_}.query(label_matchers);
    if (result.status == QuerierStatus::kMatch) {
      query_matched_unique_label_names(result.series_ids, std::forward<NameHandler>(name_handler));
    }

    return result.status;
  }

 private:
  const Index& index_;

  template <class NameHandler>
  [[nodiscard]] PROMPP_ALWAYS_INLINE QuerierStatus query_all_label_names(NameHandler&& name_handler) const {
    enumerate_label_names(std::forward<NameHandler>(name_handler), [](uint32_t) PROMPP_LAMBDA_INLINE { return true; });
    return QuerierStatus::kMatch;
  }

  template <class NameHandler, class Filter>
  void enumerate_label_names(NameHandler&& name_handler, Filter&& filter) const {
    auto& names_table = index_.data().label_name_sets_table.data().symbols_table;
    auto& names_trie = index_.trie_index().names_trie();
    for (auto it = names_trie.make_enumerative_iterator(); it.is_valid(); it.next()) {
      if (filter(it.value())) {
        name_handler(names_table[it.value()]);
      }
    }
  }

  template <class NameHandler>
  PROMPP_ALWAYS_INLINE void query_matched_unique_label_names(series_index::querier::SeriesIdSpan series_ids, NameHandler&& name_handler) const {
    auto name_ids = get_unique_name_ids(series_ids);
    enumerate_label_names(std::forward<NameHandler>(name_handler), [&name_ids](uint32_t name_id) PROMPP_LAMBDA_INLINE { return name_ids.contains(name_id); });
  }

  [[nodiscard]] std::unordered_set<uint32_t> get_unique_name_ids(series_index::querier::SeriesIdSpan series_ids) const {
    std::unordered_set<uint32_t> name_ids;
    name_ids.reserve(series_ids.size());

    for (auto series_id : series_ids) {
      auto series = index_[series_id];
      auto end = series.end();
      for (auto i = series.begin(); i != end; ++i) {
        name_ids.emplace(i.name_id());
      }
    }

    return name_ids;
  }
};

}  // namespace series_index::querier