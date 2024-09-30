#include <gtest/gtest.h>

#include "primitives/snug_composites.h"
#include "series_data/encoder.h"
#include "series_data/outdated_sample_encoder.h"
#include "series_index/queryable_encoding_bimap.h"
#include "series_index/trie/cedarpp_tree.h"
#include "status.h"

namespace {

using TrieIndex = series_index::TrieIndex<series_index::trie::CedarTrie, series_index::trie::CedarMatchesList>;
using QueryableEncodingBimap = series_index::QueryableEncodingBimap<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimapFilament, TrieIndex>;
using head::StatusGetter;
using series_data::DataStorage;
using series_data::Encoder;
using series_data::OutdatedSampleEncoder;

using Status = head::Status<std::string_view, std::vector>;

class StatusFixture : public ::testing::Test {
 protected:
  static constexpr size_t kTopItemsCount = 10;

  QueryableEncodingBimap lss_;
  DataStorage storage_;
  std::chrono::system_clock clock_;
  OutdatedSampleEncoder<std::chrono::system_clock> outdated_sample_encoder_{clock_};

  [[nodiscard]] Status get_status() const {
    Status status;
    StatusGetter<QueryableEncodingBimap, Status>{lss_, storage_, kTopItemsCount}.get(status);
    return status;
  }
};

TEST_F(StatusFixture, EmptyLssAndStorage) {
  // Arrange

  // Act
  const auto status = get_status();

  // Assert
  EXPECT_EQ(Status{}, status);
}

TEST_F(StatusFixture, FinalizedChunk) {
  // Arrange
  Encoder<decltype(outdated_sample_encoder_), 2> encoder{storage_, outdated_sample_encoder_};
  encoder.encode(0, 1, 1.0);
  encoder.encode(0, 2, 1.0);
  encoder.encode(0, 3, 1.0);

  // Act
  const auto status = get_status();

  // Assert
  EXPECT_EQ((Status{.min_max_timestamp = {.min = 1, .max = 3}, .chunk_count = 2}), status);
}

TEST_F(StatusFixture, FinalizedTimestreamChunk) {
  // Arrange
  Encoder<decltype(outdated_sample_encoder_), 2> encoder{storage_, outdated_sample_encoder_};
  encoder.encode(0, 1, 1.0);
  encoder.encode(1, 1, 1.0);
  encoder.encode(0, 2, 1.0);
  encoder.encode(1, 2, 1.0);
  encoder.encode(0, 3, 1.0);

  // Act
  const auto status = get_status();

  // Assert
  EXPECT_EQ((Status{.min_max_timestamp = {.min = 1, .max = 3}, .chunk_count = 3}), status);
}

TEST_F(StatusFixture, OpenedChunk) {
  // Arrange
  Encoder<decltype(outdated_sample_encoder_), 2> encoder{storage_, outdated_sample_encoder_};
  encoder.encode(0, 1, 1.0);
  encoder.encode(1, 2, 1.0);
  encoder.encode(2, 3, 1.0);

  // Act
  const auto status = get_status();

  // Assert
  EXPECT_EQ((Status{.min_max_timestamp = {.min = 1, .max = 3}, .chunk_count = 3}), status);
}

TEST_F(StatusFixture, EmptyChunk) {
  // Arrange
  Encoder<decltype(outdated_sample_encoder_), 2> encoder{storage_, outdated_sample_encoder_};
  encoder.encode(5, 1, 1.0);

  // Act
  const auto status = get_status();

  // Assert
  EXPECT_EQ((Status{.min_max_timestamp = {.min = 1, .max = 1}, .chunk_count = 6}), status);
}

}  // namespace