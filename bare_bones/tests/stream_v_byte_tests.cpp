#include <iterator>
#include <list>
#include <string>
#include <vector>

#include <gtest/gtest.h>

#include "bare_bones/stream_v_byte.h"

namespace {

static const std::vector<uint64_t> ETALONS_BOUNDARY_VALUES = {1,        255,        256,        65535,         65536,         16777215,
                                                              16777216, 4294967295, 4294967296, 1099511627775, 1099511627776, 9223372036854775807};

template <class T>
class Sequence : public testing::Test {};

typedef testing::Types<BareBones::StreamVByte::Sequence<BareBones::StreamVByte::Codec1238>,
                       BareBones::StreamVByte::Sequence<BareBones::StreamVByte::Codec1238Mostly1>>
    SequenceCodecTypes;
TYPED_TEST_SUITE(Sequence, SequenceCodecTypes);

TYPED_TEST(Sequence, Codec64) {
  TypeParam outcomes;

  for (auto& etalon : ETALONS_BOUNDARY_VALUES) {
    outcomes.push_back(etalon);
  }

  EXPECT_TRUE(std::ranges::equal(std::begin(ETALONS_BOUNDARY_VALUES), std::end(ETALONS_BOUNDARY_VALUES), std::begin(outcomes), std::end(outcomes)));
}

template <class T>
class Container : public testing::Test {};

typedef testing::Types<std::vector<uint8_t>, BareBones::Vector<uint8_t>> ContainerCodecTypes;

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

}  // namespace
