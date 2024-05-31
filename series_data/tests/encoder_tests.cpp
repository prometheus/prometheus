#include <numeric>

#include <gtest/gtest.h>

#include "series_data/encoder.h"

namespace {

using BareBones::BitSequenceReader;
using BareBones::Encoding::Gorilla::STALE_NAN;
using series_data::DataStorage;
using series_data::Encoder;
using series_data::chunk::DataChunk;
using series_data::chunk::FinalizedChunkList;
using series_data::chunk::OutdatedChunk;
using series_data::encoder::BitSequenceWithItemsCount;
using series_data::encoder::GorillaDecoder;
using series_data::encoder::GorillaEncoder;
using series_data::encoder::timestamp::TimestampDecoder;
using series_data::encoder::value::AscIntegerValuesGorillaDecoder;
using series_data::encoder::value::TwoDoubleConstantEncoder;
using series_data::encoder::value::ValuesGorillaDecoder;

template <uint8_t kSamplesPerChunkValue>
class EncoderTestTrait {
 protected:
  static constexpr auto kSamplesPerChunk = kSamplesPerChunkValue;

  Encoder<kSamplesPerChunk> encoder_;

  [[nodiscard]] const DataChunk& chunk(uint32_t ls_id) const noexcept { return encoder_.storage().open_chunks[ls_id]; }
  [[nodiscard]] const FinalizedChunkList* finalized_chunks(uint32_t ls_id) const noexcept {
    if (auto it = encoder_.storage().finalized_chunks_.find(ls_id); it != encoder_.storage().finalized_chunks_.end()) {
      return &it->second;
    }

    return nullptr;
  }
  [[nodiscard]] const OutdatedChunk* outdated_chunk(uint32_t ls_id) const noexcept {
    if (auto it = encoder_.storage().outdated_chunks_.find(ls_id); it != encoder_.storage().outdated_chunks_.end()) {
      return &it->second;
    }

    return nullptr;
  }

  [[nodiscard]] const BitSequenceWithItemsCount& open_chunk_timestamp(uint32_t ls_id) const noexcept {
    return encoder_.storage().timestamp_encoder_.get_stream(encoder_.storage().open_chunks[ls_id].timestamp_encoder_state_id);
  }
  [[nodiscard]] BitSequenceReader open_chunk_timestamp_reader(uint32_t ls_id) const noexcept { return open_chunk_timestamp(ls_id).reader(); }

  [[nodiscard]] const BitSequenceWithItemsCount& finalized_chunk_timestamp(uint32_t timestamp_stream_id) const noexcept {
    return encoder_.storage().finalized_timestamp_streams_[timestamp_stream_id];
  }
  [[nodiscard]] BitSequenceReader finalized_chunk_timestamp_reader(uint32_t timestamp_stream_id) const noexcept {
    return finalized_chunk_timestamp(timestamp_stream_id).reader();
  }
  [[nodiscard]] BitSequenceReader finalized_chunk_data_reader(uint32_t data_stream_id) const noexcept {
    return encoder_.storage().finalized_data_streams_[data_stream_id].reader();
  }
  [[nodiscard]] BareBones::Vector<int64_t> decode_open_chunk_timestamp_list(uint32_t ls_id) const noexcept {
    return TimestampDecoder::decode_all(open_chunk_timestamp_reader(ls_id));
  }
  [[nodiscard]] BareBones::Vector<int64_t> decode_finalized_chunk_timestamp_list(uint32_t timestamp_stream_id) const noexcept {
    return TimestampDecoder::decode_all(finalized_chunk_timestamp_reader(timestamp_stream_id));
  }

  static bool strict_compare_doubles(double a, double b) noexcept { return std::bit_cast<uint64_t>(a) == std::bit_cast<uint64_t>(b); }
};

class EncodeTestFixture : public EncoderTestTrait<series_data::kSamplesPerChunkDefault>, public testing::Test {};

TEST_F(EncodeTestFixture, EncodeUint32Constant) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 1.0);

  // Assert
  ASSERT_EQ(DataChunk::EncodingType::kUint32Constant, chunk(0).encoding_type);
  EXPECT_EQ(1.0, chunk(0).encoder.uint32_constant.value());
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2}), decode_open_chunk_timestamp_list(0));
}

