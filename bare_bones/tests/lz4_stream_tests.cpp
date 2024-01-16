#include <gtest/gtest.h>

#include <fstream>
#include <sstream>

#include "bare_bones/lz4_stream.h"

namespace {

class LZ4OStreamFixture : public testing::Test {
 protected:
  std::ostringstream string_stream_;
  BareBones::LZ4Stream::ostream lz4stream_{string_stream_};
};

TEST_F(LZ4OStreamFixture, TestFlush) {
  // Arrange
  const auto headers_length = string_stream_.str().length();

  // Act
  lz4stream_ << "1234567890";
  auto length_before_flush = string_stream_.view().size();
  lz4stream_ << std::flush;
  auto length_after_flush = string_stream_.view().size();

  // Assert
  EXPECT_EQ(headers_length, length_before_flush);
  EXPECT_GT(length_after_flush, headers_length);
}

TEST_F(LZ4OStreamFixture, NoEndingMarkAtClose) {
  // Arrange
  const auto headers_length = string_stream_.view().length();

  // Act
  lz4stream_.close();

  // Assert
  EXPECT_EQ(headers_length, string_stream_.view().length());
}

class LZ4StreamUsageFixture : public testing::Test {
 protected:
  std::stringstream string_stream_;
  BareBones::LZ4Stream::ostream lz4ostream_{string_stream_};
  BareBones::LZ4Stream::istream lz4istream_{string_stream_};

  void SetUp() override { lz4istream_.exceptions(std::ifstream::failbit | std::ifstream::badbit | std::ifstream::eofbit); }
};

TEST_F(LZ4StreamUsageFixture, NoLZ4FrameHeader) {
  // Arrange
  int value{};

  // Act
  string_stream_.str("");

  // Assert
  EXPECT_THROW(lz4istream_ >> value, std::runtime_error);
}

TEST_F(LZ4StreamUsageFixture, LZ4FrameHeaderIsIncomplete) {
  // Arrange
  int value{};

  // Act
  string_stream_.str(std::string(string_stream_.view().substr(0, LZ4F_MIN_SIZE_TO_KNOW_HEADER_LENGTH)));

  // Assert
  EXPECT_THROW(lz4istream_ >> value, std::runtime_error);
}

TEST_F(LZ4StreamUsageFixture, NoDataBlocks) {
  // Arrange
  int value{};

  // Act

  // Assert
  EXPECT_THROW(lz4istream_ >> value, std::runtime_error);
}

TEST_F(LZ4StreamUsageFixture, InvalidDataBlockSize) {
  // Arrange
  constexpr std::string_view data = "1234567890";
  std::string decoded(data.size(), '\0');

  // Act
  lz4ostream_ << data << std::flush;
  auto block_with_invalid_size = string_stream_.view();
  block_with_invalid_size.remove_suffix(1);
  string_stream_.str(std::string(block_with_invalid_size));

  // Assert
  EXPECT_THROW(lz4istream_.read(&decoded[0], decoded.size()), std::runtime_error);
}

TEST_F(LZ4StreamUsageFixture, InvalidDataBlock) {
  // Arrange
  const auto data_block_offset = string_stream_.view().size();
  constexpr size_t data_size = 1024;
  std::string data(data_size, 'a');
  std::string decoded(data_size, '\0');

  // Act
  lz4ostream_ << data << std::flush;
  string_stream_.str(string_stream_.str().replace(data_block_offset + sizeof(uint32_t), 1, "\xFF"));

  // Assert
  EXPECT_THROW(lz4istream_.read(&decoded[0], decoded.size()), std::runtime_error);
}

TEST_F(LZ4StreamUsageFixture, EncodeDecodeWithoutCompression) {
  // Arrange
  constexpr std::string_view data = "1234567890";
  std::string decoded(data.size(), '\0');

  // Act
  lz4ostream_ << data << std::flush;
  lz4istream_.read(&decoded[0], decoded.size());

  // Assert
  EXPECT_EQ(data, decoded);
}

TEST_F(LZ4StreamUsageFixture, ReadTwoOfThreeDataBlocks) {
  // Arrange
  constexpr size_t data_size = 1024;
  std::string data(data_size, 'a');
  std::string decoded(data_size * 2, '\0');

  // Act
  lz4ostream_ << data << std::flush;
  lz4ostream_ << data << std::flush;
  const auto third_data_block_offset = string_stream_.view().size();
  lz4ostream_ << data << std::flush;
  lz4istream_.read(&decoded[0], decoded.size());

  // Assert
  EXPECT_EQ(std::string(data_size * 2, 'a'), decoded);
  EXPECT_EQ(third_data_block_offset, string_stream_.tellg());
}

}  // namespace
