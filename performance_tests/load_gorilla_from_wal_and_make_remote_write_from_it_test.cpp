#include "load_gorilla_from_wal_and_make_remote_write_from_it_test.h"

#include <chrono>

#include <lz4_stream.h>

#include "primitives/primitives.h"
#include "prometheus/remote_write.h"
#include "wal/wal.h"

#include "log.h"

using namespace PromPP;  // NOLINT

std::chrono::nanoseconds load_gorilla_from_wal_and_make_remote_write_from_it::process_data(WAL::Reader& wal) const {
  std::string protobuf_buffer;
  auto start = std::chrono::steady_clock::now();
  protozero::pbf_writer pb_message(protobuf_buffer);
  wal.process_segment([&](WAL::Reader::timeseries_type timeseries) { Prometheus::RemoteWrite::write_timeseries(pb_message, timeseries); });
  protobuf_buffer_total_size += protobuf_buffer.size();
  auto end = std::chrono::steady_clock::now();
  auto period = end - start;
  period_ += period;

  log() << "PB size: " << protobuf_buffer.size() << std::endl;
  log() << "PB total size: " << protobuf_buffer_total_size << " (" << (protobuf_buffer_total_size >> 20) << " MB)" << std::endl;

  return period;
}

void load_gorilla_from_wal_and_make_remote_write_from_it::write_metrics(Metrics& metrics) const {
  metrics << (Metric() << "wal_loading_from_file_and_make_remote_write_duration_microseconds" << (period_.count() / 1000.0));
}
