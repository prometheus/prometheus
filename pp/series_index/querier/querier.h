#pragma once

#include <ranges>

#include "bare_bones/algorithm.h"
#include "bare_bones/preprocess.h"
#include "selector_querier.h"
#include "series_index/queryable_encoding_bimap.h"
#include "set_operations.h"

namespace series_index::querier {

template <class Index, template <class> class MemoryPoolContainer = std::vector>
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

    [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_error() const noexcept { return is_querier_status_error(status); }
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

    auto result_set = resolve_matcher(selector.matchers[0], memory_pool.merge1, memory_pool.merge2);
    if (selector.matchers.size() > 1 && selector.matchers[1].is_positive()) {
      memory_pool.allocate_temp_memory(max_positive_matcher_cardinality);
    }

    for (auto it = std::next(selector.matchers.begin()); it != selector.matchers.end(); ++it) {
      process_matcher(*it, memory_pool, result_set);
    }

    result.set_series_id_list(memory_pool.release_container_for_merge(result_set.data()), result_set.size());
    return result;
  }

 private:
  class MemoryPool {
    SeriesIdContainer merge_container1_;
    SeriesIdContainer merge_container2_;
    SeriesIdContainer temp_container_;

   public:
    uint32_t* merge1{};
    uint32_t* merge2{};
    uint32_t* temp{};

    explicit MemoryPool(uint32_t items_count)
        : merge_container1_(items_count), merge_container2_(items_count), merge1(merge_container1_.data()), merge2(merge_container2_.data()) {}

    PROMPP_ALWAYS_INLINE void allocate_temp_memory(uint32_t items_count) {
      temp_container_.resize(items_count);
      temp = temp_container_.data();
    }

    PROMPP_ALWAYS_INLINE SeriesIdContainer&& release_container_for_merge(const uint32_t* memory) {
      if (memory == merge_container1_.data()) {
        return std::move(merge_container1_);
      }
      return std::move(merge_container2_);
    }
  };

  const Index& index_;
  SeriesSliceList series_slice_list_;

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
    using enum PromPP::Prometheus::MatchStatus;

    if (BareBones::is_in(matcher.status, kAllMatch, kAllMatchWithExcludes)) {
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

  PROMPP_ALWAYS_INLINE void process_matcher(const PromPP::Prometheus::Selector::Matcher& matcher, MemoryPool& memory_pool, SeriesIdSpan& result_set) {
    if (matcher.is_positive()) {
      process_positive_matcher(matcher, memory_pool, result_set);
    } else if (matcher.is_negative()) {
      process_negative_matcher(matcher, memory_pool, result_set);
    }
  }

  PROMPP_ALWAYS_INLINE void process_positive_matcher(const PromPP::Prometheus::Selector::Matcher& matcher, MemoryPool& memory_pool, SeriesIdSpan& result_set) {
    if (matcher.status == PromPP::Prometheus::MatchStatus::kAllMatch) {
      result_set = intersect_sequence(result_set, index_.reverse_index().get(matcher.label_name_id));
    } else {
      result_set = SetIntersecter::intersect(result_set, resolve_matcher(matcher, memory_pool.merge2, memory_pool.temp));
    }
  }

  PROMPP_ALWAYS_INLINE void process_negative_matcher(const PromPP::Prometheus::Selector::Matcher& matcher, MemoryPool& memory_pool, SeriesIdSpan& result_set) {
    if (matcher.status == PromPP::Prometheus::MatchStatus::kAllMatch) {
      result_set = substract_sequence(result_set, index_.reverse_index().get(matcher.label_name_id));
    } else if (matcher.status == PromPP::Prometheus::MatchStatus::kPartialMatch) {
      result_set = substract_sequences(result_set, matcher);
    } else if (matcher.status == PromPP::Prometheus::MatchStatus::kAllMatchWithExcludes) {
      result_set = SetSubstractor::substract(result_set, resolve_matcher(matcher, memory_pool.merge2, memory_pool.temp));
    }
  }

  SeriesIdSpan resolve_matcher(const PromPP::Prometheus::Selector::Matcher& matcher, uint32_t*& memory, uint32_t*& temp_memory) {
    using enum PromPP::Prometheus::MatchStatus;

    if (matcher.status == kAllMatch) {
      auto sequence = index_.reverse_index().get(matcher.label_name_id);
      decode_sequence(sequence, memory);
      return {memory, sequence->count()};
    }

    if (matcher.status == kAllMatchWithExcludes) {
      auto sequence = index_.reverse_index().get(matcher.label_name_id);
      decode_sequence(sequence, memory);
      return substract_sequences(SeriesIdSpan{memory, sequence->count()}, matcher);
    }

    fill_series_slice_list(matcher, memory);
    return SetMerger::merge(series_slice_list_, memory, temp_memory);
  }

  PROMPP_ALWAYS_INLINE void fill_series_slice_list(const PromPP::Prometheus::Selector::Matcher& matcher, uint32_t* memory) {
    series_slice_list_.clear();
    series_slice_list_.reserve(matcher.matches.size());

    uint32_t offset = 0;
    for (auto label_value_id : matcher.matches) {
      auto sequence = index_.reverse_index().get(matcher.label_name_id, label_value_id);
      decode_sequence(sequence, memory + offset);
      series_slice_list_.emplace_back(SeriesSlice{.begin = offset, .end = offset + sequence->count()});
      offset += sequence->count();
    }
  }

  PROMPP_ALWAYS_INLINE static void decode_sequence(const CompactSeriesIdSequence* sequence, uint32_t* memory) {
    if (sequence->type() == CompactSeriesIdSequence::Type::kArray) {
      std::memcpy(memory, sequence->array().data(), sequence->count() * sizeof(uint32_t));
    } else {
      std::ranges::copy(sequence->sequence(), memory);
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static SeriesIdSpan intersect_sequence(SeriesIdSpan result_set, const CompactSeriesIdSequence* sequence) {
    return sequence->process_series([&result_set](const auto& series_ids) PROMPP_LAMBDA_INLINE { return SetIntersecter::intersect(result_set, series_ids); });
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static SeriesIdSpan substract_sequence(SeriesIdSpan result_set, const CompactSeriesIdSequence* sequence) {
    return sequence->process_series([&result_set](const auto& series_ids) PROMPP_LAMBDA_INLINE { return SetSubstractor::substract(result_set, series_ids); });
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE SeriesIdSpan substract_sequences(SeriesIdSpan result_set, const PromPP::Prometheus::Selector::Matcher& matcher) {
    for (auto label_value_id : matcher.matches) {
      result_set = substract_sequence(result_set, index_.reverse_index().get(matcher.label_name_id, label_value_id));
    }

    return result_set;
  }
};

}  // namespace series_index::querier
