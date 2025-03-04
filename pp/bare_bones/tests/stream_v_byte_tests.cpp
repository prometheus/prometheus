#include <iterator>
#include <list>
#include <string>
#include <vector>

#include <ranges>

#include <gtest/gtest.h>

#include "bare_bones/stream_v_byte.h"

namespace {

using BareBones::StreamVByte::CompactSequence;
using BareBones::StreamVByte::Sequence;

const std::vector<uint64_t> ETALONS_BOUNDARY_VALUES = {1,        255,        256,        65535,         65536,         16777215,
                                                       16777216, 4294967295, 4294967296, 1099511627775, 1099511627776, 9223372036854775807};

template <class T>
class SequenceFixture : public testing::Test {};

using SequenceCodecTypes = testing::Types<Sequence<BareBones::StreamVByte::Codec1238>, Sequence<BareBones::StreamVByte::Codec1238Mostly1>>;
TYPED_TEST_SUITE(SequenceFixture, SequenceCodecTypes);

TYPED_TEST(SequenceFixture, Codec64) {
  TypeParam outcomes;

  for (auto& etalon : ETALONS_BOUNDARY_VALUES) {
    outcomes.push_back(etalon);
  }

  EXPECT_TRUE(std::ranges::equal(ETALONS_BOUNDARY_VALUES, outcomes));
}

template <class T>
class Container : public testing::Test {};

using ContainerCodecTypes = testing::Types<std::vector<uint8_t>, BareBones::Vector<uint8_t>>;

TYPED_TEST_SUITE(Container, ContainerCodecTypes);

TYPED_TEST(Container, Codec64) {
  TypeParam outcomes;

  auto out = BareBones::StreamVByte::back_inserter<BareBones::StreamVByte::Codec1238>(outcomes, ETALONS_BOUNDARY_VALUES.size());
  for (auto& etalon : ETALONS_BOUNDARY_VALUES) {
    *out++ = etalon;
  }

  auto [outcomes_b_i, outcomes_e_i] = BareBones::StreamVByte::decoder<BareBones::StreamVByte::Codec1238>(outcomes.begin(), ETALONS_BOUNDARY_VALUES.size());

  EXPECT_TRUE(std::ranges::equal(std::begin(ETALONS_BOUNDARY_VALUES), std::end(ETALONS_BOUNDARY_VALUES), outcomes_b_i, outcomes_e_i));
}

class SequenceIotaFixture : public ::testing::TestWithParam<std::ranges::iota_view<uint32_t, uint32_t>> {
 protected:
  Sequence<BareBones::StreamVByte::Codec0124, 4> sequence_;
};

TEST_P(SequenceIotaFixture, TestIota) {
  // Arrange

  // Act
  std::ranges::copy(GetParam(), std::back_insert_iterator(sequence_));

  // Assert
  EXPECT_TRUE(std::ranges::equal(GetParam(), sequence_));
}

class CompactSequenceIotaFixture : public ::testing::TestWithParam<std::ranges::iota_view<uint32_t, uint32_t>> {
 protected:
  CompactSequence<BareBones::StreamVByte::Codec0124, 4> sequence_;
};

TEST_P(CompactSequenceIotaFixture, TestIota) {
  // Arrange

  // Act
  std::ranges::copy(GetParam(), std::back_insert_iterator(sequence_));

  // Assert
  EXPECT_TRUE(std::ranges::equal(GetParam(), sequence_));
}

const auto kIotaCases = testing::Values(std::views::iota(0U, 0U),
                                        std::views::iota(0U, 3U),
                                        std::views::iota(0U, 4U),
                                        std::views::iota(0U, 5U),
                                        std::views::iota(0U, 8U),
                                        std::views::iota(0U, 9U),
                                        std::views::iota(std::numeric_limits<uint32_t>::max() - 4, std::numeric_limits<uint32_t>::max()));

INSTANTIATE_TEST_SUITE_P(Cases, SequenceIotaFixture, kIotaCases);
INSTANTIATE_TEST_SUITE_P(Cases, CompactSequenceIotaFixture, kIotaCases);

}  // namespace
