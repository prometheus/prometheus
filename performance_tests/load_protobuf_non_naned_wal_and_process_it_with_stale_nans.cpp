#include "load_protobuf_non_naned_wal_and_process_it_with_stale_nans.h"

#include "prometheus/remote_write.h"

#include "third_party/protozero/pbf_reader.hpp"

#include "bare_bones/gorilla.h"  // stale_nan utils

#include "dummy_wal.h"
#include "wal/wal.h"

#include "log.h"

#include <chrono>
#include <ios>

using namespace PromPP;  // NOLINT

// non_naned_wal message struct type.
struct Message {
  uint32_t id;
  int64_t timestamp;
  std::string buffer;
};

// struct

template <typename Ostream>
Message read_message(Ostream& infile) {
  assert(infile.good());
  Message msg;
  uint32_t buf_sz_ui32;
  infile.read(reinterpret_cast<char*>(&msg.id), sizeof(msg.id));
  infile.read(reinterpret_cast<char*>(&msg.timestamp), sizeof(msg.timestamp));
  infile.read(reinterpret_cast<char*>(&buf_sz_ui32), sizeof(buf_sz_ui32));
  msg.buffer.resize_and_overwrite(buf_sz_ui32, [&infile](char* buf, size_t sz) {
    infile.read(buf, sz);
    return sz;
  });
  return msg;
}

// helper fn for converting from message to Timeseries
auto timeseries_from_msg(const Message& msg) {
  Primitives::TimeseriesSemiview tmsr;  // dummywal is not suitable
  protozero::pbf_reader pb_message(msg.buffer);
  bool have_next = pb_message.next();  // it is required to get next "message" field, or else we will flatten the timeseries into labels
  if (!have_next) {
    throw std::runtime_error("Invalid pb message in non_naned wal, expected that pb_message.next() will return true");
  }
  protozero::pbf_reader pb_timeseries = pb_message.get_message();
  Prometheus::RemoteWrite::read_timeseries(pb_timeseries, tmsr);
  return tmsr;
}

struct BenchParams {
  virtual const char* name() const = 0;
  virtual const char* total_processed_metrics_name() const = 0;
  virtual const char* avg_size_metrics_name() const = 0;
  virtual void benched_cb(const Message& msg) {}
  virtual bool need_processed_messages_count() { return false; }
};

void run_bench(BenchParams& benchmark_params, const std::string& path, const Config& config, Metrics& metrics) {
  using namespace std::chrono_literals;
  std::ifstream infile(path, std::ios_base::binary);
  lz4_stream::istream in(infile);
  size_t processed_messages_count = 0;

  // 1. read our messages
  in.exceptions(std::ios_base::eofbit | std::ios_base::badbit);

  log() << benchmark_params.name() << " run;\n";
  std::chrono::nanoseconds writer_time_ns = 0ns;
  try {
    while (in.good()) {
      auto message = read_message(in);
      auto start_add = std::chrono::steady_clock::now();
      benchmark_params.benched_cb(message);
      auto end_add = std::chrono::steady_clock::now();
      writer_time_ns += std::chrono::duration_cast<std::chrono::nanoseconds>(end_add - start_add);
      processed_messages_count++;
    }
  } catch (std::ios_base::failure& e) {
    if (processed_messages_count == 0) {
      log() << "no messages" << std::endl;
      return;
    }
  }

  log() << "Total messages processed: " << processed_messages_count << std::endl;
  if (benchmark_params.need_processed_messages_count()) {
    metrics << (Metric() << benchmark_params.total_processed_metrics_name() << processed_messages_count);
  }
  metrics << (Metric() << benchmark_params.avg_size_metrics_name() << (writer_time_ns.count() / processed_messages_count));
}

struct WriterAddCbBenchmark : BenchParams {
  WAL::Writer writer;
  const char* name() const override { return "Writer::add()"; }
  virtual const char* total_processed_metrics_name() const override { return "processed_messages_count"; }
  virtual const char* avg_size_metrics_name() const override { return "wal_writer_add_avg_time"; }
  void benched_cb(const Message& msg) override { writer.add(timeseries_from_msg(msg)); }
};

struct WriterAddManyCbBenchmark : BenchParams {
  WAL::Writer writer;
  decltype(writer)::SourceState handle = 0;
  ~WriterAddManyCbBenchmark() { decltype(writer)::DestroySourceState(handle); }
  const char* name() const override { return "Writer::add_many()"; }
  virtual const char* total_processed_metrics_name() const override { return "processed_messages_count"; }
  virtual const char* avg_size_metrics_name() const override { return "wal_writer_add_many_avg_time"; }
  void benched_cb(const Message& msg) override {
    handle = writer.add_many<decltype(writer)::add_many_generator_callback_type::without_hash_value, Primitives::TimeseriesSemiview>(
        handle, msg.timestamp, [&](auto cb) { cb(timeseries_from_msg(msg)); });
  }
};

void load_protobuf_non_naned_wal_and_process_it_with_stale_nans::execute(const Config& config, Metrics& metrics) const {
  WriterAddCbBenchmark add_bench;
  WriterAddManyCbBenchmark add_many_bench;

  run_bench(add_bench, input_file_full_name(config), config, metrics);
  run_bench(add_many_bench, input_file_full_name(config), config, metrics);
}
