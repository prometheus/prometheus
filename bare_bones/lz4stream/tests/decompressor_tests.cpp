#include <random>

#include <gtest/gtest.h>

#include "bare_bones/lz4stream/decompressor.h"
#include "bare_bones/lz4stream/decompressor_buffer.h"

namespace {

using BareBones::lz4stream::Decompressor;
using BareBones::lz4stream::DecompressorBuffer;
using BareBones::lz4stream::DecompressResult;

class DecompressorFixture : public testing::Test {
 public:
  void SetUp() override { std::ignore = decompressor_.initialize(); }

 protected:
  static constexpr std::string_view LZ4_HEADER{"\x04\x22\x4d\x18\x40\x40\xc0"};
  static constexpr char SMALL_DATA_FRAME_BUFFER[] = {// lz4 header
                                                     "\x04\x22\x4d\x18\x40\x40\xc0"
                                                     // compressed frame size with uncompressed flag (0x80)
                                                     "\x0A\x00\x00\x80"
                                                     // raw data
                                                     "1234567890"};
  static constexpr std::string_view SMALL_DATA_FRAME{SMALL_DATA_FRAME_BUFFER, sizeof(SMALL_DATA_FRAME_BUFFER) - 1};

  static constexpr char BIG_COMPRESSION_RATE_FRAME_BUFFER[] = {// lz4 header
                                                               "\x04\x22\x4d\x18\x40\x40\xc0"
                                                               // compressed frame size
                                                               "\x0E\x00\x00\x00"
                                                               // raw data
                                                               "\x1f\x61\x01\x00\xFF\xFF\xFF\xEA\x50\x61\x61\x61\x61\x61"};
  static constexpr std::string_view BIG_COMPRESSION_RATE_FRAME{BIG_COMPRESSION_RATE_FRAME_BUFFER, sizeof(BIG_COMPRESSION_RATE_FRAME_BUFFER) - 1};

 protected:
  DecompressorBuffer buffer_{{.shrink_ratio = 0.3, .threshold_size_shrink_ratio = 0.5}};
  Decompressor<DecompressorBuffer> decompressor_{buffer_, {.min_allocation_size = 1, .allocation_ratio = 2.0, .reallocation_ratio = 2.0}};
};

TEST_F(DecompressorFixture, TestInitialize) {
  // Arrange

  // Act

  // Assert
  EXPECT_TRUE(decompressor_.is_initialized());
}

TEST_F(DecompressorFixture, DecompressEmptyBlock) {
  // Arrange
  std::string_view empty;

  // Act
  auto result = decompressor_.decompress(empty);

  // Assert
  EXPECT_EQ(DecompressResult{}, result);
}

TEST_F(DecompressorFixture, DecompressHeaderOnly) {
  // Arrange

  // Act
  auto result = decompressor_.decompress(LZ4_HEADER);

  // Assert
  EXPECT_EQ(DecompressResult{}, result);
}

TEST_F(DecompressorFixture, DecompressFrameWithoutCompression) {
  // Arrange

  // Act
  auto result = decompressor_.decompress(SMALL_DATA_FRAME);

  // Assert
  EXPECT_EQ((DecompressResult{.data = "1234567890", .frame_size = SMALL_DATA_FRAME.size()}), result);
}

TEST_F(DecompressorFixture, DecompressFrameWithCompression) {
  // Arrange
  std::string expected(1024, 'a');

  // Act
  auto result = decompressor_.decompress(BIG_COMPRESSION_RATE_FRAME);

  // Assert
  EXPECT_EQ((DecompressResult{.data = expected, .frame_size = BIG_COMPRESSION_RATE_FRAME.size()}), result);
}

TEST_F(DecompressorFixture, DecompressTwoFrames) {
  // Arrange
  static constexpr char second_data_frame_buffer[] = {// compressed frame size with uncompressed flag (0x80)
                                                      "\x1A\x00\x00\x80"
                                                      // raw data
                                                      "abcdefghijklmnopqrstuvwxyz"};
  static constexpr std::string_view second_data_frame{second_data_frame_buffer, sizeof(second_data_frame_buffer) - 1};

  // Act
  auto result = decompressor_.decompress(BIG_COMPRESSION_RATE_FRAME);
  result = decompressor_.decompress(second_data_frame);

  // Assert
  EXPECT_EQ((DecompressResult{.data = "abcdefghijklmnopqrstuvwxyz", .frame_size = second_data_frame.size()}), result);
}

TEST_F(DecompressorFixture, DecompressBufferWithInvalidCompressedDataSize) {
  // Arrange
  static constexpr char buffer_with_invalid_size[] = {// compressed frame size with uncompressed flag (0x80)
                                                      "\x02\x00\x00\x80"
                                                      // raw data
                                                      "a"};

  // Act
  auto result = decompressor_.decompress({buffer_with_invalid_size, sizeof(buffer_with_invalid_size) - 1});

  // Assert
  EXPECT_EQ((DecompressResult{}), result);
}

TEST_F(DecompressorFixture, DecompressInvalidBuffer) {
  // Arrange
  static constexpr char inalid_buffer[] = {// lz4 header
                                           "\x04\x22\x4d\x18\x40\x40\xc0"
                                           // compressed frame size
                                           "\x0E\x00\x00\x00"
                                           // invalid first byte of raw data
                                           "\x00\x61\x01\x00\xFF\xFF\xFF\xEA\x50\x61\x61\x61\x61\x61"};

  // Act
  auto result = decompressor_.decompress({inalid_buffer, sizeof(inalid_buffer) - 1});

  // Assert
  EXPECT_EQ((DecompressResult{}), result);
  EXPECT_TRUE(decompressor_.lz4_call_result().is_error());
  EXPECT_EQ(LZ4F_ERROR_decompressionFailed, decompressor_.lz4_call_result().error());
}

}  // namespace
