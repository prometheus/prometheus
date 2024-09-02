#include <gtest/gtest.h>

#include "series_data/decoder.h"
#include "series_data/encoder.h"
#include "series_data/outdated_chunk_merger.h"
#include "series_data/outdated_sample_encoder.h"

namespace {

using BareBones::BitSequenceReader;
using series_data::DataStorage;
using series_data::Decoder;
using series_data::Encoder;
using series_data::OutdatedChunkMerger;
using series_data::OutdatedSampleEncoder;
using series_data::chunk::DataChunk;
using series_data::chunk::FinalizedChunkList;
using series_data::encoder::BitSequenceWithItemsCount;
using series_data::encoder::Sample;
using series_data::encoder::SampleList;
using series_data::encoder::timestamp::TimestampDecoder;

template <uint8_t kSamplesPerChunkValue>
class OutdatedChunkMergerTrait {
 protected:
  struct EncodeSample {
    uint32_t ls_id{};
    Sample sample;
  };

  using ExpectedSampleList = BareBones::Vector<Sample>;
  using ExpectedListOfSampleList = BareBones::Vector<ExpectedSampleList>;

  DataStorage storage_;
  std::chrono::system_clock clock_;
  OutdatedSampleEncoder<std::chrono::system_clock> outdated_sample_encoder_{clock_};
  Encoder<decltype(outdated_sample_encoder_), kSamplesPerChunkValue> encoder_{storage_, outdated_sample_encoder_};
  OutdatedChunkMerger<decltype(encoder_)> merger_{encoder_};

  [[nodiscard]] const DataChunk& get_open_chunk(uint32_t ls_id) { return storage_.open_chunks[ls_id]; }

  [[nodiscard]] const FinalizedChunkList* get_finalized_chunks(uint32_t ls_id) const noexcept {
    if (auto it = storage_.finalized_chunks.find(ls_id); it != storage_.finalized_chunks.end()) {
      return &it->second;
    }

    return nullptr;
  }

  void encode(std::initializer_list<EncodeSample> samples) {
    for (auto& sample : samples) {
      encoder_.encode(sample.ls_id, sample.sample.timestamp, sample.sample.value);
    }
  }

  PROMPP_ALWAYS_INLINE void encode_outdated(std::initializer_list<EncodeSample> samples) { encode(samples); }
};

class OutdatedChunkMergerInOpenConstantChunkFixture : public OutdatedChunkMergerTrait<series_data::kSamplesPerChunkDefault>,
                                                      public testing::TestWithParam<double> {};

TEST_P(OutdatedChunkMergerInOpenConstantChunkFixture, Test) {
  // Arrange
  encode({
      {.ls_id = 0, .sample = {.timestamp = 1, .value = GetParam()}},
      {.ls_id = 0, .sample = {.timestamp = 3, .value = GetParam()}},
      {.ls_id = 0, .sample = {.timestamp = 2, .value = GetParam()}},
  });
  encode_outdated({
      {.ls_id = 0, .sample = {.timestamp = 0, .value = GetParam()}},
      {.ls_id = 0, .sample = {.timestamp = 0, .value = GetParam()}},
  });

  // Act
  merger_.merge();

  // Assert
  EXPECT_TRUE(std::ranges::equal(
      ExpectedSampleList{
          {.timestamp = 0, .value = GetParam()},
          {.timestamp = 1, .value = GetParam()},
          {.timestamp = 2, .value = GetParam()},
          {.timestamp = 3, .value = GetParam()},
      },
      Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, get_open_chunk(0))));
}

INSTANTIATE_TEST_SUITE_P(Uint32ConstantChunk, OutdatedChunkMergerInOpenConstantChunkFixture, testing::Values(1.0));
INSTANTIATE_TEST_SUITE_P(DoubleConstantChunk, OutdatedChunkMergerInOpenConstantChunkFixture, testing::Values(1.1));

