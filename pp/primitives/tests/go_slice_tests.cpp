#include <gtest/gtest.h>

#include "primitives/go_slice.h"

namespace {

using PromPP::Primitives::Go::BytesStream;
using PromPP::Primitives::Go::Slice;

class BytesStreamFixture : public testing::Test {
 protected:
  Slice<char> slice_;
  BytesStream stream_{&slice_};
};

TEST_F(BytesStreamFixture, Test1) {
  // Arrange
  constexpr std::string_view expected = "Hello!5123.456";
  stream_.exceptions(std::ifstream::failbit | std::ifstream::badbit | std::ifstream::eofbit);

  // Act
  stream_ << "Hello";
  stream_ << '!';
  stream_ << 5;
  stream_ << 123.456;

  // Assert
  EXPECT_EQ(expected, std::string_view(slice_.data(), slice_.size()));
  EXPECT_EQ(expected.size(), stream_.tellp());
}

}  // namespace
