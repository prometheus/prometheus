#include <gtest/gtest.h>

#include "series_data/decoder.h"
#include "series_data/encoder.h"
#include "series_data/outdated_sample_encoder.h"

namespace {

using series_data::DataStorage;
using series_data::Decoder;
using series_data::Encoder;
using series_data::OutdatedSampleEncoder;
using series_data::chunk::DataChunk;
using series_data::encoder::Sample;

class OutdatedSampleEncoderFixture : public testing::Test {
 protected:
  using ExpectedSampleList = BareBones::Vector<Sample>;

  DataStorage storage_;
  OutdatedSampleEncoder<2> outdated_sample_encoder_{storage_};
  Encoder<decltype(outdated_sample_encoder_)> encoder_{storage_, outdated_sample_encoder_};
};

TEST_F(OutdatedSampleEncoderFixture, NoMerge) {
  // Arrange
  encoder_.encode(0, 5, 1.0);
  encoder_.encode(0, 6, 1.0);

  // Act
  encoder_.encode(0, 0, 1.0);

  // Assert
  EXPECT_TRUE(std::ranges::equal(
      ExpectedSampleList{
          {.timestamp = 5, .value = 1.0},
          {.timestamp = 6, .value = 1.0},
      },
      Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0])));
  ASSERT_TRUE(storage_.outdated_chunks.contains(0));
  EXPECT_TRUE(std::ranges::equal(
      ExpectedSampleList{
          {.timestamp = 0, .value = 1.0},
      },
      Decoder::decode_gorilla_chunk(storage_.outdated_chunks.find(0)->second.stream().stream)));
}

TEST_F(OutdatedSampleEncoderFixture, Merge) {
  // Arrange
  encoder_.encode(0, 5, 1.0);
  encoder_.encode(0, 6, 1.0);

  // Act
  encoder_.encode(0, 0, 1.0);
  encoder_.encode(0, 1, 1.0);

  // Assert
  EXPECT_TRUE(std::ranges::equal(
      ExpectedSampleList{
          {.timestamp = 0, .value = 1.0},
          {.timestamp = 1, .value = 1.0},
          {.timestamp = 5, .value = 1.0},
          {.timestamp = 6, .value = 1.0},
      },
      Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0])));
  ASSERT_FALSE(storage_.outdated_chunks.contains(0));
}

}  // namespace