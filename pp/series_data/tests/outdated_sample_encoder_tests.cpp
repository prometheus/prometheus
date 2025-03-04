#include <gmock/gmock.h>

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

class SystemClockMock {
 public:
  using time_point = std::chrono::system_clock::time_point;

  MOCK_METHOD(time_point, now, ());
};

class OutdatedSampleEncoderFixture : public testing::Test {
 protected:
  using ExpectedSampleList = BareBones::Vector<Sample>;

  DataStorage storage_;
  testing::StrictMock<SystemClockMock> clock_;
  OutdatedSampleEncoder<decltype(clock_), 2> outdated_sample_encoder_{clock_};
  Encoder<decltype(outdated_sample_encoder_)> encoder_{storage_, outdated_sample_encoder_};
};

TEST_F(OutdatedSampleEncoderFixture, NoMergeAtEncode) {
  // Arrange
  encoder_.encode(0, 5, 1.0);
  encoder_.encode(0, 6, 1.0);
  EXPECT_CALL(clock_, now()).WillOnce(testing::Return(std::chrono::system_clock::time_point{}));

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

TEST_F(OutdatedSampleEncoderFixture, MergeAtEncode) {
  // Arrange
  encoder_.encode(0, 5, 1.0);
  encoder_.encode(0, 6, 1.0);
  EXPECT_CALL(clock_, now()).WillOnce(testing::Return(std::chrono::system_clock::time_point{}));

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

TEST_F(OutdatedSampleEncoderFixture, NoMerge) {
  // Arrange
  encoder_.encode(0, 5, 1.0);
  encoder_.encode(0, 6, 1.0);

  // Act
  outdated_sample_encoder_.merge_outdated_chunks(encoder_, std::chrono::seconds{1});

  // Assert
}

TEST_F(OutdatedSampleEncoderFixture, MergeOneChunk) {
  // Arrange
  EXPECT_CALL(clock_, now())
      .WillOnce(testing::Return(std::chrono::system_clock::time_point{std::chrono::seconds{0}}))
      .WillOnce(testing::Return(std::chrono::system_clock::time_point{std::chrono::seconds{1}}))
      .WillOnce(testing::Return(std::chrono::system_clock::time_point{std::chrono::seconds{1}}));

  encoder_.encode(0, 5, 1.0);
  encoder_.encode(0, 6, 1.0);
  encoder_.encode(0, 0, 1.0);

  encoder_.encode(1, 5, 1.0);
  encoder_.encode(1, 6, 1.0);
  encoder_.encode(1, 0, 1.0);

  // Act
  outdated_sample_encoder_.merge_outdated_chunks(encoder_, std::chrono::seconds{1});

  // Assert
  EXPECT_TRUE(std::ranges::equal(
      ExpectedSampleList{
          {.timestamp = 0, .value = 1.0},
          {.timestamp = 5, .value = 1.0},
          {.timestamp = 6, .value = 1.0},
      },
      Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0])));
  ASSERT_FALSE(storage_.outdated_chunks.contains(0));
  EXPECT_TRUE(std::ranges::equal(
      ExpectedSampleList{
          {.timestamp = 5, .value = 1.0},
          {.timestamp = 6, .value = 1.0},
      },
      Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[1])));
  ASSERT_TRUE(storage_.outdated_chunks.contains(1));
}

}  // namespace