TEST_F(EncodeTestFixture, EncodeDoubleConstant) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(0, 2, 1.1);

  // Assert
  ASSERT_EQ(DataChunk::EncodingType::kDoubleConstant, chunk(0).encoding_type);
  EXPECT_EQ(1.1, encoder_.storage().double_constant_encoders_[chunk(0).encoder.double_constant].value());
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2}), decode_open_chunk_timestamp_list(0));
}

TEST_F(EncodeTestFixture, EncodeDoubleConstantNegativeValue) {
  // Arrange

  // Act
  encoder_.encode(0, 1, -1.0);
  encoder_.encode(0, 2, -1.0);

  // Assert
  ASSERT_EQ(DataChunk::EncodingType::kDoubleConstant, chunk(0).encoding_type);
  EXPECT_EQ(-1.0, encoder_.storage().double_constant_encoders_[chunk(0).encoder.double_constant].value());
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2}), decode_open_chunk_timestamp_list(0));
}

TEST_F(EncodeTestFixture, SwitchToTwoDoubleEncoderFromUint32ConstantEncoder) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 1.0);
  encoder_.encode(0, 3, 1.1);
  encoder_.encode(0, 4, 1.1);

  // Assert
  ASSERT_EQ(DataChunk::EncodingType::kTwoDoubleConstant, chunk(0).encoding_type);

  auto& encoder = encoder_.storage().two_double_constant_encoders_[chunk(0).encoder.two_double_constant];
  EXPECT_EQ(1.0, encoder.value1());
  EXPECT_EQ(2, encoder.value1_count());
  EXPECT_EQ(1.1, encoder.value2());
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2, 3, 4}), decode_open_chunk_timestamp_list(0));
}

TEST_F(EncodeTestFixture, SwitchToTwoDoubleEncoderFromDoubleConstantEncoder) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(0, 2, 1.1);
  encoder_.encode(0, 3, 1.2);
  encoder_.encode(0, 4, 1.2);

  // Assert
  ASSERT_EQ(DataChunk::EncodingType::kTwoDoubleConstant, chunk(0).encoding_type);

  auto& encoder = encoder_.storage().two_double_constant_encoders_[chunk(0).encoder.two_double_constant];
  EXPECT_EQ(1.1, encoder.value1());
  EXPECT_EQ(2, encoder.value1_count());
  EXPECT_EQ(1.2, encoder.value2());
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2, 3, 4}), decode_open_chunk_timestamp_list(0));
}

TEST_F(EncodeTestFixture, IntegerValuesGorillaEncoderWith1Value1) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, STALE_NAN);
  encoder_.encode(0, 4, 3.0);

  // Assert
  ASSERT_EQ(DataChunk::EncodingType::kAscIntegerValuesGorilla, chunk(0).encoding_type);

  auto& encoder = encoder_.storage().asc_integer_values_gorilla_encoders_[chunk(0).encoder.asc_integer_values_gorilla];
  EXPECT_TRUE(std::ranges::equal(BareBones::Vector<double>{1.0, 2.0, STALE_NAN, 3.0}, AscIntegerValuesGorillaDecoder::decode_all(encoder.stream().reader()),
                                 strict_compare_doubles));
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2, 3, 4}), decode_open_chunk_timestamp_list(0));
}

TEST_F(EncodeTestFixture, IntegerValuesGorillaEncoderWith2Value1) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 1.0);
  encoder_.encode(0, 3, STALE_NAN);
  encoder_.encode(0, 4, 2.0);

  // Assert
  ASSERT_EQ(DataChunk::EncodingType::kAscIntegerValuesGorilla, chunk(0).encoding_type);

  auto& encoder = encoder_.storage().asc_integer_values_gorilla_encoders_[chunk(0).encoder.asc_integer_values_gorilla];
  EXPECT_TRUE(std::ranges::equal(BareBones::Vector<double>{1.0, 1.0, STALE_NAN, 2.0}, AscIntegerValuesGorillaDecoder::decode_all(encoder.stream().reader()),
                                 strict_compare_doubles));
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2, 3, 4}), decode_open_chunk_timestamp_list(0));
}

