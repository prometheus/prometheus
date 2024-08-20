#pragma once

#include <unordered_set>

#include "prometheus/types.h"
#include "querier.h"

namespace series_index::querier {

template <class Index>
class LabelValuesQuerier {
 public:
  explicit LabelValuesQuerier(const Index& index) : index_(index) {}

  template <class LabelMatchers, class ValueHandler>
  [[nodiscard]] QuerierStatus query(std::string_view label_name, const LabelMatchers& label_matchers, ValueHandler&& name_handler) const {
    auto name_id = index_.trie_index().names_trie().lookup(label_name);
    if (!name_id) {
      return QuerierStatus::kNoMatch;
    }

    if (label_matchers.empty()) {
      return query_all_label_values(*name_id, *index_.trie_index().values_trie(*name_id), std::forward<ValueHandler>(name_handler));
    }

    auto result = Querier<Index>{index_}.query(label_matchers);
    if (result.status == QuerierStatus::kMatch) {
      query_matched_unique_label_values(*name_id, *index_.trie_index().values_trie(*name_id), result.series_ids, std::forward<ValueHandler>(name_handler));
    }

    return result.status;
  }

 private:
  const Index& index_;

  template <class Trie, class ValueHandler>
  [[nodiscard]] PROMPP_ALWAYS_INLINE QuerierStatus query_all_label_values(uint32_t name_id, const Trie& trie, ValueHandler&& value_handler) const {
    enumerate_label_values(name_id, trie, std::forward<ValueHandler>(value_handler), [](uint32_t) PROMPP_LAMBDA_INLINE { return true; });
    return QuerierStatus::kMatch;
  }

  template <class Trie, class ValueHandler, class Filter>
  void enumerate_label_values(uint32_t name_id, const Trie& trie, ValueHandler&& value_handler, Filter&& filter) const {
    auto& values_table = *index_.data().symbols_tables[name_id];
    for (auto it = trie.make_enumerative_iterator(); it.is_valid(); it.next()) {
      if (filter(it.value())) {
        value_handler(values_table[it.value()]);
      }
    }
  }

  template <class Trie, class ValueHandler>
  PROMPP_ALWAYS_INLINE void query_matched_unique_label_values(uint32_t name_id,
                                                              const Trie& trie,
                                                              series_index::querier::SeriesIdSpan series_ids,
                                                              ValueHandler&& value_handler) const {
    auto value_ids = get_unique_value_ids(name_id, series_ids);
    enumerate_label_values(name_id, trie, std::forward<ValueHandler>(value_handler),
                           [&value_ids](uint32_t value_id) PROMPP_LAMBDA_INLINE { return value_ids.contains(value_id); });
  }

  [[nodiscard]] std::unordered_set<uint32_t> get_unique_value_ids(uint32_t name_id, series_index::querier::SeriesIdSpan series_ids) const {
    std::unordered_set<uint32_t> value_ids;
    value_ids.reserve(series_ids.size());

    for (auto series_id : series_ids) {
      auto series = index_[series_id];
      auto end = series.end();
      for (auto i = series.begin(); i != end; ++i) {
        if (i.name_id() == name_id) {
          value_ids.emplace(i.value_id());
        }
      }
    }

    return value_ids;
  }
};

}  // namespace series_index::querier