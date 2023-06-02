#include "save_gorilla_to_wal_test.h"

#include <chrono>

#include "dummy_wal.h"
#include "wal/wal.h"

#include "log.h"

using namespace PromPP;  // NOLINT

static void ppp(const char* str, uint64_t size, uint64_t cnt) {
  log() << "Estimated " << str << " size: " << size << " (" << (size >> 20) << "M), " << static_cast<double>(size) * 8 / cnt << " bits per sample" << std::endl;
}

void save_gorilla_to_wal::execute(const Config& config, Metrics& metrics) const {
  DummyWal::Timeseries tmsr;
  DummyWal dummy_wal(input_file_full_name(config));

  std::ofstream outfile(output_file_full_name(config), std::ios_base::binary);
  lz4_stream::ostream out(outfile);
  if (!outfile.is_open()) {
    throw std::runtime_error("failed to open file '" + output_file_full_name(config) + "'");
  }

  WAL::Writer wal;

  auto start = std::chrono::steady_clock::now();
  uint64_t count = 0;
  uint64_t segments = 0;
  while (dummy_wal.read_next_segment()) {
    while (dummy_wal.read_next(tmsr)) {
      wal.add(tmsr);
      ++count;

      if (dummy_wal.cnt() % 1000000 == 0) {
        log() << "Number of label name symbols: " << wal.label_sets().data().label_name_sets_table.data().symbols_table.size() << std::endl;
        log() << "Number of label name sets: " << wal.label_sets().data().label_name_sets_table.size() << std::endl;
        log() << "Number of label sets: " << wal.label_sets().size() << std::endl;
        ppp("label_sets", wal.label_sets_bytes(), wal.samples());
        ppp("ls_id", wal.ls_id_bytes(), wal.samples());
        ppp("ts", wal.timestamps_bytes(), wal.samples());
        ppp("values", wal.values_bytes(), wal.samples());
        ppp("TOTAL", wal.total_bytes(), wal.samples());
        log() << std::endl;
      }
    }
    ++segments;
    out << wal << std::flush;
  }
  log() << "Writed points: " << count << std::endl;
  log() << "Writed segments: " << segments << std::endl;
  auto now = std::chrono::steady_clock::now();

  metrics << (Metric() << "wal_label_sets_compression_bits_per_sample" << static_cast<double>(wal.label_sets_bytes()) * 8 / wal.samples());
  metrics << (Metric() << "wal_label_set_ids_compression_bits_per_sample" << static_cast<double>(wal.ls_id_bytes()) * 8 / wal.samples());
  metrics << (Metric() << "wal_timestamps_compression_bits_per_sample" << static_cast<double>(wal.timestamps_bytes()) * 8 / wal.samples());
  metrics << (Metric() << "wal_total_compression_bits_per_sample" << static_cast<double>(wal.total_bytes()) * 8 / wal.samples());
  metrics << (Metric() << "wal_values_compression_bits_per_sample" << static_cast<double>(wal.timestamps_bytes()) * 8 / wal.samples());
  metrics << (Metric() << "wal_add_sample_avg_duration_nanoseconds"
                       << (std::chrono::duration_cast<std::chrono::nanoseconds>(now - start).count() / dummy_wal.cnt()));
}
