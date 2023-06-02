#include "load_gorilla_from_wal_and_iterate_over_label_name_and_value_ids_test.h"

#include <chrono>

#include <lz4_stream.h>

#include "primitives/primitives.h"
#include "wal/wal.h"

#include "log.h"

using namespace PromPP;  // NOLINT

std::chrono::nanoseconds load_gorilla_from_wal_and_iterate_over_label_name_and_value_ids::process_data(PromPP::WAL::Reader& wal) const {
  auto start = std::chrono::steady_clock::now();
  uint64_t sum_of_ls_symbols_ids = 0;
  wal.process_segment([&](WAL::Reader::label_set_type label_set, uint64_t ts, double v) {
    for (auto i = label_set.begin(); i != label_set.end(); ++i) {
      sum_of_ls_symbols_ids += i.name_id() + i.value_id();
    }
  });
  auto end = std::chrono::steady_clock::now();
  auto period = end - start;
  period_ += period;

  log() << "Sum of ls symbols ids: " << sum_of_ls_symbols_ids << std::endl;

  return period;
}

void load_gorilla_from_wal_and_iterate_over_label_name_and_value_ids::write_metrics(Metrics& metrics) const {
  metrics << (Metric() << "wal_loading_from_file_and_iterating_over_label_name_and_value_ids_duration_microseconds" << (period_.count() / 1000.0));
}
