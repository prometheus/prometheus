#include <chrono>
#include <fstream>

#include <benchmark/benchmark.h>

#include "bare_bones/preprocess.h"
#include "primitives/sample.h"
#include "series_data/encoder.h"
#include "series_data/outdated_sample_encoder.h"

namespace {
// timestamp min value
constexpr PromPP::Primitives::Sample::timestamp_type ts_min = 1698395400012;

struct PROMPP_ATTRIBUTE_PACKED sample_with_lsid {
  PromPP::Primitives::Sample::value_type sample_value;
  uint32_t sample_ts;
  uint32_t labelset_id;
};

void BenchmarkSeriesDataEncoder(benchmark::State& state) {
  constexpr auto get_file_name = [] -> std::string {
    if (auto& context = benchmark::internal::GetGlobalContext(); context != nullptr) {
      return context->operator[]("sde_file");
    }

    return {};
  };

  BareBones::Vector<sample_with_lsid> samples_from_file;
  {
    std::ifstream istrm(get_file_name(), std::ios::binary);
    istrm >> samples_from_file;
  }

  series_data::DataStorage storage;
  std::chrono::system_clock clock;
  series_data::OutdatedSampleEncoder outdated_sample_encoder{clock};
  series_data::Encoder encoder{storage, outdated_sample_encoder};

  for ([[maybe_unused]] auto _ : state) {
    for (auto& cur_sample : samples_from_file) {
      encoder.encode(cur_sample.labelset_id, ts_min + static_cast<PromPP::Primitives::Sample::timestamp_type>(cur_sample.sample_ts), cur_sample.sample_value);
    }
  }

  state.counters["Items"] = benchmark::Counter(static_cast<double>(samples_from_file.size()));
  state.counters["Time/item"] = benchmark::Counter(static_cast<double>(samples_from_file.size()), benchmark::Counter::kIsRate | benchmark::Counter::kInvert);

  state.counters["Memory"] =
      benchmark::Counter(static_cast<double>(storage.allocated_memory()), benchmark::Counter::kDefaults, benchmark::Counter::OneK::kIs1024);
}

BENCHMARK(BenchmarkSeriesDataEncoder)->ComputeStatistics("min", [](const std::vector<double>& v) { return *std::ranges::min_element(v); });

}  // namespace
