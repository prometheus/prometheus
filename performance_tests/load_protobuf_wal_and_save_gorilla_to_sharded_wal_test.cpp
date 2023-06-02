#include <lz4_stream.h>
#include <chrono>
#include <fstream>
#include <vector>
#include "gtest/gtest.h"

#include "load_protobuf_wal_and_save_gorilla_to_sharded_wal_test.h"
#include "log.h"
#include "primitives/primitives.h"
#include "prometheus/remote_write.h"
#include "wal/wal.h"

using namespace PromPP;  // NOLINT

void load_protobuf_wal_and_save_gorilla_to_sharded_wal::execute(const Config& config, Metrics& metrics) const {
  std::string buffer;
  std::stringstream stream_buffer;
  std::vector<Prometheus::RemoteWrite::TimeseriesProtobufHashdexRecord> hashdex;
  std::vector<std::unique_ptr<WAL::Writer>> wal_shards;

  for (const auto& number_of_shards : numbers_of_shards_) {
    std::ifstream infile(input_file_full_name(config), std::ios_base::in | std::ios_base::binary);
    lz4_stream::istream in(infile);
    if (!infile.is_open()) {
      throw std::runtime_error("failed to open file '" + input_file_full_name(config) + "'");
    }

    for (size_t shard = 0; shard < number_of_shards; ++shard) {
      wal_shards.emplace_back(std::move(std::unique_ptr<WAL::Writer>(new WAL::Writer())));
    }

    auto total_shards_time = 0.0;
    size_t total_stream_buffer_size = 0;
    uint64_t samples = 0;

    while (!in.eof()) {
      uint32_t size_to_read;
      in.read(reinterpret_cast<char*>(&size_to_read), sizeof(size_to_read));

      if (in.bad()) {
        throw std::runtime_error("bad lz4 stream");
      }

      buffer.resize(size_to_read);
      in.read(buffer.data(), size_to_read);

      protozero::pbf_reader pb(buffer);
      Prometheus::RemoteWrite::read_many_timeseries_in_hashdex<Primitives::TimeseriesSemiview,
                                                               std::vector<Prometheus::RemoteWrite::TimeseriesProtobufHashdexRecord>>(pb, hashdex);

      auto start = std::chrono::steady_clock::now();

      Primitives::TimeseriesSemiview timeseries;
      samples += hashdex.size();

      for (size_t number_of_sharder = 0; number_of_sharder < number_of_shards; ++number_of_sharder) {
        for (const auto& [chksm, pb_view] : hashdex) {
          if ((chksm % number_of_shards) == number_of_sharder) {
            Prometheus::RemoteWrite::read_timeseries(protozero::pbf_reader{pb_view}, timeseries);
            (*wal_shards[number_of_sharder]).add(timeseries, chksm);
            timeseries.clear();
          }
        }
        stream_buffer << (*wal_shards[number_of_sharder]);
      }

      auto now = std::chrono::steady_clock::now();
      total_shards_time += std::chrono::duration_cast<std::chrono::nanoseconds>(now - start).count();
      total_stream_buffer_size += stream_buffer.str().size();

      stream_buffer.str(std::string());
      hashdex.clear();
    }

    log() << "total_stream_buffer_size_" << number_of_shards << ": " << total_stream_buffer_size << std::endl;
    log() << "avg_sharded_processing_sample_nanoseconds_per_sample_" << number_of_shards << ": " << (total_shards_time / samples) << std::endl;
    log() << "avg_sharded_processing_sample_nanoseconds_per_sample_parallel_" << number_of_shards << ": " << (total_shards_time / (number_of_shards * samples))
          << std::endl;
    metrics << (Metric() << "avg_sharded_processing_sample_nanoseconds_per_sample_" + std::to_string(number_of_shards) << (total_shards_time / samples));
    metrics << (Metric() << "avg_sharded_processing_sample_nanoseconds_per_sample_parallel_" + std::to_string(number_of_shards)
                         << (total_shards_time / (number_of_shards * samples)));

    wal_shards.clear();
    infile.close();
  }
}
