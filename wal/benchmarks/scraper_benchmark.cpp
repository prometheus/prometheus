#include <fstream>

#include <benchmark/benchmark.h>

#include "wal/hashdex/scraper/scraper.h"

namespace {

using PromPP::WAL::hashdex::scraper::PrometheusScraper;

void BenchmarkScraper(benchmark::State& state) {
  constexpr auto get_file_name = [] -> std::string {
    if (auto& context = benchmark::internal::GetGlobalContext(); context != nullptr) {
      return context->operator[]("scraper_file");
    }

    return {};
  };

  std::ifstream t(get_file_name());
  std::string str((std::istreambuf_iterator(t)), std::istreambuf_iterator<char>());

  PrometheusScraper scraper;

  for ([[maybe_unused]] auto _ : state) {
    std::ignore = scraper.parse(str, 0);
  }
}

BENCHMARK(BenchmarkScraper)->ComputeStatistics("min", [](const std::vector<double>& v) { return *std::ranges::min_element(v); });

}  // namespace