class OutdatedChunkMergerInOpenChunkFixture : public OutdatedChunkMergerTrait<series_data::kSamplesPerChunkDefault>, public testing::Test {};

TEST_F(OutdatedChunkMergerInOpenChunkFixture, MergeInTwoDoubleConstantsChunk) {
  // Arrange
  encode({
      {.ls_id = 0, .sample = {.timestamp = 1, .value = 1.0}},
      {.ls_id = 0, .sample = {.timestamp = 2, .value = 1.1}},
      {.ls_id = 0, .sample = {.timestamp = 3, .value = 1.1}},
  });
  encode_outdated({
      {.ls_id = 0, .sample = {.timestamp = 0, .value = 1.0}},
      {.ls_id = 0, .sample = {.timestamp = 0, .value = 1.0}},
  });

  // Act
  merger_.merge();

  // Assert
  EXPECT_TRUE(std::ranges::equal(
      ExpectedSampleList{
          {.timestamp = 0, .value = 1.0},
          {.timestamp = 1, .value = 1.0},
          {.timestamp = 2, .value = 1.1},
          {.timestamp = 3, .value = 1.1},
      },
      Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, get_open_chunk(0))));
}

TEST_F(OutdatedChunkMergerInOpenChunkFixture, MergeInAscIntegerValuesGorillaChunk) {
  // Arrange
  encode({
      {.ls_id = 0, .sample = {.timestamp = 1, .value = 1.0}},
      {.ls_id = 0, .sample = {.timestamp = 2, .value = 2.0}},
      {.ls_id = 0, .sample = {.timestamp = 3, .value = 3.0}},
  });
  encode_outdated({
      {.ls_id = 0, .sample = {.timestamp = 0, .value = 0.0}},
      {.ls_id = 0, .sample = {.timestamp = 0, .value = 0.0}},
  });

  // Act
  merger_.merge();

  // Assert
  EXPECT_TRUE(std::ranges::equal(
      ExpectedSampleList{
          {.timestamp = 0, .value = 0.0},
          {.timestamp = 1, .value = 1.0},
          {.timestamp = 2, .value = 2.0},
          {.timestamp = 3, .value = 3.0},
      },
      Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, get_open_chunk(0))));
}

TEST_F(OutdatedChunkMergerInOpenChunkFixture, MergeInValuesGorillaChunk) {
  // Arrange
  encode({
      {.ls_id = 0, .sample = {.timestamp = 1, .value = 1.1}},
      {.ls_id = 1, .sample = {.timestamp = 1, .value = 1.1}},
      {.ls_id = 0, .sample = {.timestamp = 2, .value = 2.1}},
      {.ls_id = 0, .sample = {.timestamp = 3, .value = 3.1}},
  });
  encode_outdated({
      {.ls_id = 0, .sample = {.timestamp = 0, .value = 0.0}},
      {.ls_id = 0, .sample = {.timestamp = 0, .value = 0.0}},
  });

  // Act
  merger_.merge();

  // Assert
  EXPECT_TRUE(std::ranges::equal(
      ExpectedSampleList{
          {.timestamp = 0, .value = 0.0},
          {.timestamp = 1, .value = 1.1},
          {.timestamp = 2, .value = 2.1},
          {.timestamp = 3, .value = 3.1},
      },
      Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, get_open_chunk(0))));
}

TEST_F(OutdatedChunkMergerInOpenChunkFixture, MergeInGorillaChunk) {
  // Arrange
  encode({
      {.ls_id = 0, .sample = {.timestamp = 1, .value = 1.1}},
      {.ls_id = 0, .sample = {.timestamp = 2, .value = 2.1}},
      {.ls_id = 0, .sample = {.timestamp = 3, .value = 3.1}},
  });
  encode_outdated({
      {.ls_id = 0, .sample = {.timestamp = 0, .value = 0.0}},
      {.ls_id = 0, .sample = {.timestamp = 0, .value = 0.0}},
  });

  // Act
  merger_.merge();

  // Assert
  EXPECT_TRUE(std::ranges::equal(
      ExpectedSampleList{
          {.timestamp = 0, .value = 0.0},
          {.timestamp = 1, .value = 1.1},
          {.timestamp = 2, .value = 2.1},
          {.timestamp = 3, .value = 3.1},
      },
      Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, get_open_chunk(0))));
}

