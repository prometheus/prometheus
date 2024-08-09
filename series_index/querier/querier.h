#pragma once

#include <ranges>

#include "bare_bones/algorithm.h"
#include "bare_bones/preprocess.h"
#include "selector_querier.h"
#include "series_index/queryable_encoding_bimap.h"
#include "set_operations.h"

namespace series_index::querier {

template <class Item>
using StdVector = std::vector<Item>;

template <class Index, template <class> class MemoryPoolContainer = StdVector>
class Querier {
 public:
  class MatchersComparatorByTypeAndCardinality {
   public:
    PROMPP_ALWAYS_INLINE bool operator()(const PromPP::Prometheus::Selector::Matcher& a, const PromPP::Prometheus::Selector::Matcher& b) const noexcept {
      if (a.is_positive()) {
        if (b.is_positive()) {
          return a.cardinality < b.cardinality;
        }

        return true;
      }

      return false;
    }
  };

  using SeriesIdContainer = MemoryPoolContainer<uint32_t>;

  struct QuerierResult {
    SeriesIdContainer series_ids{};
    QuerierStatus status{QuerierStatus::kNoMatch};

    PROMPP_ALWAYS_INLINE void set_series_id_list(SeriesIdContainer&& ids, uint32_t size) noexcept {
      series_ids = std::move(ids);
      series_ids.resize(size);
      status = series_ids.empty() ? QuerierStatus::kNoMatch : QuerierStatus::kMatch;
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_error() const noexcept { return status != QuerierStatus::kMatch && status != QuerierStatus::kNoMatch; }
  };

  explicit Querier(const Index& index) : index_(index) {}

  template <class LabelMatchers>
  [[nodiscard]] QuerierResult query(const LabelMatchers& label_matchers) {
    QuerierResult result;
    PromPP::Prometheus::Selector selector;
    if (result.status = SelectorQuerier{index_.trie_index()}.query(label_matchers, selector); result.status != QuerierStatus::kMatch) {
      return result;
    }

    sort_matchers_by_type_and_cardinality(selector);
    auto max_positive_matcher_cardinality = get_max_positive_matcher_cardinality(selector);
    MemoryPool memory_pool(max_positive_matcher_cardinality);

    SeriesSliceList series_slice_list;
    auto result_set = process_first_matcher(selector.matchers[0], series_slice_list, memory_pool);
    if (selector.matchers.size() > 1 && selector.matchers[1].is_positive()) {
      memory_pool.allocate_temp_memory(max_positive_matcher_cardinality);
    }

    for (auto it = selector.matchers.begin() + 1; it != selector.matchers.end(); ++it) {
      process_matcher(*it, series_slice_list, memory_pool, result_set);
    }

    result.set_series_id_list(memory_pool.release_container_for_merge(result_set.data()), result_set.size());
    return result;
  }

 private:
  class MemoryPool {
   private:
    SeriesIdContainer merge1_;
    SeriesIdContainer merge2_;
    SeriesIdContainer temp_;

   public:
    uint32_t* merge1{};
    uint32_t* merge2{};
    uint32_t* temp{};

    explicit MemoryPool(uint32_t items_count) : merge1_(items_count), merge2_(items_count), merge1(merge1_.data()), merge2(merge2_.data()) {}

    PROMPP_ALWAYS_INLINE void allocate_temp_memory(uint32_t items_count) {
      temp_ = SeriesIdContainer{items_count};
      temp = temp_.data();
    }

    PROMPP_ALWAYS_INLINE SeriesIdContainer&& release_container_for_merge(const uint32_t* memory) {
      if (memory == merge1_.data()) {
        return std::move(merge1_);
      } else {
        return std::move(merge2_);
      }
    }
  };

  const Index& index_;

  void sort_matchers_by_type_and_cardinality(PromPP::Prometheus::Selector& selector) const noexcept {
    fill_positive_matchers_cardinality(selector);
    std::sort(selector.matchers.begin(), selector.matchers.end(), MatchersComparatorByTypeAndCardinality{});
  }

