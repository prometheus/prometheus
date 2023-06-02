#include <random>

#include <gtest/gtest.h>

#include "bare_bones/zigzag.h"

namespace {

const size_t NUM_VALUES = 10000;

struct ZigZag : public testing::Test {};

TEST_F(ZigZag, EncodeDecodeInt64T) {
  int64_t val;
  std::mt19937 gen32(testing::UnitTest::GetInstance()->random_seed());

  for (size_t i = 0; i < NUM_VALUES; ++i) {
    if (i % 2 == 0) {
      val = 0 - gen32() / 2;
    } else {
      val = gen32() / 2;
    }

    uint64_t enc_val = BareBones::Encoding::ZigZag::encode(val);
    int64_t dec_val = BareBones::Encoding::ZigZag::decode(enc_val);

    EXPECT_EQ(val, dec_val);
  }
}

TEST_F(ZigZag, EncodeDecodeInt32T) {
  int32_t val;

  std::mt19937 gen32(testing::UnitTest::GetInstance()->random_seed());

  for (size_t i = 0; i < NUM_VALUES; ++i) {
    if (i % 2 == 0) {
      val = 0 - gen32() / 2;
    } else {
      val = gen32() / 2;
    }

    uint32_t enc_val = BareBones::Encoding::ZigZag::encode(val);
    int32_t dec_val = BareBones::Encoding::ZigZag::decode(enc_val);

    EXPECT_EQ(val, dec_val);
  }
}
}  // namespace