TEST_F(EncodeTestFixture, IntegerValuesGorillaEncoderWith3Value1) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 1.0);
  encoder_.encode(0, 3, 1.0);
  encoder_.encode(0, 4, STALE_NAN);
  encoder_.encode(0, 5, 2.0);

  // Assert
  ASSERT_EQ(DataChunk::EncodingType::kAscIntegerValuesGorilla, chunk(0).encoding_type);

  auto& encoder = encoder_.storage().asc_integer_values_gorilla_encoders_[chunk(0).encoder.asc_integer_values_gorilla];
  EXPECT_TRUE(std::ranges::equal(BareBones::Vector<double>{1.0, 1.0, 1.0, STALE_NAN, 2.0},
                                 AscIntegerValuesGorillaDecoder::decode_all(encoder.stream().reader()), strict_compare_doubles));
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2, 3, 4, 5}), decode_open_chunk_timestamp_list(0));
}

TEST_F(EncodeTestFixture, SwitchToDoubleConstantEncoderFromIntegerValuesGorillaEncoderWithUniqueTimeserie) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, STALE_NAN);
  encoder_.encode(0, 4, 2.1);

  // Assert
  ASSERT_EQ(DataChunk::EncodingType::kDoubleConstant, chunk(0).encoding_type);
  EXPECT_EQ(2.1, encoder_.storage().double_constant_encoders_[chunk(0).encoder.double_constant].value());
  EXPECT_EQ((BareBones::Vector<int64_t>{4}), decode_open_chunk_timestamp_list(0));

  auto finalized = finalized_chunks(0);
  ASSERT_NE(finalized, nullptr);
  EXPECT_EQ(DataChunk::EncodingType::kAscIntegerValuesGorilla, finalized->front().encoding_type);
  EXPECT_TRUE(std::ranges::equal(BareBones::Vector<double>{1.0, 2.0, STALE_NAN},
                                 AscIntegerValuesGorillaDecoder::decode_all(finalized_chunk_data_reader(finalized->front().encoder.asc_integer_values_gorilla)),
                                 strict_compare_doubles));
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2, 3}), decode_finalized_chunk_timestamp_list(finalized->front().timestamp_encoder_state_id));
}

TEST_F(EncodeTestFixture, SwitchToDoubleConstantEncoderFromIntegerValuesGorillaEncoderWithNonUniqueTimeserie) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(1, 1, 1.0);

  encoder_.encode(0, 2, 2.0);
  encoder_.encode(1, 2, 1.0);

  encoder_.encode(0, 3, STALE_NAN);
  encoder_.encode(1, 3, 1.0);

  encoder_.encode(0, 4, 2.1);
  encoder_.encode(1, 4, 1.0);

  // Assert
  ASSERT_EQ(DataChunk::EncodingType::kDoubleConstant, chunk(0).encoding_type);
  EXPECT_EQ(2.1, encoder_.storage().double_constant_encoders_[chunk(0).encoder.double_constant].value());
  EXPECT_EQ((BareBones::Vector<int64_t>{4}), decode_open_chunk_timestamp_list(0));

  ASSERT_EQ(DataChunk::EncodingType::kUint32Constant, chunk(1).encoding_type);
  EXPECT_EQ(1.0, chunk(1).encoder.uint32_constant.value());
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2, 3, 4}), decode_open_chunk_timestamp_list(1));

  auto finalized = finalized_chunks(0);
  ASSERT_NE(finalized, nullptr);
  EXPECT_EQ(DataChunk::EncodingType::kAscIntegerValuesGorilla, finalized->front().encoding_type);
  EXPECT_TRUE(std::ranges::equal(BareBones::Vector<double>{1.0, 2.0, STALE_NAN},
                                 AscIntegerValuesGorillaDecoder::decode_all(finalized_chunk_data_reader(finalized->front().encoder.asc_integer_values_gorilla)),
                                 strict_compare_doubles));
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2, 3}), decode_finalized_chunk_timestamp_list(finalized->front().timestamp_encoder_state_id));
}