  void fill_positive_matchers_cardinality(PromPP::Prometheus::Selector& selector) const noexcept {
    for (auto& matcher : selector.matchers) {
      if (matcher.is_positive()) {
        matcher.cardinality = get_cardinality(matcher);
      }
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t get_cardinality(const PromPP::Prometheus::Selector::Matcher& matcher) const noexcept {
    if (matcher.status == PromPP::Prometheus::MatchStatus::kAllMatch) {
      return index_.reverse_index().get(matcher.label_name_id)->count();
    }

    return BareBones::accumulate(matcher.matches, 0U, [this, &matcher](uint32_t cardinality, uint32_t label_value_id) PROMPP_LAMBDA_INLINE {
      return cardinality + index_.reverse_index().get(matcher.label_name_id, label_value_id)->count();
    });
  }

  [[nodiscard]] static PROMPP_ALWAYS_INLINE uint32_t get_max_positive_matcher_cardinality(const PromPP::Prometheus::Selector& selector) noexcept {
    for (const auto& matcher : std::ranges::reverse_view(selector.matchers)) {
      if (matcher.is_positive()) {
        return matcher.cardinality;
      }
    }

    assert(false);
    return 0U;
  }

  PROMPP_ALWAYS_INLINE SeriesIdSpan process_first_matcher(const PromPP::Prometheus::Selector::Matcher& matcher,
                                                          SeriesSliceList& series_slice_list,
                                                          MemoryPool& memory_pool) {
    resolve_matcher(matcher, series_slice_list, memory_pool.merge1);
    return SetMerger::merge(series_slice_list, memory_pool.merge1, memory_pool.merge2);
  }

  PROMPP_ALWAYS_INLINE void process_matcher(const PromPP::Prometheus::Selector::Matcher& matcher,
                                            SeriesSliceList& series_slice_list,
                                            MemoryPool& memory_pool,
                                            SeriesIdSpan& result_set) {
    if (matcher.is_positive()) {
      resolve_matcher(matcher, series_slice_list, memory_pool.merge2);
      result_set = SetIntersecter::intersect(result_set, SetMerger::merge(series_slice_list, memory_pool.merge2, memory_pool.temp));
    } else if (matcher.is_negative()) {
      if (matcher.status == PromPP::Prometheus::MatchStatus::kAllMatch) {
        result_set = substract_sequence(result_set, index_.reverse_index().get(matcher.label_name_id));
      } else if (matcher.status == PromPP::Prometheus::MatchStatus::kPartialMatch) {
        for (auto label_value_id : matcher.matches) {
          result_set = substract_sequence(result_set, index_.reverse_index().get(matcher.label_name_id, label_value_id));
        }
      }
    }
  }

  void resolve_matcher(const PromPP::Prometheus::Selector::Matcher& matcher, SeriesSliceList& series_slice_list, uint32_t* memory) const {
    series_slice_list.clear();

    if (matcher.status == PromPP::Prometheus::MatchStatus::kAllMatch) {
      auto sequence = index_.reverse_index().get(matcher.label_name_id);
      decode_sequence(sequence, memory);
      series_slice_list.emplace_back(SeriesSlice{.begin = 0, .end = sequence->count()});
    } else {
      series_slice_list.reserve(matcher.matches.size());

      uint32_t offset = 0;
      for (auto label_value_id : matcher.matches) {
        auto sequence = index_.reverse_index().get(matcher.label_name_id, label_value_id);
        decode_sequence(sequence, memory + offset);
        series_slice_list.emplace_back(SeriesSlice{.begin = offset, .end = offset + sequence->count()});
        offset += sequence->count();
      }
    }
  }

  PROMPP_ALWAYS_INLINE static void decode_sequence(const CompactSeriesIdSequence* sequence, uint32_t* memory) {
    if (sequence->type() == CompactSeriesIdSequence::Type::kArray) {
      std::memcpy(memory, sequence->array().data(), sequence->count() * sizeof(uint32_t));
    } else {
      std::ranges::copy(sequence->sequence(), memory);
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static SeriesIdSpan substract_sequence(SeriesIdSpan result_set, const CompactSeriesIdSequence* sequence) {
    return sequence->process_series([&result_set](const auto& series_ids) PROMPP_LAMBDA_INLINE { return SetSubstractor::substract(result_set, series_ids); });
  }
};

}  // namespace series_index::querier