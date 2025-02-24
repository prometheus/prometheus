#include <benchmark/benchmark.h>

#include <numeric>

#include "performance_tests/dummy_wal.h"
#include "series_index/reverse_index.h"
#include "wal/wal.h"

namespace {

using BareBones::StreamVByte::CompactSequence;
using BareBones::StreamVByte::Sequence;

struct Label {
  uint32_t ls_id_;
  uint32_t name_id_;
  uint32_t value_id_;

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t value_id() const noexcept { return value_id_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t name_id() const noexcept { return name_id_; }
};

std::string_view get_wal_file() {
  if (auto& context = benchmark::internal::GetGlobalContext(); context != nullptr) {
    return context->operator[]("wal_file");
  }

  return {};
}

BareBones::Vector<Label> get_labels() {
  BareBones::Vector<Label> labels;
  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap<BareBones::Vector> label_set_bitmap;

  DummyWal::Timeseries tmsr;
  DummyWal dummy_wal{std::string{get_wal_file()}};
  uint32_t previous_label_id = std::numeric_limits<uint32_t>::max();

  while (dummy_wal.read_next_segment()) {
    while (dummy_wal.read_next(tmsr)) {
      const auto ls_id = label_set_bitmap.find_or_emplace(tmsr.label_set());
      if (previous_label_id != std::numeric_limits<uint32_t>::max() && ls_id <= previous_label_id) {
        continue;
      }

      previous_label_id = ls_id;

      auto ls = label_set_bitmap[ls_id];
      for (auto i = ls.begin(); i != ls.end(); ++i) {
        labels.emplace_back(Label{.ls_id_ = ls_id, .name_id_ = i.name_id(), .value_id_ = i.value_id()});
      }
    }
  }

  return labels;
}

void BenchmarkGenerateReverseIndex(benchmark::State& state) {
  static const auto& labels = get_labels();
  static size_t allocated_memory = 0;

  for ([[maybe_unused]] auto _ : state) {
    series_index::SeriesReverseIndex series_reverse_index;

    for (const auto& label : labels) {
      series_reverse_index.add(label, label.ls_id_);
    }
  }

  if (allocated_memory == 0) [[unlikely]] {
    series_index::SeriesReverseIndex series_reverse_index;

    for (const auto& label : labels) {
      series_reverse_index.add(label, label.ls_id_);
    }

    allocated_memory = series_reverse_index.allocated_memory();
  }
  state.counters["Memory"] = benchmark::Counter(static_cast<double>(allocated_memory), benchmark::Counter::kDefaults, benchmark::Counter::kIs1024);
}

double min_value(const std::vector<double>& v) noexcept {
  return *std::ranges::min_element(v);
}

BENCHMARK(BenchmarkGenerateReverseIndex)->ComputeStatistics("min", min_value);

}  // namespace
