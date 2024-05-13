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

TEST_F(BitsetFixture, should_resize_up_to_10_percent) {
  uint32_t prev_cap = 0;
  uint32_t curr_cap = 0;

  for (auto i = 0; i <= 10239; ++i) {
    if (bs_.capacity() > curr_cap) {
      prev_cap = curr_cap;
      curr_cap = bs_.capacity();
    }
    bs_.resize(static_cast<uint32_t>(i + 1));
    bs_.set(static_cast<uint32_t>(i));
  }

  ASSERT_GE((((bs_.capacity() - prev_cap) / (prev_cap * 1.0)) * 100), 10);
  ASSERT_EQ(bs_.capacity() % 32, 0U);
}

TEST_F(BitsetFixture, valid_unset) {
  const int size = 65536;

  bs_.resize(size);
  for (auto i = 0; i < size; ++i) {
    bs_.set(i);
  }

  bs_.clear();
  ASSERT_EQ(std::ranges::distance(bs_.begin(), bs_.end()), 0);
}

TEST_F(BitsetFixture, random_access) {
  std::vector<uint32_t> index;
  const int bs_size = 65536;

  std::mt19937 gen32(testing::UnitTest::GetInstance()->random_seed());
  // Generate index for bit position
  index.reserve(bs_size);
  for (auto i = 0; i < (bs_size); ++i) {
    index.push_back(gen32() % (bs_size - 1));
  }

  // Sort and uniq index
  std::sort(index.begin(), index.end());
  index.erase(std::unique(index.begin(), index.end()), index.end());

  // Filling the bitset from index
  bs_.resize(bs_size);
  for (auto i : index) {
    bs_.set(i);
  }

  // Prepare result vector from index
  std::vector<uint32_t> bs_vec_reference;
  bs_vec_reference.reserve(index.size());
  for (auto i : index) {
    bs_vec_reference.push_back(i);
  }

  // Save bit possition from bitset to result vector
  std::vector<uint32_t> bs_vec;
  for (auto i : bs_) {
    bs_vec.push_back(i);
  }

  ASSERT_EQ(bs_vec, bs_vec_reference);
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
  std::vector<uint32_t> bs_vec_reference = {1};

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
  for (auto i : bs_) {
    count += i;
  }

  ASSERT_EQ(count, 0U);
}

TEST_F(BitsetFixture, should_iterate_over_resized_empty) {
  bs_.resize(10);
  bs_.resize(0);

  uint32_t count = 0;
  for (auto i : bs_) {
    count += i;
  }

  ASSERT_EQ(count, 0U);
}

TEST_F(BitsetFixture, should_not_out_of_range_on_resize) {
  bs_.resize(2048);
  bs_.set(2047);
  bs_.resize(4096);

  uint32_t count = 0;
  for (auto i : bs_) {
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

}  // namespace
