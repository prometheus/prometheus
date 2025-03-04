#include "load_gorilla_from_wal_and_iterate_over_label_names_and_values_test.h"

#include <chrono>

#include "bare_bones/lz4_stream.h"
#include "primitives/primitives.h"
#include "wal/wal.h"

#include "log.h"

using namespace PromPP;  // NOLINT

std::chrono::nanoseconds load_gorilla_from_wal_and_iterate_over_label_names_and_values::process_data(WAL::Reader& wal) const {
  auto start = std::chrono::steady_clock::now();
  uint64_t sum_of_ls_symbols_lengths = 0;
  wal.process_segment([&](WAL::Reader::label_set_type label_set, uint64_t, double) {
    for (const auto& [ln, lv] : label_set) {
      sum_of_ls_symbols_lengths += ln.length() + lv.length();
    }
  });
  auto end = std::chrono::steady_clock::now();
  auto period = end - start;
  period_ += period;

  log() << "Sum of ls symbols lengths: " << sum_of_ls_symbols_lengths << std::endl;

  return period;
}

void load_gorilla_from_wal_and_iterate_over_label_names_and_values::write_metrics(Metrics& metrics) const {
  metrics << (Metric() << "wal_loading_from_file_and_iterating_over_label_names_and_values_duration_microseconds" << (period_.count() / 1000.0));
}
