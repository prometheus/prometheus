#include <gtest/gtest.h>

#include "bare_bones/lz4/decompressor_buffer.h"

namespace {

using BareBones::lz4::DecompressorBuffer;

class DecompressorBufferAllocateFixture : public testing::Test {
 protected:
  DecompressorBuffer buffer_{{.shrink_ratio = 0.01, .threshold_size_shrink_ratio = 0.01}};
};

TEST_F(DecompressorBufferAllocateFixture, AllocateNewBuffer)
{
  // Arrange

  // Act
  buffer_.allocate(9);
  buffer_.allocate(10);

  // Assert
  EXPECT_EQ(10U, buffer_.size());
}

TEST_F(DecompressorBufferAllocateFixture, NoAllocateForSmallerSize)
{
  // Arrange

  // Act
  buffer_.allocate(10);
  buffer_.allocate(9);

  // Assert
  EXPECT_EQ(10U, buffer_.size());
}


class DecompressorBufferReallocateFixture : public testing::Test {
 protected:
  void set_string_in_buffer(std::string_view str) {
    buffer_.allocate(str.size());
    memcpy(buffer_.data(), str.data(), str.size());
  }

 protected:
  DecompressorBuffer buffer_{{.shrink_ratio = 0.01, .threshold_size_shrink_ratio = 0.01}};
};

TEST_F(DecompressorBufferReallocateFixture, ReallocateEmptyBuffer) {
  // Arrange

  // Act
  buffer_.reallocate(1);

  // Assert
  EXPECT_EQ(1U, buffer_.size());
}

TEST_F(DecompressorBufferReallocateFixture, ReallocateBuffer) {
  // Arrange
  constexpr std::string_view str{"12345"};
  set_string_in_buffer(str);

  // Act
  buffer_.reallocate(str.size() + 1);

  // Assert
  EXPECT_EQ(str.size() + 1, buffer_.size());
  EXPECT_TRUE(buffer_.view().starts_with(str));
}

TEST_F(DecompressorBufferReallocateFixture, NoReallocateForSmallerSize) {
  // Arrange
  constexpr std::string_view str{"12345"};
  set_string_in_buffer(str);

  // Act
  buffer_.reallocate(str.size() - 1);

  // Assert
  EXPECT_EQ(str.size(), buffer_.size());
  EXPECT_TRUE(buffer_.view().starts_with(str));
}


struct ShrinkTestCase {
  size_t allocate_size;
  size_t expected_size;
};

class DecompressorBufferShrinkFixture : public testing::TestWithParam<ShrinkTestCase> {
 protected:
  DecompressorBuffer buffer_{{.shrink_ratio = 0.3, .threshold_size_shrink_ratio = 0.5}};
};

TEST_P(DecompressorBufferShrinkFixture, Test) {
  // Arrange

  // Act
  buffer_.allocate(100);
  buffer_.allocate(50);
  buffer_.allocate(48);
  buffer_.allocate(GetParam().allocate_size);

  // Assert
  EXPECT_EQ(GetParam().expected_size, buffer_.size());
}

INSTANTIATE_TEST_SUITE_P(NeedShrink,
                         DecompressorBufferShrinkFixture,
                         testing::Values(ShrinkTestCase{.allocate_size = 69, .expected_size = 70}, ShrinkTestCase{.allocate_size = 70, .expected_size = 70}));

INSTANTIATE_TEST_SUITE_P(NoNeedShrink, DecompressorBufferShrinkFixture, testing::Values(ShrinkTestCase{.allocate_size = 71, .expected_size = 100}));

}  // namespace
