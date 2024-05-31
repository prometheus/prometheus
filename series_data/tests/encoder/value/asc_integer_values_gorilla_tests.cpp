#include <gtest/gtest.h>

#include "series_data/encoder/value/asc_integer_values_gorilla.h"

namespace {

using BareBones::Encoding::Gorilla::STALE_NAN;
using series_data::encoder::value::AscIntegerValuesGorillaDecoder;
using series_data::encoder::value::AscIntegerValuesGorillaEncoder;

struct CanBeEncodedCase {
  double value1;
  uint8_t value1_count;
  double value2;
  double value3;
  bool expected;
};

class AscIntegerValuesGorillaEncoderCanBeEncodedFixture : public testing::TestWithParam<CanBeEncodedCase> {};

TEST_P(AscIntegerValuesGorillaEncoderCanBeEncodedFixture, Test) {
  // Arrange
  auto& test_case = GetParam();

  // Act
  auto result = AscIntegerValuesGorillaEncoder::can_be_encoded(test_case.value1, test_case.value1_count, test_case.value2, test_case.value3);

  // Assert
  EXPECT_EQ(test_case.expected, result);
}

INSTANTIATE_TEST_SUITE_P(NoStaleNans,
                         AscIntegerValuesGorillaEncoderCanBeEncodedFixture,
                         testing::Values(CanBeEncodedCase{.value1 = 0.1, .value1_count = 1, .value2 = 0.0, .value3 = 1.0, .expected = false},
                                         CanBeEncodedCase{.value1 = 0.0, .value1_count = 1, .value2 = 0.1, .value3 = 2.0, .expected = false},
                                         CanBeEncodedCase{.value1 = 0.0, .value1_count = 1, .value2 = 1.0, .value3 = 1.1, .expected = false},
                                         CanBeEncodedCase{.value1 = 1.0, .value1_count = 1, .value2 = 0.0, .value3 = 2.0, .expected = false},
                                         CanBeEncodedCase{.value1 = 0.0, .value1_count = 1, .value2 = 0.0, .value3 = 2.0, .expected = true},
                                         CanBeEncodedCase{.value1 = 0.0, .value1_count = 1, .value2 = 1.0, .value3 = 0.0, .expected = false},
                                         CanBeEncodedCase{.value1 = 0.0, .value1_count = 1, .value2 = 1.0, .value3 = 1.0, .expected = true}));
INSTANTIATE_TEST_SUITE_P(StaleNans,
                         AscIntegerValuesGorillaEncoderCanBeEncodedFixture,
                         testing::Values(CanBeEncodedCase{.value1 = STALE_NAN, .value1_count = 1, .value2 = 1.0, .value3 = 3.0, .expected = false},
                                         CanBeEncodedCase{.value1 = 0.0, .value1_count = 1, .value2 = STALE_NAN, .value3 = 3.0, .expected = false},
                                         CanBeEncodedCase{.value1 = 0.0, .value1_count = 2, .value2 = STALE_NAN, .value3 = 3.0, .expected = true},
                                         CanBeEncodedCase{.value1 = 0.0, .value1_count = 1, .value2 = 1.0, .value3 = STALE_NAN, .expected = true}));

struct IsActualCase {
  BareBones::Vector<double> values;
  double value;
  bool expected;
};

class AscIntegerValuesGorillaEncoderIsActualFixture : public testing::TestWithParam<IsActualCase> {
 protected:
  static AscIntegerValuesGorillaEncoder encode(const BareBones::Vector<double>& values) {
    AscIntegerValuesGorillaEncoder encoder(values[0]);
    encoder.encode_second(values[1]);
    for (size_t i = 2; i < values.size(); ++i) {
      encoder.encode(values[i]);
    }
    return encoder;
  }
};

TEST_P(AscIntegerValuesGorillaEncoderIsActualFixture, Test) {
  // Arrange
  auto encoder = encode(GetParam().values);

  // Act
  auto result = encoder.is_actual(GetParam().value);

  // Assert
  EXPECT_EQ(GetParam().expected, result);
}

INSTANTIATE_TEST_SUITE_P(TwoPoints,
                         AscIntegerValuesGorillaEncoderIsActualFixture,
                         testing::Values(IsActualCase{.values = {1.0, 2.0}, .value = 1.0, .expected = false},
                                         IsActualCase{.values = {1.0, 2.0}, .value = 2.0, .expected = true},
                                         IsActualCase{.values = {1.0, STALE_NAN}, .value = 1.0, .expected = false},
                                         IsActualCase{.values = {1.0, STALE_NAN}, .value = STALE_NAN, .expected = true}));

INSTANTIATE_TEST_SUITE_P(ThreePoints,
                         AscIntegerValuesGorillaEncoderIsActualFixture,
                         testing::Values(IsActualCase{.values = {1.0, 2.0, 3.0}, .value = 2.0, .expected = false},
                                         IsActualCase{.values = {1.0, 2.0, 3.0}, .value = 3.0, .expected = true},
                                         IsActualCase{.values = {1.0, 2.0, STALE_NAN}, .value = 1.0, .expected = false},
                                         IsActualCase{.values = {1.0, 2.0, STALE_NAN}, .value = STALE_NAN, .expected = true},
                                         IsActualCase{.values = {1.0, STALE_NAN, 2.0}, .value = 2.0, .expected = true},
                                         IsActualCase{.values = {1.0, STALE_NAN, 2.0}, .value = STALE_NAN, .expected = false}));

INSTANTIATE_TEST_SUITE_P(NonIntegerValue,
                         AscIntegerValuesGorillaEncoderIsActualFixture,
                         testing::Values(IsActualCase{.values = {-1.0, 0.0}, .value = -1.1, .expected = false}));

struct UsageCase {
  BareBones::Vector<double> values;
};

class AscIntegerValuesGorillaEncoderUsage : public testing::TestWithParam<UsageCase> {};

TEST_P(AscIntegerValuesGorillaEncoderUsage, Test) {
  // Arrange
  auto& values = GetParam().values;
  AscIntegerValuesGorillaEncoder encoder(values[0]);

  // Act
  encoder.encode_second(values[1]);
  for (size_t i = 2; i < values.size(); ++i) {
    encoder.encode(values[i]);
  }
  auto decoded = AscIntegerValuesGorillaDecoder::decode_all(encoder.stream().reader());

  // Assert
  EXPECT_TRUE(std::ranges::equal(values, decoded, [](double a, double b) { return std::bit_cast<uint64_t>(a) == std::bit_cast<uint64_t>(b); }));
}

INSTANTIATE_TEST_SUITE_P(Tests,
                         AscIntegerValuesGorillaEncoderUsage,
                         testing::Values(UsageCase{.values = {1.0, 2.0, 3.0}},
                                         UsageCase{.values = {1.0, 2.0, STALE_NAN}},
                                         UsageCase{.values = {1.0, 2.0, STALE_NAN, 3.0, STALE_NAN, 3.0}}));

}  // namespace
