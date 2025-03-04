#include <algorithm>
#include <random>
#include <string>
#include <vector>

#include <gtest/gtest.h>

#include "bare_bones/bitset.h"

namespace {

class BitsetFixture : public testing::Test {
 protected:
  BareBones::Bitset bs_;
};

TEST_F(BitsetFixture, valid_unset) {
  constexpr int size = 65536;

  bs_.resize(size);
  for (auto i = 0; i < size; ++i) {
    bs_.set(i);
  }

  bs_.clear();
  ASSERT_EQ(std::ranges::distance(bs_.begin(), bs_.end()), 0);
}

TEST_F(BitsetFixture, random_access) {
  // Arrange
  constexpr std::array indexes = {0U, 134U, 1087U, 5378U, 12098U, 65535U};
  static_assert(std::ranges::is_sorted(indexes));

  // Act
  bs_.resize(indexes.back() + 1);
  for (const auto index : indexes) {
    bs_.set(index);
  }

  // Assert
  EXPECT_TRUE(std::ranges::equal(bs_, indexes));
}

TEST_F(BitsetFixture, should_decriase_increase_size) {
  bs_.resize(1024);
  // set every odd bit
  for (auto i = 0; i < 1024; ++i) {
    if (i % 2) {
      bs_.set(i);
    }
  }

  // Check decrease resize
  // after resize we have bitset {0,1}
  bs_.resize(2);
  ASSERT_EQ(bs_.size(), 2U);

  // Check increase resize
  bs_.resize(4);
  // after resize we have bitset {0,1,0,0}
  // create vector that include all position of set bits
  const std::vector<uint32_t> bs_vec_reference = {1};

  // save possition of set original bitset to vector
  std::vector<uint32_t> bs_vec;
  for (auto i : bs_) {
    bs_vec.push_back(i);
  }

  ASSERT_EQ(bs_vec, bs_vec_reference);
  ASSERT_EQ(bs_.size(), 4U);
}

TEST_F(BitsetFixture, should_iterate_over_empty) {
  uint32_t count = 0;
  for (const auto i : bs_) {
    count += i;
  }

  ASSERT_EQ(count, 0U);
}

TEST_F(BitsetFixture, should_iterate_over_resized_empty) {
  bs_.resize(10);
  bs_.resize(0);

  uint32_t count = 0;
  for (const auto i : bs_) {
    count += i;
  }

  ASSERT_EQ(count, 0U);
}

TEST_F(BitsetFixture, should_not_out_of_range_on_resize) {
  bs_.resize(2048);
  bs_.set(2047);
  bs_.resize(4096);

  uint32_t count = 0;
  for (const auto i : bs_) {
    count += i;
  }

  ASSERT_EQ(count, 2047U);
}

TEST_F(BitsetFixture, ResetInFirstUint64) {
  // Arrange
  bs_.resize(1);

  // Act
  bs_.set(0);
  bs_.reset(0);

  // Assert
  EXPECT_FALSE(bs_[0]);
}

TEST_F(BitsetFixture, ResetInSecondUint64) {
  // Arrange
  bs_.resize(65);

  // Act
  bs_.set(64);
  bs_.reset(64);

  // Assert
  EXPECT_FALSE(bs_[64]);
}

TEST_F(BitsetFixture, TestResetCorrectness) {
  // Arrange
  bs_.resize(3);

  // Act
  bs_.set(0);
  bs_.set(1);
  bs_.set(2);
  bs_.reset(1);

  // Assert
  EXPECT_TRUE(bs_[0]);
  EXPECT_FALSE(bs_[1]);
  EXPECT_TRUE(bs_[2]);
}

TEST_F(BitsetFixture, PopcountOnEmptyBitset) {
  // Arrange
  bs_.resize(1);

  // Act

  // Assert
  EXPECT_EQ(0U, bs_.popcount());
}

TEST_F(BitsetFixture, PopcountOnNonEmptyBitset) {
  // Arrange
  bs_.resize(9);

  // Act
  bs_.set(0);
  bs_.set(4);
  bs_.set(8);

  // Assert
  EXPECT_EQ(3U, bs_.popcount());
}

TEST_F(BitsetFixture, PopcountAfterResizeInCurrentUint64) {
  // Arrange
  bs_.resize(1);

  // Act
  bs_.set(0);
  bs_.resize(0);

  // Assert
  EXPECT_EQ(0U, bs_.popcount());
}

TEST_F(BitsetFixture, PopcountAfterResizeInNextUint64) {
  // Arrange
  bs_.resize(9);

  // Act
  bs_.set(8);
  bs_.resize(0);
  bs_.resize(9);

  // Assert
  EXPECT_EQ(0U, bs_.popcount());
}

}  // namespace
