#include "load_gorilla_from_wal_and_iterate_over_label_set_ids_test.h"

#include <chrono>

#include <lz4_stream.h>

#include "primitives/primitives.h"

#include "log.h"

using namespace PromPP;  // NOLINT

std::chrono::nanoseconds load_gorilla_from_wal_and_iterate_over_label_set_ids::process_data(WAL::Reader& wal) const {
  auto start = std::chrono::steady_clock::now();
  uint64_t sum_of_ls_id = 0;
  wal.process_segment([&](Primitives::LabelSetID ls_id, uint64_t ts, double v) { sum_of_ls_id += ls_id; });
  auto end = std::chrono::steady_clock::now();
  auto period = std::chrono::duration_cast<std::chrono::nanoseconds>(end - start);
  period_ += period;
  log() << "Sum of ls_id: " << sum_of_ls_id << std::endl;
  return period;
}

void load_gorilla_from_wal_and_iterate_over_label_set_ids::write_metrics(Metrics& metrics) const {
  metrics << (Metric() << "wal_loading_from_file_and_iterating_over_label_set_ids_duration_microseconds" << period_.count() / 1000.0);
}
