#include <iostream>
#include <random>

#include <gtest/gtest.h>

#include <ranges>

#include "bare_bones/sparse_vector.h"
#include "bare_bones/vector.h"

namespace {

TEST(BareBonesSparseVector, should_access_sorted_keys) {
  BareBones::SparseVector<uint32_t, BareBones::Vector> sv;
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
  BareBones::SparseVector<uint32_t, BareBones::Vector> sv;
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
  // Arrange
  constexpr std::array values = {124906U, 315107U, 1006518U, 1723575U, 2586253U, 5161614U, 5703086U, 6539749U, 7322267U, 7929935U};
  static_assert(std::ranges::is_sorted(values));
  BareBones::SparseVector<uint32_t, BareBones::Vector> sv;

  // Act
  sv.resize(values.back() + 1);
  for (const auto value : values) {
    sv[value] = value;
  }

  // Assert
  EXPECT_TRUE(std::ranges::equal(values, std::ranges::views::transform(values, [&](auto i) { return sv[i]; })));
}

TEST(BareBonesSparseVector, should_increase_decriase_size) {
  BareBones::SparseVector<uint32_t, BareBones::Vector> sv;
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
  BareBones::SparseVector<uint32_t, BareBones::Vector> sv;
  uint32_t count = 0;
  for (auto i : sv) {
    count += i.second;
  }
  ASSERT_EQ(count, 0);
}
}  // namespace
