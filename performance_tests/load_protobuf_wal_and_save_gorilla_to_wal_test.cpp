#include "load_protobuf_wal_and_save_gorilla_to_wal_test.h"

#include <chrono>

#include <lz4_stream.h>

#include "prometheus/remote_write.h"
#include "wal/wal.h"

#include "third_party/protozero/pbf_reader.hpp"

#include "log.h"

using namespace PromPP;  // NOLINT

void load_protobuf_wal_and_save_gorilla_to_wal::execute(const Config& config, Metrics& metrics) const {
  std::ifstream infile(input_file_full_name(config), std::ios_base::in | std::ios_base::binary);
  lz4_stream::istream in(infile);
  if (!infile.is_open()) {
    throw std::runtime_error("failed to open file '" + input_file_full_name(config) + "'");
  }

  std::ofstream outfile(output_file_full_name(config), std::ios_base::binary);
  lz4_stream::ostream out(outfile);
  if (!outfile.is_open()) {
    throw std::runtime_error("failed to open file '" + output_file_full_name(config) + "'");
  }

  WAL::Writer wal;

  std::string buffer;
  uint64_t number_of_samples = 0;

  auto start = std::chrono::steady_clock::now();
  while (!in.eof()) {
    uint32_t size_to_read;
    in.read(reinterpret_cast<char*>(&size_to_read), sizeof(size_to_read));

    if (in.bad()) {
      throw std::runtime_error("bad lz4 stream");
    }

    buffer.resize(size_to_read);
    in.read(buffer.data(), size_to_read);

    protozero::pbf_reader pb(buffer);
    Prometheus::RemoteWrite::read_many_timeseries<Primitives::TimeseriesSemiview>(pb, [&](const auto& timeseries) {
      wal.add(timeseries);
      number_of_samples += timeseries.samples().size();
    });

    if (number_of_samples % 1000000 == 0) {
      auto now = std::chrono::steady_clock::now();
      log() << "Number of samples processed: " << number_of_samples << std::endl;
      log() << "Process time per sample (overall avg): " << (std::chrono::duration_cast<std::chrono::nanoseconds>(now - start).count() / number_of_samples)
            << " ns" << std::endl;
    }

    out << wal << std::flush;
  }

  auto now = std::chrono::steady_clock::now();

  metrics << (Metric() << "protobuf_wal_loading_sample_avg_duration_nanoseconds"
                       << std::chrono::duration_cast<std::chrono::nanoseconds>(now - start).count() / number_of_samples);
}
