#include <random>

#include <gtest/gtest.h>

#include "bare_bones/lz4/compressor.h"

namespace {

using BareBones::lz4::CompressedBufferSize;
using BareBones::lz4::StreamCompressor;

class StreamCompressorFixture : public testing::Test {
 public:
  void SetUp() override { std::ignore = compressor_.initialize(); }

 protected:
  static std::string create_random_string(size_t size) {
    std::string random_string;
    random_string.reserve(size);

    std::mt19937 generator(std::random_device{}());
    std::uniform_int_distribution<unsigned char> char_distributor('a', 'z');

    for (size_t i = 0; i < size; ++i) {
      random_string.push_back(char_distributor(generator));
    }

    return random_string;
  }

 protected:
  static constexpr std::string_view LZ4_HEADER{"\x04\x22\x4d\x18\x40\x40\xc0"};
  static constexpr std::string_view SMALL_DATA_FRAME{"1234567890"};
  static constexpr char COMPRESSED_SMALL_DATA_FRAME_BUFFER[] = {// lz4 header
                                                                "\x04\x22\x4d\x18\x40\x40\xc0"
                                                                // compressed frame size with uncompressed flag (0x80)
                                                                "\x0A\x00\x00\x80"
                                                                // raw data
                                                                "1234567890"};
  static constexpr std::string_view COMPRESSED_SMALL_DATA_FRAME{COMPRESSED_SMALL_DATA_FRAME_BUFFER, sizeof(COMPRESSED_SMALL_DATA_FRAME_BUFFER) - 1};

 protected:
  StreamCompressor compressor_;
};

TEST_F(StreamCompressorFixture, TestInitialize) {
  // Arrange

  // Act

  // Assert
  EXPECT_TRUE(compressor_.is_initialized());
}

TEST_F(StreamCompressorFixture, CompressEmptyData) {
  // Arrange
  constexpr std::string_view data_frame;

  // Act
  auto result = compressor_.compress(data_frame);

  // Assert
  EXPECT_EQ(LZ4_HEADER, result);
}

TEST_F(StreamCompressorFixture, CompressOneFrameWithoutCompression) {
  // Arrange

  // Act
  auto result = compressor_.compress(SMALL_DATA_FRAME);

  // Assert
  EXPECT_EQ(COMPRESSED_SMALL_DATA_FRAME, result);
}

TEST_F(StreamCompressorFixture, CompressTwoFrames) {
  // Arrange
  static constexpr std::string_view second_data_frame{"abcdefghijklmnopqrstuvwxyz"};
  static constexpr char compressed_second_data_frame_buffer[] = {// compressed frame size with uncompressed flag (0x80)
                                                                   "\x1A\x00\x00\x80"
                                                                   // raw data
                                                                   "abcdefghijklmnopqrstuvwxyz"};
  static constexpr std::string_view compressed_second_data_frame{compressed_second_data_frame_buffer, sizeof(compressed_second_data_frame_buffer) - 1};

  // Act
  auto result1 = std::string(compressor_.compress(SMALL_DATA_FRAME));
  auto result2 = std::string(compressor_.compress(second_data_frame));

  // Assert
  EXPECT_EQ(COMPRESSED_SMALL_DATA_FRAME, result1);
  EXPECT_EQ(compressed_second_data_frame, result2);
}

TEST_F(StreamCompressorFixture, CompressEmptySecondFrame) {
  // Arrange

  // Act
  auto result1 = std::string(compressor_.compress(SMALL_DATA_FRAME));
  auto result2 = std::string(compressor_.compress({}));

  // Assert
  EXPECT_TRUE(result2.empty());
  EXPECT_FALSE(compressor_.lz4_call_result().is_error());
}

TEST_F(StreamCompressorFixture, CompressedFrameGreaterThan64K) {
  // Arrange
  constexpr size_t size_64kb = std::numeric_limits<uint16_t>().max();
  auto big_buffer = create_random_string(size_64kb * 2);

  // Act
  auto result = compressor_.compress(big_buffer);

  // Assert
  ASSERT_GT(result.size(), LZ4_HEADER.size() + sizeof(CompressedBufferSize));
  ASSERT_GT(*reinterpret_cast<const CompressedBufferSize*>(result.data() + LZ4_HEADER.size()), size_64kb);
}

TEST_F(StreamCompressorFixture, CompressOneWithCompression) {
  // Arrange
  std::string repeated_string(1024, 'a');
  static constexpr char expected_buffer[] = {// lz4 header
                                             "\x04\x22\x4d\x18\x40\x40\xc0"
                                             // compressed frame size
                                             "\x0E\x00\x00\x00"
                                             // raw data
                                             "\x1f\x61\x01\x00\xFF\xFF\xFF\xEA\x50\x61\x61\x61\x61\x61"};
  static constexpr std::string_view expected{expected_buffer, sizeof(expected_buffer) - 1};

  // Act
  auto result = compressor_.compress(repeated_string);

  // Assert
  EXPECT_EQ(expected, result);
}

}  // namespace
