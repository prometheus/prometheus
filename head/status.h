#pragma once

#include "primitives/primitives.h"
#include "prometheus/value.h"
#include "series_data/decoder.h"
#include "series_index/reverse_index.h"

namespace head {

template <class StringType>
struct StringCountItem {
  StringType name{};
  uint32_t count{};

  bool operator==(const StringCountItem&) const noexcept = default;
};

template <class StringType>
struct StringPairCountItem {
  StringType name{};
  StringType value{};
  uint32_t count{};

  bool operator==(const StringPairCountItem&) const noexcept = default;
};

template <class StringType, template <class> class Container>
struct Status {
  using String = StringType;

  PromPP::Primitives::TimeInterval min_max_timestamp{};
  Container<StringCountItem<StringType>> label_value_count_by_label_name{};
  Container<StringCountItem<StringType>> series_count_by_metric_name{};
  Container<StringCountItem<StringType>> memory_in_bytes_by_label_name{};
  Container<StringPairCountItem<StringType>> series_count_by_label_value_pair{};
  uint32_t num_series{};
  uint32_t chunk_count{};
  uint32_t num_label_pairs{};

  bool operator==(const Status&) const noexcept = default;
};

template <class Element>
class TopItems {
 public:
  using Elements = std::vector<Element>;

  explicit TopItems(size_t limit) {
    assert(limit > 0);
    elements_.resize(limit);
    min_element_ = &elements_.front();
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const Element& min_element() const noexcept { return *min_element_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const Elements& elements() const noexcept { return elements_; }

  template <class ElementConstructor>
  PROMPP_ALWAYS_INLINE void add(uint32_t count, ElementConstructor&& element_constructor) noexcept {
    if (count > min_element_->count) {
      *min_element_ = std::forward<ElementConstructor>(element_constructor)();
      // min_element returns iterator with overloaded operator `*` that returns reference to element in vector's memory.
      // Explicit operator `&` reinterprets reference into pointer to address in vector's memory.
      // Important! We assume that we never reallocate vector's memory, so pointer always valid.
      min_element_ = &*std::ranges::min_element(elements_, [](const Element& lhs, const Element& rhs) PROMPP_LAMBDA_INLINE { return lhs.count < rhs.count; });
    }
  }

  PROMPP_ALWAYS_INLINE void sort() noexcept {
    std::ranges::sort(elements_, [](const Element& lhs, const Element& rhs) PROMPP_LAMBDA_INLINE { return lhs.count > rhs.count; });
    min_element_ = &elements_.back();
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t size() const noexcept { return elements_.size(); }

 private:
  Elements elements_;
  Element* min_element_;
};

template <class Lss, class Status>
class StatusGetter {
 public:
  StatusGetter(const Lss& lss, const series_data::DataStorage& data_storage, size_t limit)
      : lss_(lss),
        data_storage_(data_storage),
        top_label_value_count_by_name_{limit},
        top_series_count_by_metric_name_{limit},
        top_memory_in_bytes_by_label_name_{limit},
        top_series_count_by_label_value_pair_{limit} {
    fill();
  }

  void get(Status& status) {
    static constexpr auto fill_top_items = [](const auto& top_items, auto& destination) PROMPP_LAMBDA_INLINE {
      destination.reserve(top_items.size());
      std::ranges::copy_if(top_items.elements(), std::back_inserter(destination), [&](const auto& item) PROMPP_LAMBDA_INLINE { return item.count > 0; });
    };

    status.min_max_timestamp = min_max_timestamp_;
    status.num_series = lss_.size();
    status.chunk_count = data_storage_.chunk_count();
    status.num_label_pairs = label_count_;

    fill_top_items(top_label_value_count_by_name_, status.label_value_count_by_label_name);
    fill_top_items(top_series_count_by_metric_name_, status.series_count_by_metric_name);
    fill_top_items(top_memory_in_bytes_by_label_name_, status.memory_in_bytes_by_label_name);
    fill_top_items(top_series_count_by_label_value_pair_, status.series_count_by_label_value_pair);
  }

 private:
  using StringType = typename Status::String;

  using TopLabelValueCountByLabelName = TopItems<StringCountItem<StringType>>;
  using TopSeriesCountByMetricName = TopItems<StringCountItem<StringType>>;
  using TopMemoryInBytesByLabelName = TopItems<StringCountItem<StringType>>;
  using TopSeriesCountByLabelValuePair = TopItems<StringPairCountItem<StringType>>;

  const Lss& lss_;
  const series_data::DataStorage& data_storage_;

  TopLabelValueCountByLabelName top_label_value_count_by_name_;
  TopSeriesCountByMetricName top_series_count_by_metric_name_;
  TopMemoryInBytesByLabelName top_memory_in_bytes_by_label_name_;
  TopSeriesCountByLabelValuePair top_series_count_by_label_value_pair_;
  PromPP::Primitives::TimeInterval min_max_timestamp_{};
  uint32_t label_count_{};

  void fill() noexcept {
    fill_storage_statistic();
    fill_lss_statistic();
    fill_reverse_index_statistic();
  }

  PROMPP_ALWAYS_INLINE void fill_storage_statistic() noexcept { min_max_timestamp_ = series_data::Decoder::get_time_interval(data_storage_); }

  void fill_lss_statistic() noexcept {
    auto& names = lss_.data().label_name_sets_table.data().symbols_table;
    for (uint32_t name_id = 0; name_id < names.size(); ++name_id) {
      auto count = lss_.data().symbols_tables[name_id]->size();

      label_count_ += count;
      top_label_value_count_by_name_.add(count,
                                         [&] PROMPP_LAMBDA_INLINE { return StringCountItem<StringType>{.name = StringType(names[name_id]), .count = count}; });
    }

    top_label_value_count_by_name_.sort();
  }

  void fill_reverse_index_statistic() noexcept {
    const auto metric_name_id = lss_.trie_index().names_trie().lookup(PromPP::Prometheus::kMetricLabelName).value_or(std::numeric_limits<uint32_t>::max());

    auto& names = lss_.reverse_index().labels_by_name();
    for (uint32_t name_id = 0; name_id < names.size(); ++name_id) {
      auto& values = names[name_id].series_by_value();

      enumerate_values_in_reverse_index(values, name_id);
      if (name_id == metric_name_id) [[unlikely]] {
        enumerate_metric_name_values_in_reverse_index(values, name_id);
      }
    }

    top_series_count_by_metric_name_.sort();
    top_memory_in_bytes_by_label_name_.sort();
    top_series_count_by_label_value_pair_.sort();
  }

  void enumerate_values_in_reverse_index(const BareBones::Vector<series_index::CompactSeriesIdSequence>& values, uint32_t name_id) noexcept {
    uint32_t size_in_bytes = 0;

    for (uint32_t value_id = 0; value_id < values.size(); ++value_id) {
      auto series_count = values[value_id].count();

      auto label_value = get_label_value(name_id, value_id);
      size_in_bytes += series_count * label_value.size();

      top_series_count_by_label_value_pair_.add(series_count, [&] PROMPP_LAMBDA_INLINE {
        return StringPairCountItem<StringType>{.name = get_label_name(name_id), .value = label_value, .count = series_count};
      });
    }

    top_memory_in_bytes_by_label_name_.add(
        size_in_bytes, [&] PROMPP_LAMBDA_INLINE { return StringCountItem<StringType>{.name = get_label_name(name_id), .count = size_in_bytes}; });
  }

  void enumerate_metric_name_values_in_reverse_index(const BareBones::Vector<series_index::CompactSeriesIdSequence>& values, uint32_t name_id) {
    for (uint32_t value_id = 0; value_id < values.size(); ++value_id) {
      auto series_count = values[value_id].count();
      top_series_count_by_metric_name_.add(
          series_count, [&] PROMPP_LAMBDA_INLINE { return StringCountItem<StringType>{.name = get_label_value(name_id, value_id), .count = series_count}; });
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE StringType get_label_name(uint32_t name_id) const noexcept {
    return StringType(lss_.data().label_name_sets_table.data().symbols_table[name_id]);
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE StringType get_label_value(uint32_t name_id, uint32_t value_id) const noexcept {
    return StringType(lss_.data().symbols_tables[name_id]->operator[](value_id));
  }
};

}  // namespace head