class OutdatedChunkMergerInFinalizedConstantChunkFixture : public OutdatedChunkMergerTrait<4>, public testing::TestWithParam<double> {};

TEST_P(OutdatedChunkMergerInFinalizedConstantChunkFixture, Test) {
  // Arrange
  encode({{.ls_id = 0, .sample = {.timestamp = 1, .value = GetParam()}},
          {.ls_id = 0, .sample = {.timestamp = 3, .value = GetParam()}},
          {.ls_id = 0, .sample = {.timestamp = 5, .value = GetParam()}},
          {.ls_id = 0, .sample = {.timestamp = 7, .value = GetParam()}},
          {.ls_id = 0, .sample = {.timestamp = 9, .value = GetParam()}},
          {.ls_id = 0, .sample = {.timestamp = 11, .value = GetParam()}},
          {.ls_id = 0, .sample = {.timestamp = 13, .value = GetParam()}},
          {.ls_id = 0, .sample = {.timestamp = 15, .value = GetParam()}},
          {.ls_id = 0, .sample = {.timestamp = 17, .value = GetParam()}}});
  encode_outdated({
      {.ls_id = 0, .sample = {.timestamp = 0, .value = GetParam()}},
      {.ls_id = 0, .sample = {.timestamp = 8, .value = GetParam()}},
      {.ls_id = 0, .sample = {.timestamp = 5, .value = GetParam()}},
      {.ls_id = 0, .sample = {.timestamp = 9, .value = GetParam()}},
  });

  // Act
  merger_.merge();

  // Assert
  auto finalized = get_finalized_chunks(0);
  ASSERT_NE(nullptr, finalized);
  EXPECT_TRUE(std::ranges::equal(
      ExpectedListOfSampleList{
          {
              {.timestamp = 0, .value = GetParam()},
              {.timestamp = 1, .value = GetParam()},
              {.timestamp = 3, .value = GetParam()},
              {.timestamp = 5, .value = GetParam()},
          },
          {
              {.timestamp = 7, .value = GetParam()},
              {.timestamp = 8, .value = GetParam()},
          },
          {
              {.timestamp = 9, .value = GetParam()},
              {.timestamp = 11, .value = GetParam()},
              {.timestamp = 13, .value = GetParam()},
              {.timestamp = 15, .value = GetParam()},
          },
          {
              {.timestamp = 17, .value = GetParam()},
          },
      },
      Decoder::decode_chunks(storage_, *finalized, get_open_chunk(0))));
}

INSTANTIATE_TEST_SUITE_P(Uint32ConstantChunk, OutdatedChunkMergerInFinalizedConstantChunkFixture, testing::Values(1.0));
INSTANTIATE_TEST_SUITE_P(DoubleConstantChunk, OutdatedChunkMergerInFinalizedConstantChunkFixture, testing::Values(1.1));

class OutdatedChunkMergerInFinalizedChunkFixture : public OutdatedChunkMergerTrait<4>, public testing::Test {};

