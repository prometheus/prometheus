#include <gtest/gtest.h>

#include "primitives/label_set.h"
#include "primitives/snug_composites.h"
#include "series_data/encoder.h"
#include "series_data/outdated_sample_encoder.h"
#include "series_index/queryable_encoding_bimap.h"
#include "series_index/trie/cedarpp_tree.h"
#include "status.h"

namespace {

using TrieIndex = series_index::TrieIndex<series_index::trie::CedarTrie, series_index::trie::CedarMatchesList>;
using QueryableEncodingBimap =
    series_index::QueryableEncodingBimap<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimapFilament, BareBones::Vector, TrieIndex>;
using head::StatusGetter;
using PromPP::Primitives::LabelViewSet;
using series_data::DataStorage;
using series_data::Encoder;
using series_data::OutdatedSampleEncoder;
using QuerySource = series_index::QueriedSeries::Source;

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
  EXPECT_EQ((Status{.min_max_timestamp = {.min = 1, .max = 1}, .chunk_count = 1}), status);
}

TEST_F(StatusFixture, QueriedSeries) {
  // Arrange
  lss_.find_or_emplace(LabelViewSet{{"job", "cron1"}});
  lss_.find_or_emplace(LabelViewSet{{"job", "cron2"}});
  lss_.find_or_emplace(LabelViewSet{{"job", "cron3"}});
  lss_.set_queried_series(QuerySource::kRule, std::vector{0U, 1U, 2U});
  lss_.set_queried_series(QuerySource::kFederate, std::vector{0U, 1U});
  lss_.set_queried_series(QuerySource::kOther, std::vector{0U});

  // Act
  const auto status = get_status();

  // Assert
  EXPECT_EQ(3U, status.rule_queried_series);
  EXPECT_EQ(2U, status.federate_queried_series);
  EXPECT_EQ(1U, status.other_queried_series);
}

}  // namespace