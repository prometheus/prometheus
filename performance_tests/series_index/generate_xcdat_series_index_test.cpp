#include "generate_xcdat_series_index_test.h"

#include <chrono>

#include "xcdat/xcdat.hpp"

#include "performance_tests/dummy_wal.h"
#include "wal/wal.h"

namespace performance_tests::series_index {

void GenerateXcdatSeriesIndex::execute([[maybe_unused]] const Config& config, [[maybe_unused]] Metrics& metrics) const {
  DummyWal::Timeseries tmsr;
  DummyWal dummy_wal(input_file_full_name(config));

  auto* names_list = new std::vector<std::string>();
  auto* labels_values_vector = new std::vector<std::vector<std::string>>();

  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap label_set_bitmap;
  uint32_t previous_label_id = std::numeric_limits<uint32_t>::max();
  std::chrono::nanoseconds add_index_time{};

  while (dummy_wal.read_next_segment()) {
    while (dummy_wal.read_next(tmsr)) {
      auto ls_id = label_set_bitmap.find_or_emplace(tmsr.label_set());
      if (previous_label_id != std::numeric_limits<uint32_t>::max() && ls_id <= previous_label_id) {
        continue;
      }

      previous_label_id = ls_id;

      auto ls = label_set_bitmap[ls_id];
      auto ls_it = tmsr.label_set().begin();
      for (auto i = ls.begin(); i != ls.end(); ++i) {
        names_list->emplace_back((*ls_it).first);

        if (labels_values_vector->size() <= i.name_id()) {
          labels_values_vector->emplace_back();
        }

        (*labels_values_vector)[i.name_id()].emplace_back((*ls_it).second);
        ++ls_it;
      }
    }
  }

  std::sort(names_list->begin(), names_list->end());
  names_list->erase(std::unique(names_list->begin(), names_list->end()), names_list->end());

  auto start_tm = std::chrono::steady_clock::now();
  xcdat::trie_15_type names_trie(*names_list);
  add_index_time += std::chrono::steady_clock::now() - start_tm;

  std::vector<xcdat::trie_15_type> labels_trie_vector;
  labels_trie_vector.reserve(labels_values_vector->size());

  for (auto& values : *labels_values_vector) {
    std::sort(values.begin(), values.end());
    values.erase(std::unique(values.begin(), values.end()), values.end());

    start_tm = std::chrono::steady_clock::now();
    labels_trie_vector.emplace_back(values);
    add_index_time += std::chrono::steady_clock::now() - start_tm;
  }

  delete names_list;
  delete labels_values_vector;

  if (previous_label_id != std::numeric_limits<uint32_t>::max()) {
    metrics << (Metric() << "xcdat_series_index_add_nanoseconds" << (add_index_time.count() / (previous_label_id + 1)));
  }
}

}  // namespace performance_tests::series_index