TEST_F(OutdatedChunkMergerInFinalizedChunkFixture, MergeInTwoDoubleConstantsChunk) {
  // Arrange
  encode({{.ls_id = 0, .sample = {.timestamp = 1, .value = 1.0}},
          {.ls_id = 0, .sample = {.timestamp = 3, .value = 1.1}},
          {.ls_id = 0, .sample = {.timestamp = 5, .value = 1.1}},
          {.ls_id = 0, .sample = {.timestamp = 7, .value = 1.1}},
          {.ls_id = 0, .sample = {.timestamp = 9, .value = 1.1}},
          {.ls_id = 0, .sample = {.timestamp = 11, .value = 1.2}}});
  encode_outdated({
      {.ls_id = 0, .sample = {.timestamp = 0, .value = 1.0}},
      {.ls_id = 0, .sample = {.timestamp = 8, .value = 1.1}},
      {.ls_id = 0, .sample = {.timestamp = 5, .value = 1.1}},
      {.ls_id = 0, .sample = {.timestamp = 9, .value = 1.2}},
  });

  // Act
  merger_.merge();

  // Assert
  auto finalized = get_finalized_chunks(0);
  ASSERT_NE(nullptr, finalized);
  EXPECT_TRUE(std::ranges::equal(
      ExpectedListOfSampleList{
          {
              {.timestamp = 0, .value = 1.0},
              {.timestamp = 1, .value = 1.0},
              {.timestamp = 3, .value = 1.1},
              {.timestamp = 5, .value = 1.1},
          },
          {
              {.timestamp = 7, .value = 1.1},
              {.timestamp = 8, .value = 1.1},
          },
          {
              {.timestamp = 9, .value = 1.2},
              {.timestamp = 11, .value = 1.2},
          },
      },
      Decoder::decode_chunks(storage_, *finalized, get_open_chunk(0))));
}

TEST_F(OutdatedChunkMergerInFinalizedChunkFixture, MergeInAscIntegerValuesGorillaChunk) {
  // Arrange
  encode({{.ls_id = 0, .sample = {.timestamp = 1, .value = 1.0}},
          {.ls_id = 0, .sample = {.timestamp = 3, .value = 3.0}},
          {.ls_id = 0, .sample = {.timestamp = 5, .value = 5.0}},
          {.ls_id = 0, .sample = {.timestamp = 7, .value = 7.0}},
          {.ls_id = 0, .sample = {.timestamp = 9, .value = 9.0}},
          {.ls_id = 0, .sample = {.timestamp = 11, .value = 11.0}}});
  encode_outdated({
      {.ls_id = 0, .sample = {.timestamp = 0, .value = 0.0}},
      {.ls_id = 0, .sample = {.timestamp = 8, .value = 8.0}},
      {.ls_id = 0, .sample = {.timestamp = 5, .value = 5.0}},
      {.ls_id = 0, .sample = {.timestamp = 9, .value = 9.0}},
  });

  // Act
  merger_.merge();

  // Assert
  auto finalized = get_finalized_chunks(0);
  ASSERT_NE(nullptr, finalized);
  EXPECT_TRUE(std::ranges::equal(
      ExpectedListOfSampleList{
          {
              {.timestamp = 0, .value = 0.0},
              {.timestamp = 1, .value = 1.0},
              {.timestamp = 3, .value = 3.0},
              {.timestamp = 5, .value = 5.0},
          },
          {
              {.timestamp = 7, .value = 7.0},
              {.timestamp = 8, .value = 8.0},
          },
          {
              {.timestamp = 9, .value = 9.0},
              {.timestamp = 11, .value = 11.0},
          },
      },
      Decoder::decode_chunks(storage_, *finalized, get_open_chunk(0))));
}