TEST_F(EncodeTestFixture, ValuesGorillaEncoder) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(1, 1, 1.1);

  encoder_.encode(0, 2, 2.0);
  encoder_.encode(1, 2, 2.0);

  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 3.0);

  // Assert
  ASSERT_EQ(DataChunk::EncodingType::kValuesGorilla, chunk(0).encoding_type);

  auto& encoder = encoder_.storage().values_gorilla_encoders_[chunk(0).encoder.values_gorilla];
  EXPECT_TRUE(
      std::ranges::equal(BareBones::Vector<double>{1.1, 2.0, 3.0, 3.0}, ValuesGorillaDecoder::decode_all(encoder.stream().reader()), strict_compare_doubles));
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2, 3, 4}), decode_open_chunk_timestamp_list(0));
}

TEST_F(EncodeTestFixture, GorillaEncoder) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(0, 2, 1.1);
  encoder_.encode(0, 3, 2.0);
  encoder_.encode(0, 4, 3.0);
  encoder_.encode(0, 5, STALE_NAN);

  // Assert
  ASSERT_EQ(DataChunk::EncodingType::kGorilla, chunk(0).encoding_type);

  auto& encoder = encoder_.storage().gorilla_encoders_[chunk(0).encoder.gorilla];
  EXPECT_EQ((GorillaDecoder::SampleList{{.timestamp = 1, .value = 1.1},
                                        {.timestamp = 2, .value = 1.1},
                                        {.timestamp = 3, .value = 2.0},
                                        {.timestamp = 4, .value = 3.0},
                                        {.timestamp = 5, .value = STALE_NAN}}),
            GorillaDecoder::decode_all(encoder.stream().reader()));
}

class FinalizeChunkTestFixture : public EncoderTestTrait<4>, public testing::Test {
 protected:
  static constexpr double kIntegerValue = 1.0;
  static constexpr double kDoubleValue = 1.1;

  static BareBones::Vector<int64_t> create_timestamps() {
    BareBones::Vector<int64_t> timestamps;
    timestamps.resize(kSamplesPerChunk);
    std::iota(timestamps.begin(), timestamps.end(), 0);

    return timestamps;
  }

  void assert_finalized_timestamp(const DataChunk& finalized_chunk) {
    EXPECT_EQ(kSamplesPerChunk, finalized_chunk_timestamp(finalized_chunk.timestamp_encoder_state_id).count());
    EXPECT_EQ(create_timestamps(), decode_finalized_chunk_timestamp_list(finalized_chunk.timestamp_encoder_state_id));
  }

  template <class ValueAsserter, class TimestampAsserter>
  void assert_result(uint32_t ls_id, ValueAsserter&& value_asserter, TimestampAsserter&& timestamp_asserter) {
    auto& open_chunk = chunk(ls_id);
    EXPECT_EQ(1U, open_chunk_timestamp(ls_id).count());
    EXPECT_EQ((BareBones::Vector<int64_t>{kSamplesPerChunk}), decode_open_chunk_timestamp_list(ls_id));

    auto finalized = finalized_chunks(ls_id);
    ASSERT_NE(finalized, nullptr);

    value_asserter(open_chunk, finalized->front());
    timestamp_asserter(finalized->front());
  };
};

TEST_F(FinalizeChunkTestFixture, FinalizeUint32ConstantChunkWithUniqueTimeserie) {
  // Arrange

  // Act
  for (uint8_t i = 0; i <= kSamplesPerChunk; ++i) {
    encoder_.encode(0, i, kIntegerValue);
  }

  // Assert
  assert_result(
      0,
      [](const DataChunk& open_chunk, const DataChunk& finalized_chunk) {
        ASSERT_EQ(DataChunk::EncodingType::kUint32Constant, open_chunk.encoding_type);
        EXPECT_EQ(kIntegerValue, open_chunk.encoder.uint32_constant.value());

        ASSERT_EQ(DataChunk::EncodingType::kUint32Constant, finalized_chunk.encoding_type);
        EXPECT_EQ(kIntegerValue, finalized_chunk.encoder.uint32_constant.value());
      },
      [this](const DataChunk& finalized_chunk) { assert_finalized_timestamp(finalized_chunk); });
}

