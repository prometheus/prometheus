#include "full_save_lss_test.h"

#include <chrono>

#include <lz4_stream.h>

#include "primitives/snug_composites.h"

using namespace PromPP;  // NOLINT

void full_save_lss::execute(const Config& config, Metrics& metrics) const {
  std::ifstream infile(input_file_full_name(config), std::ios_base::binary);
  lz4_stream::istream in(infile);
  if (!infile.is_open()) {
    throw std::runtime_error("failed to open file '" + input_file_full_name(config) + "'");
  }

  Primitives::SnugComposites::LabelSet::DecodingTable lss;
  while (!in.eof()) {
    in >> lss;
  }

  std::ofstream outfile(output_file_full_name(config), std::ios_base::binary);
  lz4_stream::ostream out(outfile);
  if (!outfile.is_open()) {
    throw std::runtime_error("failed to open file '" + output_file_full_name(config) + "'");
  }

  auto start = std::chrono::steady_clock::now();
  out << lss;
  auto now = std::chrono::steady_clock::now();

  metrics << (Metric() << "decoding_table_save_to_file_duration_microseconds" << (std::chrono::duration_cast<std::chrono::microseconds>(now - start).count()));
}
