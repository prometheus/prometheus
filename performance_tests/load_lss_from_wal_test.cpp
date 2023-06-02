#include "load_lss_from_wal_test.h"

#include <chrono>

#include <lz4_stream.h>

#include "primitives/snug_composites.h"

#include "log.h"

using namespace PromPP;  // NOLINT

void load_lss_from_wal::execute(const Config& config, Metrics& metrics) const {
  std::ifstream infile(input_file_full_name(config), std::ios_base::binary);
  lz4_stream::istream in(infile);
  if (!infile.is_open()) {
    throw std::runtime_error("failed to open file '" + input_file_full_name(config) + "'");
  }

  Primitives::SnugComposites::LabelSet::DecodingTable lss;

  auto start = std::chrono::steady_clock::now();
  while (!in.eof()) {
    auto old_size = lss.size();
    in >> lss;

    auto now = std::chrono::steady_clock::now();
    log() << "Loaded label sets: " << (lss.size() - old_size) << std::endl;
    log() << "Load time per label set (overall avg): " << (std::chrono::duration_cast<std::chrono::nanoseconds>(now - start).count() / lss.size()) << " ns"
          << std::endl;
    log() << "Number of label name symbols: " << lss.data().label_name_sets_table.data().symbols_table.size() << std::endl;
    log() << "Number of label name sets: " << lss.data().label_name_sets_table.size() << std::endl;
    log() << "Number of label sets: " << lss.size() << std::endl;
    log() << std::endl;
  }
  auto now = std::chrono::steady_clock::now();

  metrics << (Metric() << "decoding_table_wal_add_label_set_avg_duration_nanoseconds"
                       << static_cast<double>(std::chrono::duration_cast<std::chrono::nanoseconds>(now - start).count()) / lss.size());
}
