#include "load_gorilla_from_wal_and_process_data.h"

#include <lz4_stream.h>

#include "log.h"

using namespace PromPP;  // NOLINT

void load_gorilla_from_wal_and_process_data::execute(const Config& config, Metrics& metrics) const {
  std::ifstream infile(input_file_full_name(config), std::ios_base::binary);
  lz4_stream::istream in(infile);
  if (!infile.is_open()) {
    throw std::runtime_error("failed to open file '" + input_file_full_name(config) + "'");
  }
  std::chrono::nanoseconds period(0);

  WAL::Reader wal;
  while (!in.eof()) {
    uint32_t series = wal.series();
    uint64_t samples = wal.samples();

    auto start = std::chrono::steady_clock::now();
    in >> wal;
    auto end = std::chrono::steady_clock::now();

    auto time_to_process_data = process_data(wal);
    period += time_to_process_data + (end - start);

    log() << "Loaded label sets: " << (wal.series() - series) << std::endl;
    log() << "Number of label name symbols: " << wal.label_sets().data().label_name_sets_table.data().symbols_table.size() << std::endl;
    log() << "Number of label name sets: " << wal.label_sets().data().label_name_sets_table.size() << std::endl;
    log() << "Number of label sets: " << wal.label_sets().size() << std::endl;
    log() << "Loaded samples: " << (wal.samples() - samples) << std::endl;
    log() << "Number of samples processed: " << wal.samples() << std::endl;
    log() << "Load time per sample (overall avg): " << (period.count() / (1.0 * wal.samples())) << " ns" << std::endl;
    log() << std::endl;
  }

  write_metrics(metrics);
}
