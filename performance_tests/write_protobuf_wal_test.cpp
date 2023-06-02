#include "write_protobuf_wal_test.h"

#include "dummy_wal.h"
#include "prometheus/remote_write.h"

#include "third_party/protozero/pbf_writer.hpp"

#include "log.h"

using namespace PromPP;  // NOLINT

void write_protobuf_wal::execute(const Config& config, Metrics& metrics) const {
  DummyWal::Timeseries tmsr;
  DummyWal dummy_wal(input_file_full_name(config));

  std::ofstream outfile(output_file_full_name(config), std::ios_base::binary);
  lz4_stream::ostream out(outfile);
  if (!outfile.is_open()) {
    throw std::runtime_error("failed to open file '" + output_file_full_name(config) + "'");
  }

  std::string buffer;
  uint64_t encoded_size = 0;

  // it's ugly here, because dummy wal can have only 1 sample in timeseries
  auto start = std::chrono::steady_clock::now();
  while (dummy_wal.read_next_segment()) {
    protozero::pbf_writer pb_message(buffer);

    protozero::pbf_writer pb_timeseries(pb_message, 1);
    bool first = true;
    size_t prev_ls_chksm = 0;
    while (dummy_wal.read_next(tmsr)) {
      size_t ls_chksm = hash_value(tmsr.label_set());

      if (ls_chksm != prev_ls_chksm || first) {
        if (!first) {
          pb_timeseries.commit();
          pb_timeseries = protozero::pbf_writer(pb_message, 1);
        }
        first = false;
        prev_ls_chksm = ls_chksm;

        Prometheus::RemoteWrite::write_label_set(pb_timeseries, tmsr.label_set());
      }

      for (const auto& smpl : tmsr.samples()) {
        Prometheus::RemoteWrite::write_sample(pb_timeseries, smpl);
      }

      if (dummy_wal.cnt() % 1000000 == 0) {
        auto now = std::chrono::steady_clock::now();
        log() << "Processed: " << dummy_wal.cnt()
              << " Time per sample: " << (std::chrono::duration_cast<std::chrono::nanoseconds>(now - start).count() / dummy_wal.cnt()) << " ns" << std::endl;
        log() << "Encoded size: " << ((encoded_size + buffer.size()) >> 20) << "MB" << std::endl;
      }
    }
    pb_timeseries.commit();

    // write size
    uint32_t segment_size = buffer.size();
    out.write(reinterpret_cast<const char*>(&segment_size), sizeof(segment_size));

    // write message
    out.write(buffer.data(), buffer.size());

    encoded_size += buffer.size();

    buffer.resize(0);
  }

  auto now = std::chrono::steady_clock::now();

  metrics << (Metric() << "protobuf_wal_add_sample_avg_duration_nanoseconds"
                       << std::chrono::duration_cast<std::chrono::nanoseconds>(now - start).count() / dummy_wal.cnt());
  metrics << (Metric() << "protobuf_wal_overall_size_megabytes" << (encoded_size >> 20));
}
