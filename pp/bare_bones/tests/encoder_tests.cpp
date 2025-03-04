#include <random>
#include <ranges>
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
  static std::vector<value_type> make_data(std::string_view key) {
    std::vector<value_type> data;

    if (key == "empty") {
      return data;
    }
    if (key == "increment") {
      for (value_type i = 0; i < NUM_VALUES; ++i) {
        data.push_back(i);
      }
      return data;
    }
    if (key == "random") {
      std::mt19937 gen32(testing::UnitTest::GetInstance()->random_seed());
      for (value_type i = 0; i < NUM_VALUES; ++i) {
        // use half of value for correct ZigZag
        data.push_back(gen32() / 2);
      }
      std::sort(data.begin(), data.end());
      return data;
    }
    throw std::invalid_argument(std::string(key));
  }
};

using EncoderTypes = testing::Types<BareBones::EncodedSequence<BareBones::Encoding::RLE<DataSequence>>,
                                    BareBones::EncodedSequence<BareBones::Encoding::DeltaRLE<DataSequence>>,
                                    BareBones::EncodedSequence<BareBones::Encoding::DeltaZigZagRLE<DataSequence>>,
                                    BareBones::EncodedSequence<BareBones::Encoding::RLE<DataSequence64>>,
                                    BareBones::EncodedSequence<BareBones::Encoding::DeltaRLE<DataSequence64>>,
                                    BareBones::EncodedSequence<BareBones::Encoding::DeltaZigZagRLE<DataSequence64>>,
                                    BareBones::EncodedSequence<BareBones::Encoding::RLE<DataSequence64Mostly1>>,
                                    BareBones::EncodedSequence<BareBones::Encoding::DeltaRLE<DataSequence64Mostly1>>,
                                    BareBones::EncodedSequence<BareBones::Encoding::DeltaZigZagRLE<DataSequence64Mostly1>>>;
TYPED_TEST_SUITE(EncoderTest, EncoderTypes);

TYPED_TEST(EncoderTest, should_keep_push_back_order) {
  for (constexpr std::array cases{"empty", "increment", "random"}; auto key : cases) {
    SCOPED_TRACE(key);

    // Arrange
    const auto etalons = EncoderTest<TypeParam>::make_data(key);

    // Act
    TypeParam outcomes;
    std::ranges::copy(etalons, std::back_insert_iterator(outcomes));

    // Assert
    ASSERT_TRUE(std::ranges::equal(etalons, outcomes));
  }
}

TYPED_TEST(EncoderTest, should_dump_and_restore) {
  for (constexpr std::array cases{"empty", "increment", "random"}; auto key : cases) {
    SCOPED_TRACE(key);

    // Arrange
    const auto etalons = EncoderTest<TypeParam>::make_data(key);
    TypeParam src_outcomes, dst_outcomes;
    std::ranges::copy(etalons, std::back_insert_iterator(src_outcomes));
    auto save_size = src_outcomes.save_size();

    // Act
    std::ostringstream out;
    out << src_outcomes;
    ASSERT_EQ(out.str().length(), save_size);
    std::istringstream in(out.str());
    in >> dst_outcomes;

    // Assert
    EXPECT_TRUE(std::ranges::equal(src_outcomes, dst_outcomes));
  }
}

constexpr std::array kEtalonsBoundaryValues = {1UL,        255UL,        256UL,        65535UL,         65536UL,         16777215UL,
                                               16777216UL, 4294967295UL, 4294967296UL, 1099511627775UL, 1099511627776UL, 9223372036854775807UL};

template <class T>
class EncoderTest64 : public testing::Test {};

using EncoderTypes64 = testing::Types<BareBones::EncodedSequence<BareBones::Encoding::RLE<DataSequence64>>,
                                      BareBones::EncodedSequence<BareBones::Encoding::DeltaRLE<DataSequence64>>,
                                      BareBones::EncodedSequence<BareBones::Encoding::DeltaZigZagRLE<DataSequence64>>,
                                      BareBones::EncodedSequence<BareBones::Encoding::RLE<DataSequence64Mostly1>>,
                                      BareBones::EncodedSequence<BareBones::Encoding::DeltaRLE<DataSequence64Mostly1>>,
                                      BareBones::EncodedSequence<BareBones::Encoding::DeltaZigZagRLE<DataSequence64Mostly1>>>;
TYPED_TEST_SUITE(EncoderTest64, EncoderTypes64);

TYPED_TEST(EncoderTest64, boundary_values) {
  // Arrange
  TypeParam outcomes;

  // Act
  std::ranges::copy(kEtalonsBoundaryValues, std::back_insert_iterator(outcomes));

  // Assert
  EXPECT_TRUE(std::ranges::equal(outcomes, kEtalonsBoundaryValues));
}

}  // namespace
