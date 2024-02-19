#include "generate_series_reverse_index_test.h"

#include <chrono>

#include "performance_tests/dummy_wal.h"
#include "primitives/series_reverse_index.h"
#include "wal/wal.h"

namespace performance_tests::series_index {

void GenerateSeriesReverseIndex::execute([[maybe_unused]] const Config& config, [[maybe_unused]] Metrics& metrics) const {
  DummyWal::Timeseries tmsr;
  DummyWal dummy_wal(input_file_full_name(config));

  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap label_set_bitmap;
  PromPP::Primitives::SeriesReverseIndex series_reverse_index;
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
      auto start_tm = std::chrono::steady_clock::now();
      for (auto i = ls.begin(); i != ls.end(); ++i) {
        series_reverse_index.add(i, ls_id);
      }
      add_index_time += std::chrono::steady_clock::now() - start_tm;
    }
  }

  if (previous_label_id != std::numeric_limits<uint32_t>::max()) {
    metrics << (Metric() << "series_reverse_index_add_allocated_memory_kb" << series_reverse_index.allocated_memory() / 1024);
    metrics << (Metric() << "series_reverse_index_add_nanoseconds" << (add_index_time.count() / (previous_label_id + 1)));
  }
}

}  // namespace performance_tests::series_index