TEST_F(FinalizeChunkTestFixture, FinalizeUint32ConstantChunkWithNonUniqueTimeserie) {
  // Arrange

  // Act
  for (uint8_t i = 0; i <= kSamplesPerChunk; ++i) {
    encoder_.encode(0, i, kIntegerValue);
    encoder_.encode(1, i, kIntegerValue);
  }

  // Assert
  static constexpr auto value_asserter = [](const DataChunk& open_chunk, const DataChunk& finalized_chunk) {
    ASSERT_EQ(DataChunk::EncodingType::kUint32Constant, open_chunk.encoding_type);
    EXPECT_EQ(kIntegerValue, open_chunk.encoder.uint32_constant.value());

    ASSERT_EQ(DataChunk::EncodingType::kUint32Constant, finalized_chunk.encoding_type);
    EXPECT_EQ(kIntegerValue, finalized_chunk.encoder.uint32_constant.value());
  };
  static const auto timestamp_asserter = [this](const DataChunk& finalized_chunk) { assert_finalized_timestamp(finalized_chunk); };
  assert_result(0, value_asserter, timestamp_asserter);
  assert_result(1, value_asserter, timestamp_asserter);
}

TEST_F(FinalizeChunkTestFixture, FinalizeDoubleConstantChunk) {
  // Arrange

  // Act
  for (uint8_t i = 0; i <= kSamplesPerChunk; ++i) {
    encoder_.encode(0, i, kDoubleValue);
  }

  // Assert
  assert_result(
      0,
      [this](const DataChunk& open_chunk, const DataChunk& finalized_chunk) {
        ASSERT_EQ(DataChunk::EncodingType::kDoubleConstant, open_chunk.encoding_type);
        EXPECT_EQ(kDoubleValue, encoder_.storage().double_constant_encoders_[open_chunk.encoder.double_constant].value());

        ASSERT_EQ(DataChunk::EncodingType::kDoubleConstant, finalized_chunk.encoding_type);
        EXPECT_EQ(kDoubleValue, encoder_.storage().double_constant_encoders_[finalized_chunk.encoder.double_constant].value());
      },
      [this](const DataChunk& finalized_chunk) { assert_finalized_timestamp(finalized_chunk); });
}

TEST_F(FinalizeChunkTestFixture, FinalizeTwoDoubleConstantChunk) {
  // Arrange

  // Act
  encoder_.encode(0, 0, kDoubleValue);
  for (uint8_t i = 0; i < kSamplesPerChunk; ++i) {
    encoder_.encode(0, i + 1, kDoubleValue + 0.1);
  }

  // Assert
  assert_result(
      0,
      [this](const DataChunk& open_chunk, const DataChunk& finalized_chunk) {
        ASSERT_EQ(DataChunk::EncodingType::kDoubleConstant, open_chunk.encoding_type);
        EXPECT_EQ(kDoubleValue + 0.1, encoder_.storage().double_constant_encoders_[open_chunk.encoder.double_constant].value());

        ASSERT_EQ(DataChunk::EncodingType::kTwoDoubleConstant, finalized_chunk.encoding_type);
        EXPECT_EQ(TwoDoubleConstantEncoder(kDoubleValue, kDoubleValue + 0.1, 1),
                  encoder_.storage().two_double_constant_encoders_[finalized_chunk.encoder.two_double_constant]);
      },
      [this](const DataChunk& finalized_chunk) { assert_finalized_timestamp(finalized_chunk); });
}

