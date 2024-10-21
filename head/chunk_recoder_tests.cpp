#include <gtest/gtest.h>

#include "chunk_recoder.h"
#include "series_data/encoder.h"
#include "series_data/outdated_sample_encoder.h"

namespace {

using head::ChunkRecoder;
using PromPP::Primitives::TimeInterval;
using series_data::DataStorage;
using series_data::Encoder;
using series_data::OutdatedSampleEncoder;
using std::operator""s;

class ChunkRecoderFixture : public ::testing::Test {
 protected:
  struct RecodeInfo {
    TimeInterval interval{.min = 0, .max = 0};
    uint32_t series_id{ChunkRecoder::kInvalidSeriesId};
    uint8_t samples_count{};
    std::string buffer;
    bool has_more_data{};

    bool operator==(const RecodeInfo&) const noexcept = default;
  };

  DataStorage storage_;
  std::chrono::system_clock clock_;
  OutdatedSampleEncoder<std::chrono::system_clock> outdated_sample_encoder_{clock_};

  static RecodeInfo recode(ChunkRecoder& recoder) noexcept {
    RecodeInfo info;
    recoder.recode_next_chunk(info);
    info.has_more_data = recoder.has_more_data();
    info.buffer = std::string{recoder.bytes().begin(), recoder.bytes().end()};
    return info;
  }
};

TEST_F(ChunkRecoderFixture, EmptyStorage) {
  // Arrange
  ChunkRecoder recoder(&storage_, {});

  // Act
  const auto info1 = recode(recoder);
  const auto info2 = recode(recoder);

  // Assert
  EXPECT_EQ(RecodeInfo{}, info1);
  EXPECT_EQ(RecodeInfo{}, info2);
}

TEST_F(ChunkRecoderFixture, StorageWithOneChunk) {
  // Arrange
  Encoder encoder{storage_, outdated_sample_encoder_};
  encoder.encode(0, 1, 1.0);
  encoder.encode(0, 2, 1.0);
  ChunkRecoder recoder(&storage_, {.min = 0, .max = 3});

  // Act
  const auto info1 = recode(recoder);
  const auto info2 = recode(recoder);

  // Assert
  EXPECT_EQ((RecodeInfo{
                .interval = {.min = 1, .max = 2},
                .series_id = 0,
                .samples_count = 2,
                .buffer = "\x00\x02\x02\x3f\xf0\x00\x00\x00\x00\x00\x00\x01\x00"s,
                .has_more_data = false,
            }),
            info1);
  EXPECT_EQ(RecodeInfo{}, info2);
}

TEST_F(ChunkRecoderFixture, StorageWithEmptyChunks) {
  // Arrange
  Encoder encoder{storage_, outdated_sample_encoder_};
  encoder.encode(2, 1, 1.0);
  encoder.encode(2, 2, 1.0);
  encoder.encode(4, 3, 2.0);
  encoder.encode(4, 4, 2.0);
  ChunkRecoder recoder(&storage_, {.min = 0, .max = 5});

  // Act
  const auto info1 = recode(recoder);
  const auto info2 = recode(recoder);

  // Assert
  EXPECT_EQ((RecodeInfo{
                .interval = {.min = 1, .max = 2},
                .series_id = 2,
                .samples_count = 2,
                .buffer = "\x00\x02\x02\x3f\xf0\x00\x00\x00\x00\x00\x00\x01\x00"s,
                .has_more_data = true,
            }),
            info1);
  EXPECT_EQ((RecodeInfo{
                .interval = {.min = 3, .max = 4},
                .series_id = 4,
                .samples_count = 2,
                .buffer = "\x00\x02\x06\x40\x00\x00\x00\x00\x00\x00\x00\x01\x00"s,
                .has_more_data = false,
            }),
            info2);
}

TEST_F(ChunkRecoderFixture, ChunkWithFinalizedTimestampStream) {
  // Arrange
  Encoder<decltype(outdated_sample_encoder_), 2> encoder{storage_, outdated_sample_encoder_};
  encoder.encode(0, 1, 1.0);
  encoder.encode(1, 1, 1.0);
  encoder.encode(0, 2, 1.0);
  encoder.encode(1, 2, 1.0);
  encoder.encode(1, 3, 1.0);
  ChunkRecoder recoder(&storage_, {.min = 0, .max = 4});

  // Act
  const auto info1 = recode(recoder);
  const auto info2 = recode(recoder);
  const auto info3 = recode(recoder);

  // Assert
  EXPECT_EQ((RecodeInfo{
                .interval = {.min = 1, .max = 2},
                .series_id = 0,
                .samples_count = 2,
                .buffer = "\x00\x02\x02\x3f\xf0\x00\x00\x00\x00\x00\x00\x01\x00"s,
                .has_more_data = true,
            }),
            info1);
  EXPECT_EQ((RecodeInfo{
                .interval = {.min = 1, .max = 2},
                .series_id = 1,
                .samples_count = 2,
                .buffer = "\x00\x02\x02\x3f\xf0\x00\x00\x00\x00\x00\x00\x01\x00"s,
                .has_more_data = true,
            }),
            info2);
  EXPECT_EQ((RecodeInfo{
                .interval = {.min = 3, .max = 3},
                .series_id = 1,
                .samples_count = 1,
                .buffer = "\x00\x01\x06\x3f\xf0\x00\x00\x00\x00\x00\x00"s,
                .has_more_data = false,
            }),
            info3);
}

TEST_F(ChunkRecoderFixture, GorillaChunk) {
  // Arrange
  Encoder encoder{storage_, outdated_sample_encoder_};
  encoder.encode(0, 1, 1.1);
  encoder.encode(0, 2, 1.2);
  encoder.encode(0, 3, 1.3);
  encoder.encode(0, 4, 1.4);
  ChunkRecoder recoder(&storage_, {.min = 0, .max = 5});

  // Act
  const auto info = recode(recoder);

  // Assert
  EXPECT_EQ((RecodeInfo{
                .interval = {.min = 1, .max = 4},
                .series_id = 0,
                .samples_count = 4,
                .buffer = "\x00\x04\x02\x3f\xf1\x99\x99\x99\x99\x99\x9a\x01\xdd\x95\x55\x55\x55\x55\x55\x52\xdb\x97\xff\xff\xff\xff\xff\xfe\xdd\x95\x55\x55\x55"
                          "\x55\x55\x56"s,
                .has_more_data = false,
            }),
            info);
}

TEST_F(ChunkRecoderFixture, NoChunksByTimeInterval) {
  // Arrange
  Encoder encoder{storage_, outdated_sample_encoder_};
  encoder.encode(0, 1, 1.1);
  encoder.encode(0, 2, 1.2);
  ChunkRecoder recoder(&storage_, {.min = 0, .max = 1});

  // Act
  const auto info = recode(recoder);

  // Assert
  EXPECT_EQ(RecodeInfo{}, info);
}

TEST_F(ChunkRecoderFixture, PartialReencodingByTimeInterval) {
  // Arrange
  Encoder encoder{storage_, outdated_sample_encoder_};
  encoder.encode(0, 0, 1.0);
  encoder.encode(0, 1, 1.0);
  encoder.encode(0, 2, 1.0);
  encoder.encode(0, 3, 1.0);
  encoder.encode(0, 4, 1.0);
  encoder.encode(1, 0, 1.0);
  ChunkRecoder recoder(&storage_, {.min = 1, .max = 3});

  // Act
  const auto info = recode(recoder);

  // Assert
  EXPECT_EQ((RecodeInfo{
                .interval = {.min = 1, .max = 2},
                .series_id = 0,
                .samples_count = 2,
                .buffer = "\x00\x02\x02\x3f\xf0\x00\x00\x00\x00\x00\x00\x01\x00"s,
                .has_more_data = false,
            }),
            info);
}

TEST_F(ChunkRecoderFixture, EmptyFinalizedChunk) {
  // Arrange
  Encoder<decltype(outdated_sample_encoder_), 2> encoder{storage_, outdated_sample_encoder_};
  encoder.encode(0, 1, 1.0);
  encoder.encode(0, 2, 1.0);
  encoder.encode(0, 5, 1.0);
  ChunkRecoder recoder(&storage_, {.min = 3, .max = 4});

  // Act
  const auto info = recode(recoder);

  // Assert
  EXPECT_EQ(RecodeInfo{}, info);
}

TEST_F(ChunkRecoderFixture, EmptyFinalizedChunkNonEmptyOpenedChunk) {
  // Arrange
  Encoder<decltype(outdated_sample_encoder_), 2> encoder{storage_, outdated_sample_encoder_};
  encoder.encode(0, 1, 1.0);
  encoder.encode(0, 2, 1.0);
  encoder.encode(0, 5, 1.0);
  ChunkRecoder recoder(&storage_, {.min = 3, .max = 6});

  // Act
  const auto info = recode(recoder);

  // Assert
  EXPECT_EQ((RecodeInfo{
                .interval = {.min = 5, .max = 5},
                .series_id = 0,
                .samples_count = 1,
                .buffer = "\x00\x01\x0a\x3f\xf0\x00\x00\x00\x00\x00\x00"s,
                .has_more_data = false,
            }),
            info);
}

}  // namespace