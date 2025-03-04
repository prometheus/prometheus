#include <filesystem>
#include <iostream>

#include <gtest/gtest.h>

#include "bare_bones/lz4_stream.h"
#include "configuration.h"
#include "primitives/snug_composites.h"
#include "test_file_name_suffix.h"

using namespace PromPP;  // NOLINT

namespace {

struct SnugComposite : public testing::Test {
  std::ifstream underlying_stream_for_lss_wal_lz4_stream;
  BareBones::LZ4Stream::istream lss_wal_lz4_stream{&underlying_stream_for_lss_wal_lz4_stream};
  std::ifstream underlying_stream_for_lss_full_lz4_stream;
  BareBones::LZ4Stream::istream lss_full_lz4_stream{&underlying_stream_for_lss_full_lz4_stream};
  Primitives::SnugComposites::LabelSet::DecodingTable<BareBones::Vector> expected_result;

  void SetUp() final {
    std::string lss_wal_file_full_name =
        Configuration::get_path_to_test_data() + "/" + "lss_wal" + "." + test_file_name_suffix(Configuration::get_input_data_ordering());
    if (!std::filesystem::exists(lss_wal_file_full_name)) {
      throw std::runtime_error("file '" + lss_wal_file_full_name + "', needed for tests SnugComposite, does not exist");
    }

    underlying_stream_for_lss_wal_lz4_stream.open(lss_wal_file_full_name, std::ios_base::binary);
    if (!underlying_stream_for_lss_wal_lz4_stream.is_open()) {
      throw std::runtime_error("An attempt to open file '" + lss_wal_file_full_name + "', needed for tests SnugComposite, failed");
    }

    std::string lss_full_file_full_name =
        Configuration::get_path_to_test_data() + "/" + "lss_full" + "." + test_file_name_suffix(Configuration::get_input_data_ordering());
    if (!std::filesystem::exists(lss_full_file_full_name)) {
      throw std::runtime_error("file '" + lss_full_file_full_name + "', needed for tests SnugComposite, does not exist");
    }

    underlying_stream_for_lss_full_lz4_stream.open(lss_full_file_full_name, std::ios_base::binary);
    if (!underlying_stream_for_lss_wal_lz4_stream.is_open()) {
      throw std::runtime_error("An attempt to open file '" + lss_full_file_full_name + "', needed for tests SnugComposite, failed");
    }

    while (!lss_wal_lz4_stream.eof()) {
      lss_wal_lz4_stream >> expected_result;
    }
  }
};

TEST_F(SnugComposite, DecodingTable) {
  Primitives::SnugComposites::LabelSet::DecodingTable<BareBones::Vector> actual_result;
  lss_full_lz4_stream >> actual_result;

  EXPECT_TRUE(std::ranges::equal(expected_result.begin(), expected_result.end(), actual_result.begin(), actual_result.end()));
}

TEST_F(SnugComposite, ParallelEncodingBimap) {
  Primitives::SnugComposites::LabelSet::ParallelEncodingBimap<BareBones::Vector> actual_result;
  lss_full_lz4_stream >> actual_result;

  EXPECT_TRUE(std::ranges::equal(expected_result.begin(), expected_result.end(), actual_result.begin(), actual_result.end(), std::equal_to()));
}

TEST_F(SnugComposite, EncodingBimap) {
  Primitives::SnugComposites::LabelSet::EncodingBimap<BareBones::Vector> actual_result;
  lss_full_lz4_stream >> actual_result;

  EXPECT_TRUE(std::ranges::equal(expected_result.begin(), expected_result.end(), actual_result.begin(), actual_result.end(), std::equal_to()));
}

TEST_F(SnugComposite, OrderedEncodingBimap) {
  Primitives::SnugComposites::LabelSet::OrderedEncodingBimap<BareBones::Vector> actual_result;
  lss_full_lz4_stream >> actual_result;

  EXPECT_TRUE(
      std::ranges::equal(expected_result.begin(), expected_result.end(), actual_result.unordered_begin(), actual_result.unordered_end(), std::equal_to()));
}

TEST_F(SnugComposite, OrderedIndexingTable) {
  Primitives::SnugComposites::LabelSet::OrderedIndexingTable<BareBones::Vector> actual_result;
  lss_full_lz4_stream >> actual_result;

  EXPECT_TRUE(std::ranges::equal(expected_result.begin(), expected_result.end(), actual_result.begin(), actual_result.end(), std::equal_to()));
}

}  // namespace