TEST_F(FinalizeChunkTestFixture, FinalizeAscIntegerValuesGorillaChunk) {
  // Arrange

  // Act
  encoder_.encode(0, 0, 1.0);
  encoder_.encode(0, 1, 2.0);
  encoder_.encode(0, 2, 3.0);
  encoder_.encode(0, 3, 4.0);
  encoder_.encode(0, 4, 5.0);

  // Assert
  assert_result(
      0,
      [this](const DataChunk& open_chunk, const DataChunk& finalized_chunk) {
        ASSERT_EQ(DataChunk::EncodingType::kUint32Constant, open_chunk.encoding_type);
        EXPECT_EQ(5.0, open_chunk.encoder.uint32_constant.value());

        EXPECT_EQ(DataChunk::EncodingType::kAscIntegerValuesGorilla, finalized_chunk.encoding_type);
        EXPECT_TRUE(
            std::ranges::equal(BareBones::Vector<double>{1.0, 2.0, 3.0, 4.0},
                               AscIntegerValuesGorillaDecoder::decode_all(finalized_chunk_data_reader(finalized_chunk.encoder.asc_integer_values_gorilla)),
                               strict_compare_doubles));
      },
      [this](const DataChunk& finalized_chunk) { assert_finalized_timestamp(finalized_chunk); });
}

TEST_F(FinalizeChunkTestFixture, FinalizeValuesGorillaChunk) {
  // Arrange

  // Act
  encoder_.encode(0, 0, 1.1);
  encoder_.encode(1, 0, 1.1);

  encoder_.encode(0, 1, 2.1);
  encoder_.encode(1, 1, 2.1);

  encoder_.encode(0, 2, 3.1);
  encoder_.encode(0, 3, 4.1);
  encoder_.encode(0, 4, 5.1);

  // Assert
  assert_result(
      0,
      [this](const DataChunk& open_chunk, [[maybe_unused]] const DataChunk& finalized_chunk) {
        ASSERT_EQ(DataChunk::EncodingType::kDoubleConstant, open_chunk.encoding_type);
        EXPECT_EQ(5.1, encoder_.storage().double_constant_encoders_[open_chunk.encoder.double_constant].value());

        EXPECT_EQ(DataChunk::EncodingType::kValuesGorilla, finalized_chunk.encoding_type);
        EXPECT_TRUE(std::ranges::equal(BareBones::Vector<double>{1.1, 2.1, 3.1, 4.1},
                                       ValuesGorillaDecoder::decode_all(finalized_chunk_data_reader(finalized_chunk.encoder.values_gorilla)),
                                       strict_compare_doubles));
      },
      [this](const DataChunk& finalized_chunk) { assert_finalized_timestamp(finalized_chunk); });
}

TEST_F(FinalizeChunkTestFixture, FinalizeGorillaChunk) {
  // Arrange

  // Act
  encoder_.encode(0, 0, 1.1);
  encoder_.encode(0, 1, 2.1);
  encoder_.encode(0, 2, 3.1);
  encoder_.encode(0, 3, 4.1);
  encoder_.encode(0, 4, 5.1);

  // Assert
  assert_result(
      0,
      [this](const DataChunk& open_chunk, const DataChunk& finalized_chunk) {
        ASSERT_EQ(DataChunk::EncodingType::kDoubleConstant, open_chunk.encoding_type);
        EXPECT_EQ(5.1, encoder_.storage().double_constant_encoders_[open_chunk.encoder.double_constant].value());

        EXPECT_EQ(DataChunk::EncodingType::kGorilla, finalized_chunk.encoding_type);
        EXPECT_EQ((GorillaDecoder::SampleList{
                      {.timestamp = 0, .value = 1.1},
                      {.timestamp = 1, .value = 2.1},
                      {.timestamp = 2, .value = 3.1},
                      {.timestamp = 3, .value = 4.1},
                  }),
                  GorillaDecoder::decode_all(BitSequenceWithItemsCount::reader(encoder_.storage().finalized_data_streams_[finalized_chunk.encoder.gorilla])));
      },
      [](const DataChunk&) {});
}

class EncodeOutdatedChunkTestFixture : public EncoderTestTrait<series_data::kSamplesPerChunkDefault>, public testing::Test {};

TEST_F(EncodeOutdatedChunkTestFixture, EncodeUint32ConstantActualSample) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 1, 1.0);

  // Assert
  ASSERT_EQ(DataChunk::EncodingType::kUint32Constant, chunk(0).encoding_type);
  EXPECT_EQ((BareBones::Vector<int64_t>{1}), decode_open_chunk_timestamp_list(0));
}

