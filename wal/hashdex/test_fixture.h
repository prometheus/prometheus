#pragma once

#include <iostream>

#include "metric.h"

namespace PromPP::WAL::hashdex {

inline std::ostream& operator<<(std::ostream& stream, const Metric& item) {
  stream << "hash: " << item.hash << ", labels: { ";

  for (const auto& [name, value] : item.timeseries.label_set()) {
    stream << name << " = " << value << ", ";
  }

  stream << " }, samples: { ";

  for (const auto& [timestamp, value] : item.timeseries.samples()) {
    stream << timestamp << " => " << value << ", ";
  }

  stream << " }";

  return stream;
}

inline std::ostream& operator<<(std::ostream& stream, const Metadata& item) {
  stream << "[" << static_cast<int>(item.type) << "] " << item.metric_name << ": " << item.text;
  return stream;
}

inline void calculate_labelset_hash(std::vector<Metric>& metrics) noexcept {
  for (auto& item : metrics) {
    item.hash = Primitives::hash::hash_of_label_set(item.timeseries.label_set());
  }
}

template <class Metrics>
[[nodiscard]] std::vector<Metric> get_metrics(const Metrics& metrics) noexcept {
  std::vector<Metric> items;
  items.reserve(metrics.size());

  for (auto& item : metrics) {
    auto& scraped_item = items.emplace_back(Metric{.hash = item.hash()});
    item.read(scraped_item.timeseries);
  }

  return items;
}

template <class MetadataType>
[[nodiscard]] std::vector<Metadata> get_metadata(const MetadataType& metadata) noexcept {
  std::vector<Metadata> items;
  items.reserve(metadata.size());

  for (auto& item : metadata) {
    if constexpr (std::is_same_v<decltype(item), const Metadata&>) {
      items.emplace_back(Metadata{.metric_name = item.metric_name, .text = item.text, .type = item.type});
    } else {
      items.emplace_back(Metadata{.metric_name = item.metric_name(), .text = item.text(), .type = item.type()});
    }
  }

  return items;
}

}  // namespace PromPP::WAL::hashdex