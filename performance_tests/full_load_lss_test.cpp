#include "full_load_lss_test.h"

#include <chrono>

#include "bare_bones/lz4_stream.h"
#include "primitives/snug_composites.h"

using namespace PromPP;  // NOLINT

void full_load_lss::execute(const Config& config, Metrics& metrics) const {
  // test load time: DecodingTable
  {
    std::ifstream infile(input_file_full_name(config), std::ios_base::binary);
    BareBones::LZ4Stream::istream in(&infile);
    if (!infile.is_open()) {
      throw std::runtime_error("failed to open file '" + input_file_full_name(config) + "'");
    }

    Primitives::SnugComposites::LabelSet::DecodingTable<BareBones::Vector> lss;

    auto start = std::chrono::steady_clock::now();
    in >> lss;
    auto now = std::chrono::steady_clock::now();

    metrics << (Metric() << "decoding_table_load_from_file_duration_microseconds"
                         << (std::chrono::duration_cast<std::chrono::microseconds>(now - start).count()));
  }

  // test load time: ParallelEncodingBimap
  {
    std::ifstream infile(input_file_full_name(config), std::ios_base::binary);
    BareBones::LZ4Stream::istream in(&infile);
    if (!infile.is_open()) {
      throw std::runtime_error("failed to open file '" + input_file_full_name(config) + "'");
    }

    Primitives::SnugComposites::LabelSet::ParallelEncodingBimap<BareBones::Vector> lss;

    auto start = std::chrono::steady_clock::now();
    in >> lss;
    auto now = std::chrono::steady_clock::now();

    metrics << (Metric() << "parallel_encoding_bimap_load_from_file_duration_microseconds"
                         << (std::chrono::duration_cast<std::chrono::microseconds>(now - start).count()));
  }

  // test load time: EncodingBimap
  {
    std::ifstream infile(input_file_full_name(config), std::ios_base::binary);
    BareBones::LZ4Stream::istream in(&infile);
    if (!infile.is_open()) {
      throw std::runtime_error("failed to open file '" + input_file_full_name(config) + "'");
    }

    Primitives::SnugComposites::LabelSet::EncodingBimap<BareBones::Vector> lss;

    auto start = std::chrono::steady_clock::now();
    in >> lss;
    auto now = std::chrono::steady_clock::now();

    metrics << (Metric() << "encoding_bimap_load_from_file_duration_microseconds"
                         << (std::chrono::duration_cast<std::chrono::microseconds>(now - start).count()));
  }

  // test load time: OrderedEncodingBimap
  {
    std::ifstream infile(input_file_full_name(config), std::ios_base::binary);
    if (!infile.is_open()) {
      throw std::runtime_error("failed to open file '" + input_file_full_name(config) + "'");
    }

    Primitives::SnugComposites::LabelSet::OrderedEncodingBimap<BareBones::Vector> lss;

    BareBones::LZ4Stream::istream in(&infile);
    auto start = std::chrono::steady_clock::now();
    in >> lss;
    auto now = std::chrono::steady_clock::now();

    metrics << (Metric() << "ordered_encoding_bimap_load_from_file_duration_microseconds"
                         << (std::chrono::duration_cast<std::chrono::microseconds>(now - start).count()));
  }

  // test load time: OrderedIndexingTable
  {
    std::ifstream infile(input_file_full_name(config), std::ios_base::binary);
    if (!infile.is_open()) {
      throw std::runtime_error("failed to open file '" + input_file_full_name(config) + "'");
    }

    Primitives::SnugComposites::LabelSet::OrderedIndexingTable<BareBones::Vector> lss;

    BareBones::LZ4Stream::istream in(&infile);
    auto start = std::chrono::steady_clock::now();
    in >> lss;
    auto now = std::chrono::steady_clock::now();

    metrics << (Metric() << "ordered_indexing_table_load_from_file_duration_microseconds"
                         << (std::chrono::duration_cast<std::chrono::microseconds>(now - start).count()));
  }
}
