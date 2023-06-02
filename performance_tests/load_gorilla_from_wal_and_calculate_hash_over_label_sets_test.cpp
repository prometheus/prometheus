#include "load_gorilla_from_wal_and_calculate_hash_over_label_sets_test.h"

#include <chrono>

#include <lz4_stream.h>

#include "primitives/primitives.h"
#include "wal/wal.h"

#include "log.h"

using namespace PromPP;  // NOLINT

std::chrono::nanoseconds load_gorilla_from_wal_and_calculate_hash_over_label_sets::process_data(WAL::Reader& wal) const {
  auto start = std::chrono::steady_clock::now();
  uint64_t xor_of_ls_hash = 0;
  wal.process_segment([&](WAL::Reader::label_set_type label_set, uint64_t ts, double v) { xor_of_ls_hash ^= hash_value(label_set); });
  auto end = std::chrono::steady_clock::now();
  auto period = end - start;
  period_ += period;

  log() << "XOR of all LabelSets hashes: " << xor_of_ls_hash << std::endl;

  return period;
}

void load_gorilla_from_wal_and_calculate_hash_over_label_sets::write_metrics(Metrics& metrics) const {
  metrics << (Metric() << "wal_loading_from_file_and_calculation_hash_over_label_sets_duration_microseconds" << (period_.count() / 1000.0));
}
