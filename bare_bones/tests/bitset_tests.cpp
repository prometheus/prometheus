#include <algorithm>
#include <functional>
#include <random>
#include <ranges>
#include <sstream>
#include <string>
#include <vector>

#include <gtest/gtest.h>

#include "bare_bones/bitset.h"

namespace {
TEST(BareBonesBitset, should_resize_up_to_10_percent) {
  BareBones::Bitset bs;
  uint32_t prev_cap = 0;
  uint32_t curr_cap = 0;

  for (auto i = 0; i <= 10239; ++i) {
    if (bs.capacity() > curr_cap) {
      prev_cap = curr_cap;
      curr_cap = bs.capacity();
    }
    bs.resize(static_cast<uint32_t>(i + 1));
    bs.set(static_cast<uint32_t>(i));
  }

  ASSERT_GE((((bs.capacity() - prev_cap) / (prev_cap * 1.0)) * 100), 10);
  ASSERT_EQ(bs.capacity() % 32, 0);
}

TEST(BareBonesBitset, valid_unset) {
  BareBones::Bitset bs;
  const int size = 65536;

  bs.resize(size);
  for (auto i = 0; i < size; ++i) {
    bs.set(i);
  }

  bs.clear();
  ASSERT_EQ(std::ranges::distance(bs.begin(), bs.end()), 0);
}

TEST(BareBonesBitset, random_access) {
  BareBones::Bitset bs;
  std::vector<uint32_t> index;
  const int bs_size = 65536;

  std::mt19937 gen32(testing::UnitTest::GetInstance()->random_seed());
  // Generate index for bit position
  for (auto i = 0; i < (bs_size); ++i) {
    index.push_back(gen32() % (bs_size - 1));
  }

  // Sort and uniq index
  std::sort(index.begin(), index.end());
  index.erase(std::unique(index.begin(), index.end()), index.end());

  // Filling the bitset from index
  bs.resize(bs_size);
  for (auto i : index) {
    bs.set(i);
  }

  // Prepare result vector from index
  std::vector<uint32_t> bs_vec_reference;
  for (auto i : index) {
    bs_vec_reference.push_back(i);
  }

  // Save bit possition from bitset to result vector
  std::vector<uint32_t> bs_vec;
  for (auto i : bs) {
    bs_vec.push_back(i);
  }

  ASSERT_EQ(bs_vec, bs_vec_reference);
}

TEST(BareBonesBitset, should_decriase_increase_size) {
  BareBones::Bitset bs;

  bs.resize(1024);
  // set every odd bit
  for (auto i = 0; i < 1024; ++i) {
    if (i % 2) {
      bs.set(i);
    }
  }

  // Check decrease resize
  // after resize we have bitset {0,1}
  bs.resize(2);
  ASSERT_EQ(bs.size(), 2);

  // Check increase resize
  bs.resize(4);
  // after resize we have bitset {0,1,0,0}
  // create vector that include all position of set bits
  std::vector<uint32_t> bs_vec_reference = {1};

  // save possition of set original bitset to vector
  std::vector<uint32_t> bs_vec;
  for (auto i : bs) {
    bs_vec.push_back(i);
  }

  ASSERT_EQ(bs_vec, bs_vec_reference);
  ASSERT_EQ(bs.size(), 4);
}

TEST(BareBonesBitset, should_iterate_over_empty) {
  BareBones::Bitset bs;

  uint32_t count = 0;
  for (auto i : bs) {
    count += i;
  }

  ASSERT_EQ(count, 0);
}

TEST(BareBonesBitset, should_iterate_over_resized_empty) {
  BareBones::Bitset bs;
  bs.resize(10);
  bs.resize(0);

  uint32_t count = 0;
  for (auto i : bs) {
    count += i;
  }

  ASSERT_EQ(count, 0);
}

TEST(BareBonesTest, should_not_out_of_range_on_resize) {
  BareBones::Bitset bs;
  bs.resize(2048);
  bs.set(2047);
  bs.resize(4096);

  uint32_t count = 0;
  for (auto i : bs) {
    count += i;
  }

  ASSERT_EQ(count, 2047);
}
}  // namespace
