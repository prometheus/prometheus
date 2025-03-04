#include <algorithm>
#include <iterator>
#include <random>
#include <string>
#include <vector>

#include <gtest/gtest.h>

#include "bare_bones/encoding.h"

namespace {

using DataSequence = BareBones::StreamVByte::Sequence<BareBones::StreamVByte::Codec0124Frequent0>;
using DataSequence64 = BareBones::StreamVByte::Sequence<BareBones::StreamVByte::Codec1238>;
using DataSequence64Mostly1 = BareBones::StreamVByte::Sequence<BareBones::StreamVByte::Codec1238Mostly1>;
template <class T>
class EncoderTest : public testing::Test {
  using value_type = typename T::value_type;

  static constexpr value_type NUM_VALUES = 100;

 public:
  static std::vector<value_type> make_data(const char* key) {
    std::vector<value_type> data;

    if (strcmp(key, "empty") == 0) {
      return data;
    }
    if (strcmp(key, "increment") == 0) {
      for (value_type i = 0; i < NUM_VALUES; ++i) {
        data.push_back(i);
      }
      return data;
    }
    if (strcmp(key, "random") == 0) {
      std::mt19937 gen32(testing::UnitTest::GetInstance()->random_seed());
      for (value_type i = 0; i < NUM_VALUES; ++i) {
        // use half of value for correct ZigZag
        data.push_back(gen32() / 2);
      }
      std::sort(data.begin(), data.end());
      return data;
    }
    throw std::invalid_argument(key);
  }
};

typedef testing::Types<BareBones::EncodedSequence<BareBones::Encoding::RLE<DataSequence>>,
                       BareBones::EncodedSequence<BareBones::Encoding::DeltaRLE<DataSequence>>,
                       BareBones::EncodedSequence<BareBones::Encoding::DeltaZigZagRLE<DataSequence>>,
                       BareBones::EncodedSequence<BareBones::Encoding::RLE<DataSequence64>>,
                       BareBones::EncodedSequence<BareBones::Encoding::DeltaRLE<DataSequence64>>,
                       BareBones::EncodedSequence<BareBones::Encoding::DeltaZigZagRLE<DataSequence64>>,
                       BareBones::EncodedSequence<BareBones::Encoding::RLE<DataSequence64Mostly1>>,
                       BareBones::EncodedSequence<BareBones::Encoding::DeltaRLE<DataSequence64Mostly1>>,
                       BareBones::EncodedSequence<BareBones::Encoding::DeltaZigZagRLE<DataSequence64Mostly1>>>
    EncoderTypes;
TYPED_TEST_SUITE(EncoderTest, EncoderTypes);

TYPED_TEST(EncoderTest, should_keep_push_back_order) {
  const char* cases[] = {"empty", "increment", "random"};
  for (auto key : cases) {
    SCOPED_TRACE(key);
    const auto etalons = EncoderTest<TypeParam>::make_data(key);

    TypeParam outcomes;
    for (const auto& x : etalons) {
      outcomes.push_back(x);
    }
    // TODO: Need understand - why it's doesn't work
    // ASSERT_TRUE(std::ranges::equal(std::begin(data), std::end(data), std::begin(actual), std::end(actual)));
    auto etalon = etalons.begin();
    auto outcome = outcomes.begin();
    size_t count = 0;
    while (etalon != etalons.end() && outcome != outcomes.end()) {
      EXPECT_EQ(*etalon++, *outcome++);
      count++;
    }
    EXPECT_EQ(etalon == etalons.end(), outcome == outcomes.end());
    EXPECT_EQ(etalons.size(), count);
  }
}

TYPED_TEST(EncoderTest, should_dump_and_restore) {
  const char* cases[] = {"empty", "increment", "random"};
  for (auto key : cases) {
    SCOPED_TRACE(key);
    const auto etalons = EncoderTest<TypeParam>::make_data(key);

    TypeParam src_outcomes, dst_outcomes;
    for (const auto& etalon : etalons) {
      src_outcomes.push_back(etalon);
    }
    auto save_size = src_outcomes.save_size();

    std::ostringstream out;
    out << src_outcomes;
    ASSERT_EQ(out.str().length(), save_size);
    std::istringstream in(out.str());
    in >> dst_outcomes;

    auto src_outcome = src_outcomes.begin();
    auto dst_outcome = dst_outcomes.begin();
    while (src_outcome != src_outcomes.end() && dst_outcome != dst_outcomes.end()) {
      EXPECT_EQ(*src_outcome++, *dst_outcome++);
    }
    EXPECT_EQ(src_outcome == src_outcomes.end(), dst_outcome == dst_outcomes.end());
  }
}

static const std::vector<uint64_t> ETALONS_BOUNDARY_VALUES = {1,        255,        256,        65535,         65536,         16777215,
                                                              16777216, 4294967295, 4294967296, 1099511627775, 1099511627776, 9223372036854775807};

template <class T>
class EncoderTest64 : public testing::Test {};

typedef testing::Types<BareBones::EncodedSequence<BareBones::Encoding::RLE<DataSequence64>>,
                       BareBones::EncodedSequence<BareBones::Encoding::DeltaRLE<DataSequence64>>,
                       BareBones::EncodedSequence<BareBones::Encoding::DeltaZigZagRLE<DataSequence64>>,
                       BareBones::EncodedSequence<BareBones::Encoding::RLE<DataSequence64Mostly1>>,
                       BareBones::EncodedSequence<BareBones::Encoding::DeltaRLE<DataSequence64Mostly1>>,
                       BareBones::EncodedSequence<BareBones::Encoding::DeltaZigZagRLE<DataSequence64Mostly1>>>
    EncoderTypes64;
TYPED_TEST_SUITE(EncoderTest64, EncoderTypes64);

TYPED_TEST(EncoderTest64, boundary_values) {
  TypeParam outcomes;
  for (const auto& x : ETALONS_BOUNDARY_VALUES) {
    outcomes.push_back(x);
  }

  auto etalon = ETALONS_BOUNDARY_VALUES.begin();
  auto outcome = outcomes.begin();
  size_t count = 0;
  while (etalon != ETALONS_BOUNDARY_VALUES.end() && outcome != outcomes.end()) {
    EXPECT_EQ(*etalon++, *outcome++);
    count++;
  }

  EXPECT_EQ(etalon == ETALONS_BOUNDARY_VALUES.end(), outcome == outcomes.end());
  EXPECT_EQ(ETALONS_BOUNDARY_VALUES.size(), count);
}

}  // namespace