TEST_F(EncodeOutdatedChunkTestFixture, EncodeUint32ConstantOutdatedSample) {
  // Arrange

  // Act
  encoder_.encode(0, 2, 1.0);
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 1, 2.0);

  // Assert
  ASSERT_EQ(DataChunk::EncodingType::kUint32Constant, chunk(0).encoding_type);
  EXPECT_EQ((BareBones::Vector<int64_t>{2}), decode_open_chunk_timestamp_list(0));

  auto outdated = outdated_chunk(0);
  ASSERT_NE(nullptr, outdated);
  EXPECT_EQ((GorillaDecoder::SampleList{{.timestamp = 1, .value = 1.0}, {.timestamp = 1, .value = 2.0}}),
            GorillaDecoder::decode_all(outdated->stream().reader()));
}

TEST_F(EncodeOutdatedChunkTestFixture, EncodeDoubleConstantActualSample) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(0, 1, 1.1);

  // Assert
  ASSERT_EQ(DataChunk::EncodingType::kDoubleConstant, chunk(0).encoding_type);
  EXPECT_EQ((BareBones::Vector<int64_t>{1}), decode_open_chunk_timestamp_list(0));
}

TEST_F(EncodeOutdatedChunkTestFixture, EncodeDoubleConstantOutdatedSample) {
  // Arrange

  // Act
  encoder_.encode(0, 2, 1.1);
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(0, 1, 1.2);

  // Assert
  ASSERT_EQ(DataChunk::EncodingType::kDoubleConstant, chunk(0).encoding_type);
  EXPECT_EQ((BareBones::Vector<int64_t>{2}), decode_open_chunk_timestamp_list(0));

  auto outdated = outdated_chunk(0);
  ASSERT_NE(nullptr, outdated);
  EXPECT_EQ((GorillaDecoder::SampleList{{.timestamp = 1, .value = 1.1}, {.timestamp = 1, .value = 1.2}}),
            GorillaDecoder::decode_all(outdated->stream().reader()));
}

TEST_F(EncodeOutdatedChunkTestFixture, EncodeTwoDoubleConstantActualSample) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(0, 2, 1.2);
  encoder_.encode(0, 2, 1.2);

  // Assert
  ASSERT_EQ(DataChunk::EncodingType::kTwoDoubleConstant, chunk(0).encoding_type);
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2}), decode_open_chunk_timestamp_list(0));
}

TEST_F(EncodeOutdatedChunkTestFixture, EncodeTwoDoubleConstantOutdatedSample) {
  // Arrange

  // Act
  encoder_.encode(0, 2, 1.1);
  encoder_.encode(0, 3, 1.2);
  encoder_.encode(0, 1, 1.2);
  encoder_.encode(0, 1, 1.3);

  // Assert
  ASSERT_EQ(DataChunk::EncodingType::kTwoDoubleConstant, chunk(0).encoding_type);
  EXPECT_EQ((BareBones::Vector<int64_t>{2, 3}), decode_open_chunk_timestamp_list(0));

  auto outdated = outdated_chunk(0);
  ASSERT_NE(nullptr, outdated);
  EXPECT_EQ((GorillaDecoder::SampleList{{.timestamp = 1, .value = 1.2}, {.timestamp = 1, .value = 1.3}}),
            GorillaDecoder::decode_all(outdated->stream().reader()));
}

TEST_F(EncodeOutdatedChunkTestFixture, EncodeIntegerValuesGorillaEncoderActualSample) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, STALE_NAN);
  encoder_.encode(0, 3, STALE_NAN);

  // Assert
  ASSERT_EQ(DataChunk::EncodingType::kAscIntegerValuesGorilla, chunk(0).encoding_type);
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2, 3}), decode_open_chunk_timestamp_list(0));
}

