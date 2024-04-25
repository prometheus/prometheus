#include "generate_cedarpp_series_index_test.h"

#include <chrono>

#include "cedar/cedarpp.h"

#include "performance_tests/dummy_wal.h"
#include "wal/wal.h"

namespace performance_tests::series_index {

void GenerateCedarppSeriesIndex::execute([[maybe_unused]] const Config& config, [[maybe_unused]] Metrics& metrics) const {
  DummyWal::Timeseries tmsr;
  DummyWal dummy_wal(input_file_full_name(config));

  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap label_set_bitmap;
  uint32_t previous_label_id = std::numeric_limits<uint32_t>::max();
  cedar::da<uint32_t> names_trie;
  std::vector<std::unique_ptr<cedar::da<uint32_t>>> labels_trie_vector;
  std::chrono::nanoseconds add_index_time{};

  while (dummy_wal.read_next_segment()) {
    while (dummy_wal.read_next(tmsr)) {
      auto ls_id = label_set_bitmap.find_or_emplace(tmsr.label_set());
      if (previous_label_id != std::numeric_limits<uint32_t>::max() && ls_id <= previous_label_id) {
        continue;
      }

      previous_label_id = ls_id;

      auto ls = label_set_bitmap[ls_id];
      auto start_tm = std::chrono::steady_clock::now();
      auto ls_it = tmsr.label_set().begin();
      for (auto i = ls.begin(); i != ls.end(); ++i) {
        names_trie.update((*ls_it).first.data(), (*ls_it).first.length(), 0) = i.name_id();

        if (labels_trie_vector.size() <= i.name_id()) {
          labels_trie_vector.emplace_back(new cedar::da<uint32_t>());
        }

        labels_trie_vector[i.name_id()]->update((*ls_it).second.data(), (*ls_it).second.length(), 0) = i.value_id();
        ++ls_it;
      }
      add_index_time += std::chrono::steady_clock::now() - start_tm;
    }
  }

  if (previous_label_id != std::numeric_limits<uint32_t>::max()) {
    metrics << (Metric() << "cedarpp_series_index_add_nanoseconds" << (add_index_time.count() / (previous_label_id + 1)));
  }

  size_t allocated_memory =
      names_trie.allocated_memory() + std::accumulate(labels_trie_vector.begin(), labels_trie_vector.end(), 0ULL,
                                                      [](size_t sum, const auto& label_trie) { return sum + label_trie->allocated_memory(); });
  metrics << (Metric() << "cedarpp_series_index_allocated_memory_kb" << allocated_memory / 1024);
}

}  // namespace performance_tests::series_index