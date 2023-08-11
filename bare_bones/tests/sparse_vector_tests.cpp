#include <iostream>

#include <gtest/gtest.h>

#include "bare_bones/sparse_vector.h"

namespace {

TEST(BareBonesSparseVector, should_access_sorted_keys) {
  BareBones::SparseVector<uint32_t> sv;
  sv.resize(3);
  sv[0] = 0;
  sv[1] = 1;
  sv[2] = 2;

  auto j = 0;

  for (auto i = sv.begin(); i != sv.end(); ++i, ++j) {
    auto [k, v] = *i;
    ASSERT_EQ(k, j);
    ASSERT_EQ(v, j);
  }
}

TEST(BareBonesSparseVector, valid_clear) {
  BareBones::SparseVector<uint32_t> sv;
  sv.resize(3);
  sv[0] = 0;
  sv[1] = 1;
  sv[2] = 2;

  sv.clear();

  auto count = 0;
  for (auto i = sv.begin(); i != sv.end(); ++i) {
    ++count;
  }
  ASSERT_EQ(count, 0);
}

TEST(BareBonesSparseVector, random_access) {
  const size_t num_values = 10000;
  BareBones::SparseVector<uint32_t> sv;
  std::map<uint32_t, uint32_t> mp;
  std::vector<uint32_t> index;

  std::mt19937 gen32(testing::UnitTest::GetInstance()->random_seed());
  uint32_t max_index = 0;
  for (size_t i = 0; i < num_values; ++i) {
    auto v = gen32() % 8'000'000;
    if (v > max_index) {
      max_index = v;
    }
    index.push_back(v);
  }
  for (auto i = index.begin(); i != index.end(); ++i) {
    mp[*i] = *i;
  }

  sv.resize(max_index + 1);
  for (auto i = index.begin(); i != index.end(); ++i) {
    sv[*i] = *i;
  }

  std::map<uint32_t, uint32_t> mp_sv;
  for (auto i = sv.begin(); i != sv.end(); ++i) {
    auto [in, v] = *i;
    mp_sv[in] = v;
  }

  ASSERT_EQ(mp, mp_sv);
}

TEST(BareBonesSparseVector, should_increase_decriase_size) {
  BareBones::SparseVector<uint32_t> sv;
  sv.resize(3);
  sv[0] = 0;
  sv[1] = 1;
  sv[2] = 2;
  uint32_t count = 0;
  for (auto i = sv.begin(); i != sv.end(); ++i) {
    ++count;
  }
  ASSERT_EQ(count, 3);

  sv.resize(1);
  count = 0;
  for (auto i = sv.begin(); i != sv.end(); ++i) {
    ++count;
  }

  ASSERT_EQ(count, 1);
}

TEST(BareBonesSparseVector, should_iterate_over_empty) {
  BareBones::SparseVector<uint32_t> sv;
  uint32_t count = 0;
  for (auto i : sv) {
    count += i.second;
  }
  ASSERT_EQ(count, 0);
}
}  // namespace
