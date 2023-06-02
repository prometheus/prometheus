#include "load_ordered_indexing_table_in_loop_test.h"

#include "primitives/snug_composites.h"

#include <lz4_stream.h>

#include <chrono>

using namespace PromPP;  // NOLINT

void load_ordered_indexing_table_in_loop::execute(const Config& config, Metrics& metrics) const {
  Primitives::SnugComposites::LabelSet::OrderedEncodingBimap source_lss;
  std::ifstream infile(input_file_full_name(config), std::ios_base::binary);
  lz4_stream::istream in(infile);
  if (!infile.is_open()) {
    throw std::runtime_error("failed to open file '" + input_file_full_name(config) + "'");
  }

  in >> source_lss;

  // Load
  Primitives::SnugComposites::LabelSet::OrderedIndexingTable lss;

  auto start = std::chrono::steady_clock::now();
  for (auto i = source_lss.begin(); i != source_lss.end(); ++i) {
    lss.emplace_back(*i);
  }
  auto now = std::chrono::steady_clock::now();

  metrics << (Metric() << "ordered_indexing_table_add_label_sets_one_by_one" << std::chrono::duration_cast<std::chrono::microseconds>(now - start).count());
}
