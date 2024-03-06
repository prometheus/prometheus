#include <gtest/gtest.h>

#include <fstream>
#include <sstream>

#include "bare_bones/lz4_stream.h"

namespace {

class LZ4OStreamSetStreamFixture : public testing::Test {};

TEST_F(LZ4OStreamSetStreamFixture, WriteHeaderInConstructor) {
  // Arrange
  std::ostringstream stream;
  BareBones::LZ4Stream::ostream lz4stream{&stream};

  // Act

  // Assert
  EXPECT_GT(stream.str().length(), 0);
}

TEST_F(LZ4OStreamSetStreamFixture, WriteHeaderInSetter) {
  // Arrange
  std::ostringstream stream;
  BareBones::LZ4Stream::ostream lz4stream{nullptr};

  // Act
  lz4stream.set_stream(&stream);

  // Assert
  EXPECT_GT(stream.str().length(), 0);
}

TEST_F(LZ4OStreamSetStreamFixture, WorkWithTemporaryStream) {
  // Arrange
  BareBones::LZ4Stream::ostream lz4stream{nullptr};

  // Act
  {
    std::ostringstream stream;
    lz4stream.set_stream(&stream);
    lz4stream << "12345" << std::flush;
    lz4stream.set_stream(nullptr);
  }

  // Assert
}

class LZ4OStreamFixture : public testing::Test {
 protected:
  std::ostringstream string_stream_;
  BareBones::LZ4Stream::ostream lz4stream_{&string_stream_};
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

TEST_F(LZ4OStreamFixture, NoEndMarkAtClose) {
  // Arrange
  const auto headers_length = string_stream_.view().length();

  // Act
  lz4stream_.close();

  // Assert
  EXPECT_EQ(headers_length, string_stream_.view().length());
}

class LZ4IStreamFixture : public testing::Test {
 protected:
  std::stringstream string_stream_;
  BareBones::LZ4Stream::ostream lz4ostream_{&string_stream_};
  BareBones::LZ4Stream::istream lz4istream_{&string_stream_};

  void SetUp() override { lz4istream_.exceptions(std::ifstream::failbit | std::ifstream::badbit | std::ifstream::eofbit); }
};

TEST_F(LZ4IStreamFixture, ResetPointersOnSetStream) {
  // Arrange
  std::stringstream string_stream2;
  std::string buffer1(3, '\0');
  std::string buffer2(3, '\0');

  // Act
  lz4ostream_ << "123456" << std::flush;
  lz4ostream_.set_stream(&string_stream2);
  lz4ostream_ << "abcdef" << std::flush;
  lz4ostream_.set_stream(nullptr);

  lz4istream_.read(&buffer1[0], buffer1.length());
  lz4istream_.set_stream(&string_stream2);
  lz4istream_.read(&buffer2[0], buffer2.length());

  // Assert
  EXPECT_EQ("123", buffer1);
  EXPECT_EQ("abc", buffer2);
}

TEST_F(LZ4IStreamFixture, ResetStateOnSetStream) {
  // Arrange
  std::stringstream string_stream2;
  std::string buffer1(6, '\0');
  std::string buffer2(3, '\0');

  // Act
  lz4ostream_ << "123" << std::flush;
  lz4ostream_.set_stream(&string_stream2);
  lz4ostream_ << "456" << std::flush;
  lz4ostream_.set_stream(nullptr);

  EXPECT_THROW(lz4istream_.read(&buffer1[0], buffer1.length()), std::runtime_error);
  lz4istream_.set_stream(&string_stream2);
  lz4istream_.read(&buffer2[0], buffer2.length());

  // Assert
  EXPECT_FALSE(lz4istream_.eof());
  EXPECT_EQ("456", buffer2);
}

TEST_F(LZ4IStreamFixture, NoLZ4FrameHeader) {
  // Arrange
  int value{};

  // Act
  string_stream_.str("");

  // Assert
  EXPECT_THROW(lz4istream_ >> value, std::runtime_error);
}

TEST_F(LZ4IStreamFixture, LZ4FrameHeaderIsIncomplete) {
  // Arrange
  int value{};

  // Act
  string_stream_.str(std::string(string_stream_.view().substr(0, LZ4F_MIN_SIZE_TO_KNOW_HEADER_LENGTH)));

  // Assert
  EXPECT_THROW(lz4istream_ >> value, std::runtime_error);
}

TEST_F(LZ4IStreamFixture, NoDataBlocks) {
  // Arrange
  int value{};

  // Act

  // Assert
  EXPECT_THROW(lz4istream_ >> value, std::runtime_error);
}

TEST_F(LZ4IStreamFixture, InvalidDataBlockSize) {
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

TEST_F(LZ4IStreamFixture, InvalidDataBlock) {
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

TEST_F(LZ4IStreamFixture, StopReadingAfterEndMark) {
  // Arrange
  constexpr size_t data_size = 1024;
  std::string data(data_size, 'a');
  std::string decoded(data_size, '\0');
  uint32_t end_mark = 0;
  uint32_t crc32 = 0x01020304;

  // Act
  lz4ostream_ << data << std::flush;
  auto buffer_size = string_stream_.view().length();

  string_stream_.str(
      string_stream_.str().append(reinterpret_cast<const char*>(&end_mark), sizeof(end_mark)).append(reinterpret_cast<const char*>(&crc32), sizeof(crc32)));

  // Assert
  EXPECT_NO_THROW(lz4istream_.read(&decoded[0], decoded.size()));
  EXPECT_THROW(lz4istream_.read(&decoded[0], 1), std::runtime_error);
  EXPECT_EQ(buffer_size + sizeof(end_mark), string_stream_.tellg());
}

class LZ4StreamUsageFixture : public testing::Test {
 protected:
  std::stringstream string_stream_;
  BareBones::LZ4Stream::ostream lz4ostream_{&string_stream_};
  BareBones::LZ4Stream::istream lz4istream_{&string_stream_};

  void SetUp() override { lz4istream_.exceptions(std::ifstream::failbit | std::ifstream::badbit | std::ifstream::eofbit); }
};

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
