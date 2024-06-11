#include <gtest/gtest.h>

#include "bare_bones/gorilla.h"
#include "series_data/encoder/value/uint32_constant.h"

namespace {

using BareBones::Encoding::Gorilla::STALE_NAN;
using series_data::encoder::value::Uint32ConstantEncoder;

struct CanBeEncodedCase {
  double value;
  bool expected;
};

class Uint32ConstantEncoderCanBeEncodedFixture : public testing::TestWithParam<CanBeEncodedCase> {};

TEST_P(Uint32ConstantEncoderCanBeEncodedFixture, Test) {
  // Arrange
  auto& test_case = GetParam();

  uint64_t gg = 0x43F0000000000000;
  double value = 18446700000000000000.0;  // 1.84467e+19;
  std::cin >> value;
  // scanf("%lf", &value);
  std::cout << std::bit_cast<double>(gg) << ", can be encoded: " << Uint32ConstantEncoder::can_be_encoded(std::bit_cast<double>(gg))
            << ", uint64_t: " << static_cast<uint64_t>(value) << std::endl;

  // Act
  auto result = Uint32ConstantEncoder::can_be_encoded(test_case.value);

  // Assert
  EXPECT_EQ(test_case.expected, result);
}

INSTANTIATE_TEST_SUITE_P(Tests,
                         Uint32ConstantEncoderCanBeEncodedFixture,
                         testing::Values(CanBeEncodedCase{.value = STALE_NAN, .expected = false},
                                         CanBeEncodedCase{.value = 0.0, .expected = true},
                                         CanBeEncodedCase{.value = 1.0, .expected = true},
                                         CanBeEncodedCase{.value = 1.1, .expected = false},
                                         CanBeEncodedCase{.value = -1.0, .expected = false},
                                         CanBeEncodedCase{.value = std::numeric_limits<uint32_t>::max(), .expected = true},
                                         CanBeEncodedCase{.value = std::numeric_limits<uint32_t>::max() + 1.0, .expected = false}));

struct EncodeCase {
  double initial_value;
  double value;
  bool expected;
};

class Uint32ConstantEncoderEncodeAndIsActualFixture : public testing::TestWithParam<EncodeCase> {
 protected:
  Uint32ConstantEncoder encoder_{GetParam().initial_value};
};

TEST_P(Uint32ConstantEncoderEncodeAndIsActualFixture, Encode) {
  // Arrange

  // Act
  auto result = encoder_.encode(GetParam().value);

  // Assert
  EXPECT_EQ(GetParam().expected, result);
}

TEST_P(Uint32ConstantEncoderEncodeAndIsActualFixture, IsActual) {
  // Arrange

  // Act
  auto result = encoder_.is_actual(GetParam().value);

  // Assert
  EXPECT_EQ(GetParam().expected, result);
}

INSTANTIATE_TEST_SUITE_P(
    Test,
    Uint32ConstantEncoderEncodeAndIsActualFixture,
    testing::Values(EncodeCase{.initial_value = 0.0, .value = 0.0, .expected = true},
                    EncodeCase{.initial_value = 0.0, .value = 1.0, .expected = false},
                    EncodeCase{.initial_value = 0.0, .value = STALE_NAN, .expected = false},
                    EncodeCase{.initial_value = 0.0, .value = -1.0, .expected = false},
                    EncodeCase{.initial_value = std::numeric_limits<uint32_t>::max(), .value = std::numeric_limits<uint32_t>::max() + 1.0, .expected = false},
                    EncodeCase{.initial_value = std::numeric_limits<uint32_t>::max(), .value = std::numeric_limits<uint32_t>::max(), .expected = true}));

}  // namespace