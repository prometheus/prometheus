#include "load_gorilla_from_wal_and_iterate_over_series_label_name_ids_test.h"

#include <chrono>

#include <lz4_stream.h>

#include "primitives/primitives.h"

#include "log.h"

using namespace PromPP;  // NOLINT

std::chrono::nanoseconds load_gorilla_from_wal_and_iterate_over_series_label_name_ids::process_data(WAL::Reader& wal) const {
  auto start = std::chrono::steady_clock::now();
  uint64_t sum_of_ls_name_symbols_ids = 0;
  uint64_t count = 0;
  wal.process_segment([&](WAL::Reader::timeseries_type timeseries) {
    count += timeseries.samples().size();
    for (auto i = timeseries.label_set().names().begin(); i != timeseries.label_set().names().end(); ++i) {
      sum_of_ls_name_symbols_ids += i.id();
    }
  });
  auto end = std::chrono::steady_clock::now();
  auto period = end - start;
  period_ += period;

  log() << "Sum of ls name symbols ids: " << sum_of_ls_name_symbols_ids << std::endl;

  log() << "Writed points: " << count << std::endl;

  return period;
}

void load_gorilla_from_wal_and_iterate_over_series_label_name_ids::write_metrics(Metrics& metrics) const {
  metrics << (Metric() << "wal_loading_from_file_and_iterating_over_series_label_name_ids_duration_microseconds" << (period_.count() / 1000.0));
}
