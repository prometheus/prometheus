#include <random>

#include <gtest/gtest.h>

#include "bare_bones/bit_sequence.h"

namespace {

const size_t NUM_VALUES = 1000;

std::vector<uint64_t> generate_uint64_vector() {
  std::vector<uint64_t> data;
  std::mt19937 gen32(testing::UnitTest::GetInstance()->random_seed());
  for (size_t i = 0; i < NUM_VALUES; ++i) {
    data.push_back(gen32());
  }

  return data;
}

std::vector<double> generate_double_vector() {
  std::vector<double> data;
  std::uniform_real_distribution<double> unif;
  std::mt19937 gen32(testing::UnitTest::GetInstance()->random_seed());
  for (size_t i = 0; i < NUM_VALUES; ++i) {
    data.push_back(unif(gen32));
  }

  return data;
}

struct BitSequence : public testing::Test {
  BareBones::BitSequence bitseq;
};

TEST_F(BitSequence, SingleBit) {
  EXPECT_TRUE(bitseq.empty());
  EXPECT_EQ(bitseq.size(), 0);

  bitseq.push_back_single_zero_bit();
  EXPECT_EQ(bitseq.size(), 1);

  bitseq.push_back_single_one_bit();
  EXPECT_EQ(bitseq.size(), 2);

  EXPECT_FALSE(bitseq.empty());

  auto bitseq_reader = bitseq.reader();
  uint32_t outcome = bitseq_reader.read_bits_u32(0, 1);
  EXPECT_EQ(outcome, 0);

  bitseq_reader.ff(1);
  outcome = bitseq_reader.read_bits_u32(0, 1);
  EXPECT_EQ(outcome, 1);
  bitseq_reader.ff(1);

  EXPECT_EQ(bitseq_reader.left(), 0);

  bitseq.clear();
  EXPECT_TRUE(bitseq.empty());
  EXPECT_EQ(bitseq.size(), 0);
}

TEST_F(BitSequence, U64Svbyte0248) {
  EXPECT_TRUE(bitseq.empty());
  EXPECT_EQ(bitseq.size(), 0);

  const auto etalons = generate_uint64_vector();

  for (const auto& ts : etalons) {
    bitseq.push_back_u64_svbyte_0248(ts);
  }

  auto bitseq_reader = bitseq.reader();

  for (const auto& etalon : etalons) {
    auto outcome = bitseq_reader.consume_u64_svbyte_0248();
    EXPECT_EQ(outcome, etalon);
  }

  EXPECT_EQ(bitseq_reader.left(), 0);

  bitseq.clear();
  EXPECT_TRUE(bitseq.empty());
  EXPECT_EQ(bitseq.size(), 0);
}

TEST_F(BitSequence, U64Svbyte2468) {
  EXPECT_TRUE(bitseq.empty());
  EXPECT_EQ(bitseq.size(), 0);

  const auto etalons = generate_uint64_vector();

  for (const auto& ts : etalons) {
    bitseq.push_back_u64_svbyte_2468(ts);
  }

  auto bitseq_reader = bitseq.reader();

  for (const auto& etalon : etalons) {
    auto outcome = bitseq_reader.consume_u64_svbyte_2468();
    EXPECT_EQ(outcome, etalon);
  }

  EXPECT_EQ(bitseq_reader.left(), 0);

  bitseq.clear();
  EXPECT_TRUE(bitseq.empty());
  EXPECT_EQ(bitseq.size(), 0);
}

TEST_F(BitSequence, D64Svbyte2468) {
  EXPECT_TRUE(bitseq.empty());
  EXPECT_EQ(bitseq.size(), 0);

  const auto etalons = generate_double_vector();

  for (const auto& v : etalons) {
    bitseq.push_back_d64_svbyte_0468(v);
  }

  auto bitseq_reader = bitseq.reader();

  for (const auto& etalon : etalons) {
    auto outcome = bitseq_reader.consume_d64_svbyte_0468();
    EXPECT_EQ(outcome, etalon);
  }

  EXPECT_EQ(bitseq_reader.left(), 0);

  bitseq.clear();
  EXPECT_TRUE(bitseq.empty());
  EXPECT_EQ(bitseq.size(), 0);
}
}  // namespace