TEST_F(OutdatedChunkMergerInFinalizedChunkFixture, MergeInValuesGorillaChunk) {
  // Arrange
  encode({
      {.ls_id = 0, .sample = {.timestamp = 1, .value = 1.1}},
      {.ls_id = 1, .sample = {.timestamp = 1, .value = 1.1}},
      {.ls_id = 0, .sample = {.timestamp = 2, .value = 2.1}},
      {.ls_id = 1, .sample = {.timestamp = 2, .value = 2.1}},
      {.ls_id = 0, .sample = {.timestamp = 3, .value = 3.1}},
      {.ls_id = 1, .sample = {.timestamp = 3, .value = 3.1}},
      {.ls_id = 0, .sample = {.timestamp = 4, .value = 4.1}},
      {.ls_id = 1, .sample = {.timestamp = 4, .value = 4.1}},
      {.ls_id = 0, .sample = {.timestamp = 5, .value = 5.1}},
  });
  encode_outdated({
      {.ls_id = 0, .sample = {.timestamp = 0, .value = 0.0}},
      {.ls_id = 0, .sample = {.timestamp = 0, .value = 0.0}},
  });

  // Act
  merger_.merge();

  // Assert
  auto finalized = get_finalized_chunks(0);
  ASSERT_NE(nullptr, finalized);
  EXPECT_TRUE(std::ranges::equal(
      ExpectedListOfSampleList{
          {
              {.timestamp = 0, .value = 0.0},
              {.timestamp = 1, .value = 1.1},
              {.timestamp = 2, .value = 2.1},
              {.timestamp = 3, .value = 3.1},
          },
          {
              {.timestamp = 4, .value = 4.1},
          },
          {
              {.timestamp = 5, .value = 5.1},
          },
      },
      Decoder::decode_chunks(storage_, *finalized, get_open_chunk(0))));
}

TEST_F(OutdatedChunkMergerInFinalizedChunkFixture, MergeInGorillaChunk) {
  // Arrange
  encode({
      {.ls_id = 0, .sample = {.timestamp = 1, .value = 1.1}},
      {.ls_id = 0, .sample = {.timestamp = 2, .value = 2.1}},
      {.ls_id = 0, .sample = {.timestamp = 3, .value = 3.1}},
      {.ls_id = 0, .sample = {.timestamp = 4, .value = 4.1}},
      {.ls_id = 0, .sample = {.timestamp = 5, .value = 5.1}},
  });
  encode_outdated({
      {.ls_id = 0, .sample = {.timestamp = 0, .value = 0.0}},
      {.ls_id = 0, .sample = {.timestamp = 0, .value = 0.0}},
  });

  // Act
  merger_.merge();

  // Assert
  auto finalized = get_finalized_chunks(0);
  ASSERT_NE(nullptr, finalized);
  EXPECT_TRUE(std::ranges::equal(
      ExpectedListOfSampleList{
          {
              {.timestamp = 0, .value = 0.0},
              {.timestamp = 1, .value = 1.1},
              {.timestamp = 2, .value = 2.1},
              {.timestamp = 3, .value = 3.1},
          },
          {
              {.timestamp = 4, .value = 4.1},
          },
          {
              {.timestamp = 5, .value = 5.1},
          },
      },
      Decoder::decode_chunks(storage_, *finalized, get_open_chunk(0))));
}

class OutdatedChunkMergerFixture : public OutdatedChunkMergerTrait<series_data::kSamplesPerChunkDefault>, public testing::Test {};

TEST_F(OutdatedChunkMergerFixture, UseLastValueForSameTimestampInOutdatedChunk) {
  // Arrange
  encode({
      {.ls_id = 0, .sample = {.timestamp = 1, .value = 1.1}},
      {.ls_id = 0, .sample = {.timestamp = 2, .value = 2.1}},
      {.ls_id = 0, .sample = {.timestamp = 3, .value = 3.1}},
  });
  encode_outdated({
      {.ls_id = 0, .sample = {.timestamp = 0, .value = 0.1}},
      {.ls_id = 0, .sample = {.timestamp = 0, .value = 0.2}},
      {.ls_id = 0, .sample = {.timestamp = 0, .value = 0.0}},
  });

  // Act
  merger_.merge();

  // Assert
  EXPECT_TRUE(std::ranges::equal(
      ExpectedSampleList{
          {.timestamp = 0, .value = 0.1},
          {.timestamp = 1, .value = 1.1},
          {.timestamp = 2, .value = 2.1},
          {.timestamp = 3, .value = 3.1},
      },
      Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, get_open_chunk(0))));
}

}  // namespace