#include <gtest/gtest.h>

#include "prometheus/tsdb/chunkenc/bstream.h"

#include "bare_bones/gorilla.h"

namespace {

using BareBones::AllocationSize;
using PromPP::Prometheus::tsdb::chunkenc::BStream;

class BStreamFixture : public testing::Test {
 protected:
  static constexpr std::array kAllocationSizesTable = {AllocationSize{0U}, AllocationSize{32U}};

  BStream<kAllocationSizesTable> stream_;
};

TEST_F(BStreamFixture, WriteSingleBit) {
  // Arrange

  // Act
  stream_.write_single_bit();

  // Assert
  EXPECT_EQ(0x80, *stream_.raw_bytes());
}

TEST_F(BStreamFixture, WriteByte) {
  // Arrange

  // Act
  stream_.write_byte(1);

  // Assert
  EXPECT_EQ(1, *stream_.raw_bytes());
}

TEST_F(BStreamFixture, WriteByteToNextByte) {
  // Arrange
  stream_.write_zero_bit();

  // Act
  stream_.write_byte(1);

  // Assert
  EXPECT_EQ(0x00, stream_.raw_bytes()[0]);
  EXPECT_EQ(0x80, stream_.raw_bytes()[1]);
}

TEST_F(BStreamFixture, WriteByteToNextByte2) {
  // Arrange
  stream_.write_zero_bit();
  stream_.write_zero_bit();
  stream_.write_zero_bit();
  stream_.write_zero_bit();
  stream_.write_zero_bit();
  stream_.write_zero_bit();
  stream_.write_zero_bit();

  // Act
  stream_.write_byte(1);

  // Assert
  EXPECT_EQ(0x00, stream_.raw_bytes()[0]);
  EXPECT_EQ(0x02, stream_.raw_bytes()[1]);
}

TEST_F(BStreamFixture, WriteBits) {
  // Arrange

  // Act
  stream_.write_bits(59, 9);

  // Assert
  EXPECT_EQ(0x1D, stream_.raw_bytes()[0]);
  EXPECT_EQ(0x80, stream_.raw_bytes()[1]);
}

TEST_F(BStreamFixture, WriteBits2) {
  // Arrange

  // Act
  stream_.write_bits(0b101010101010101, 15);

  // Assert
  EXPECT_EQ(0xAA, stream_.raw_bytes()[0]);
  EXPECT_EQ(0xAA, stream_.raw_bytes()[1]);
}

}  // namespace