TEST_F(EncodeOutdatedChunkTestFixture, EncodeIntegerValuesGorillaEncoderOutdatedSample) {
  // Arrange

  // Act
  encoder_.encode(0, 2, 1.0);
  encoder_.encode(0, 3, 2.0);
  encoder_.encode(0, 4, STALE_NAN);
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 1, 2.0);

  // Assert
  ASSERT_EQ(DataChunk::EncodingType::kAscIntegerValuesGorilla, chunk(0).encoding_type);
  EXPECT_EQ((BareBones::Vector<int64_t>{2, 3, 4}), decode_open_chunk_timestamp_list(0));

  auto outdated = outdated_chunk(0);
  ASSERT_NE(nullptr, outdated);
  EXPECT_EQ((GorillaDecoder::SampleList{{.timestamp = 1, .value = 1.0}, {.timestamp = 1, .value = 2.0}}),
            GorillaDecoder::decode_all(outdated->stream().reader()));
}

TEST_F(EncodeOutdatedChunkTestFixture, EncodeValuesGorillaActualSample) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(1, 1, 1.1);
  encoder_.encode(0, 2, 2.1);
  encoder_.encode(0, 3, STALE_NAN);
  encoder_.encode(0, 3, STALE_NAN);

  // Assert
  ASSERT_EQ(DataChunk::EncodingType::kValuesGorilla, chunk(0).encoding_type);
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2, 3}), decode_open_chunk_timestamp_list(0));
}

TEST_F(EncodeOutdatedChunkTestFixture, EncodeValuesGorillaOutdatedSample) {
  // Arrange

  // Act
  encoder_.encode(0, 2, 1.1);
  encoder_.encode(1, 2, 1.1);
  encoder_.encode(0, 3, 2.1);
  encoder_.encode(0, 4, STALE_NAN);
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 1, 1.1);

  // Assert
  ASSERT_EQ(DataChunk::EncodingType::kValuesGorilla, chunk(0).encoding_type);
  EXPECT_EQ((BareBones::Vector<int64_t>{2, 3, 4}), decode_open_chunk_timestamp_list(0));

  auto outdated = outdated_chunk(0);
  ASSERT_NE(nullptr, outdated);
  EXPECT_EQ((GorillaDecoder::SampleList{{.timestamp = 1, .value = 1.0}, {.timestamp = 1, .value = 1.1}}),
            GorillaDecoder::decode_all(outdated->stream().reader()));
}

TEST_F(EncodeOutdatedChunkTestFixture, EncodeGorillaActualSample) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(0, 2, 2.1);
  encoder_.encode(0, 3, STALE_NAN);
  encoder_.encode(0, 3, STALE_NAN);

  // Assert
  ASSERT_EQ(DataChunk::EncodingType::kGorilla, chunk(0).encoding_type);
  auto& encoder = encoder_.storage().gorilla_encoders_[chunk(0).encoder.gorilla];
  EXPECT_EQ((GorillaDecoder::SampleList{{.timestamp = 1, .value = 1.1}, {.timestamp = 2, .value = 2.1}, {.timestamp = 3, .value = STALE_NAN}}),
            GorillaDecoder::decode_all(encoder.stream().reader()));
}

TEST_F(EncodeOutdatedChunkTestFixture, EncodeGorillaOutdatedSample) {
  // Arrange

  // Act
  encoder_.encode(0, 2, 1.1);
  encoder_.encode(0, 3, 2.1);
  encoder_.encode(0, 4, STALE_NAN);
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 1, 1.1);

  // Assert
  ASSERT_EQ(DataChunk::EncodingType::kGorilla, chunk(0).encoding_type);
  auto& encoder = encoder_.storage().gorilla_encoders_[chunk(0).encoder.gorilla];
  EXPECT_EQ((GorillaDecoder::SampleList{{.timestamp = 2, .value = 1.1}, {.timestamp = 3, .value = 2.1}, {.timestamp = 4, .value = STALE_NAN}}),
            GorillaDecoder::decode_all(encoder.stream().reader()));

  auto outdated = outdated_chunk(0);
  ASSERT_NE(nullptr, outdated);
  EXPECT_EQ((GorillaDecoder::SampleList{{.timestamp = 1, .value = 1.0}, {.timestamp = 1, .value = 1.1}}),
            GorillaDecoder::decode_all(outdated->stream().reader()));
}

}  // namespace
