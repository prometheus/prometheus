#include <iterator>
#include <random>

#include <gtest/gtest.h>

#include "bare_bones/vector.h"

namespace {

TEST(BareBonesVector, should_increase_capacity_up_50_percent) {
  BareBones::Vector<uint8_t> vec;
  size_t prev_cap = 0;
  size_t curr_cap = 0;

  for (int i = 0; i <= 64; ++i) {
    vec.push_back(1);
    if (vec.capacity() > curr_cap) {
      prev_cap = curr_cap;
      curr_cap = vec.capacity();
    }
  }

  ASSERT_EQ(vec.capacity() % 32, 0);
  ASSERT_GE((((vec.capacity() - prev_cap) / (prev_cap * 1.0)) * 100), 50);
}

TEST(BareBonesVector, should_increase_capacity_up_10_percent) {
  BareBones::Vector<uint8_t> vec;
  size_t prev_cap = 0, curr_cap = 0;

  for (int i = 0; i <= 81920; ++i) {
    vec.push_back(1);
    if (vec.capacity() > curr_cap) {
      prev_cap = curr_cap;
      curr_cap = vec.capacity();
    }
  }

  ASSERT_EQ(vec.capacity() % 4096, 0);
  ASSERT_GE((((vec.capacity() - prev_cap) / (prev_cap * 1.0)) * 100), 10);
}
}  // namespace
