#include <random>

#include <gtest/gtest.h>

#include "bare_bones/bit_sequence.h"

namespace {

using BareBones::AllocationSize;
using BareBones::BitSequenceReader;
using BareBones::CompactBitSequence;

const size_t NUM_VALUES = 1000;

constexpr std::array kAllocationSizesTable = {AllocationSize{0U}, AllocationSize{32U}};

std::vector<uint64_t> generate_uint64_vector() {
  std::vector<uint64_t> data;
  std::mt19937 gen32(testing::UnitTest::GetInstance()->random_seed());
  data.reserve(NUM_VALUES);
  for (size_t i = 0; i < NUM_VALUES; ++i) {
    data.push_back(gen32());
  }

  return data;
}

std::vector<double> generate_double_vector() {
  std::vector<double> data;
  std::uniform_real_distribution<double> unif;
  std::mt19937 gen32(testing::UnitTest::GetInstance()->random_seed());
  data.reserve(NUM_VALUES);
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
  uint32_t outcome = bitseq_reader.read_bits_u32(1);
  EXPECT_EQ(outcome, 0);

  bitseq_reader.ff(1);
  outcome = bitseq_reader.read_bits_u32(1);
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

struct CopyWithSizeConstructorCase {
  uint8_t bits_count;
  uint32_t expected;
};

class BitSequenceCopyWithSizeConstructorFixture : public testing::TestWithParam<CopyWithSizeConstructorCase> {};

TEST_P(BitSequenceCopyWithSizeConstructorFixture, Test) {
  // Arrange
  BareBones::BitSequence stream;
  stream.push_back_u32(std::numeric_limits<uint32_t>::max());

  // Act
  BareBones::BitSequence stream2(stream, GetParam().bits_count);

  // Assert
  ASSERT_EQ(GetParam().bits_count, stream2.size());
  EXPECT_EQ(GetParam().expected, *reinterpret_cast<const uint32_t*>(stream2.bytes().data()));
}

INSTANTIATE_TEST_SUITE_P(Tests,
                         BitSequenceCopyWithSizeConstructorFixture,
                         testing::Values(CopyWithSizeConstructorCase{.bits_count = 0, .expected = 0b0},
                                         CopyWithSizeConstructorCase{.bits_count = 1, .expected = 0b1},
                                         CopyWithSizeConstructorCase{.bits_count = 2, .expected = 0b11},
                                         CopyWithSizeConstructorCase{.bits_count = 3, .expected = 0b111},
                                         CopyWithSizeConstructorCase{.bits_count = 4, .expected = 0b1111},
                                         CopyWithSizeConstructorCase{.bits_count = 5, .expected = 0b11111},
                                         CopyWithSizeConstructorCase{.bits_count = 6, .expected = 0b111111},
                                         CopyWithSizeConstructorCase{.bits_count = 7, .expected = 0b1111111},
                                         CopyWithSizeConstructorCase{.bits_count = 8, .expected = 0b11111111},
                                         CopyWithSizeConstructorCase{.bits_count = 9, .expected = 0b111111111},
                                         CopyWithSizeConstructorCase{.bits_count = 16, .expected = 0b1111111111111111},
                                         CopyWithSizeConstructorCase{.bits_count = 32, .expected = std::numeric_limits<uint32_t>::max()}));

TEST_F(BitSequenceCopyWithSizeConstructorFixture, SourceStreamIsEmpty) {
  // Arrange
  BareBones::BitSequence stream;

  // Act
  BareBones::BitSequence stream2(stream, 0);

  // Assert
  EXPECT_TRUE(stream2.empty());
  EXPECT_TRUE(stream2.bytes().empty());
}

class CompactBitSequenceFixture : public testing::Test {
 protected:
  CompactBitSequence<kAllocationSizesTable> stream_;
};

TEST_F(CompactBitSequenceFixture, MoveConstructor) {
  // Arrange
  stream_.push_back_single_one_bit();

  // Act
  auto stream2 = std::move(stream_);

  // Assert
  EXPECT_EQ(0U, stream_.size_in_bits());
  ASSERT_TRUE(stream_.bytes().empty());

  EXPECT_EQ(1U, stream2.size_in_bits());
  ASSERT_FALSE(stream2.bytes().empty());
  EXPECT_EQ(0b1U, stream2.bytes()[0]);
}

TEST_F(CompactBitSequenceFixture, MoveOperator) {
  // Arrange
  stream_.push_back_single_one_bit();

  // Act
  decltype(stream_) stream2;
  stream2.push_back_single_one_bit();
  stream2 = std::move(stream_);

  // Assert
  EXPECT_EQ(0U, stream_.size_in_bits());
  ASSERT_TRUE(stream_.bytes().empty());

  EXPECT_EQ(1U, stream2.size_in_bits());
  ASSERT_FALSE(stream2.bytes().empty());
  EXPECT_EQ(0b1U, stream2.bytes()[0]);
}

TEST_F(CompactBitSequenceFixture, PushOnebit) {
  // Arrange

  // Act
  stream_.push_back_single_zero_bit();
  stream_.push_back_single_one_bit();
  stream_.push_back_single_zero_bit();
  stream_.push_back_single_one_bit();
  stream_.push_back_single_zero_bit();
  stream_.push_back_single_one_bit();
  stream_.push_back_single_zero_bit();
  stream_.push_back_single_one_bit();

  // Assert
  EXPECT_EQ(8U, stream_.size_in_bits());
  EXPECT_EQ(0b10101010, stream_.filled_bytes()[0]);
}

TEST_F(CompactBitSequenceFixture, PushUint32) {
  // Arrange

  // Act
  stream_.push_back_single_zero_bit();
  stream_.push_back_bits_u32(32, 0b10101010101010101010101010101010);
  auto bytes = stream_.bytes<uint32_t>().data();

  // Assert
  ASSERT_EQ(33U, stream_.size_in_bits());
  EXPECT_EQ(0b01010101010101010101010101010100ULL, bytes[0]);
  EXPECT_EQ(0b1UL, bytes[1]);
}

TEST_F(CompactBitSequenceFixture, PushUint64) {
  // Arrange

  // Act
  stream_.push_back_single_zero_bit();
  stream_.push_back_u64(0b1010101010101010101010101010101010101010101010101010101010101010);
  auto bytes = stream_.bytes<uint64_t>().data();

  // Assert
  ASSERT_EQ(65U, stream_.size_in_bits());
  EXPECT_EQ(0b0101010101010101010101010101010101010101010101010101010101010100ULL, bytes[0]);
  EXPECT_EQ(0b1ULL, bytes[1]);
}

TEST_F(CompactBitSequenceFixture, PushUint64_2) {
  // Arrange

  // Act
  stream_.push_back_u64(0b1010101010101010101010101010101010101010101010101010101010101010);
  auto bytes = stream_.bytes<uint64_t>().data();

  // Assert
  ASSERT_EQ(64U, stream_.size_in_bits());
  EXPECT_EQ(0b1010101010101010101010101010101010101010101010101010101010101010, bytes[0]);
}

template <class T>
class BitSequenceReaderFixture : public testing::Test {};

typedef testing::Types<BareBones::BitSequence, CompactBitSequence<kAllocationSizesTable>> BitSequenceTypes;
TYPED_TEST_SUITE(BitSequenceReaderFixture, BitSequenceTypes);

TYPED_TEST(BitSequenceReaderFixture, read_u64) {
  // Arrange
  constexpr auto value = 0b0101010101010101010101010101010101010101010101010101010101010101U;
  TypeParam stream;
  stream.push_back_u64(value);

  // Act
  auto reader = stream.reader();

  // Assert
  EXPECT_EQ(value, reader.read_u64());
}

TYPED_TEST(BitSequenceReaderFixture, read_u64_2) {
  // Arrange
  constexpr auto value = 0b0101010101010101010101010101010101010101010101010101010101010101U;
  TypeParam stream;
  stream.push_back_single_zero_bit();
  stream.push_back_u64(value);

  // Act
  auto reader = stream.reader();
  reader.ff(1);

  // Assert
  EXPECT_EQ(value, reader.read_u64());
}

TYPED_TEST(BitSequenceReaderFixture, read_bits_u64) {
  // Arrange
  constexpr auto value = 0b0101010101010101010101010101010101010101010101010101010101010101U;
  TypeParam stream;
  stream.push_back_u64(value);

  // Act
  auto reader = stream.reader();

  // Assert
  EXPECT_EQ(value, reader.read_bits_u64(BareBones::Bit::to_bits(sizeof(uint64_t))));
}

TYPED_TEST(BitSequenceReaderFixture, read_bits_u64_2) {
  // Arrange
  constexpr auto value = 0b0101010101010101010101010101010101010101010101010101010101010101U;
  TypeParam stream;
  stream.push_back_single_zero_bit();
  stream.push_back_u64(value);

  // Act
  auto reader = stream.reader();
  reader.ff(1);

  // Assert
  EXPECT_EQ(value, reader.read_bits_u64(BareBones::Bit::to_bits(sizeof(uint64_t))));
}

}  // namespace
