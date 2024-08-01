#include <gtest/gtest.h>

#include "primitives/primitives.h"
#include "prometheus/tsdb/chunkenc/bstream.h"
#include "prometheus/tsdb/chunkenc/xor.h"

namespace {

using BareBones::AllocationSize;
using PromPP::Primitives::Sample;
using PromPP::Prometheus::tsdb::chunkenc::BStream;
using PromPP::Prometheus::tsdb::chunkenc::TimestampEncoder;
using PromPP::Prometheus::tsdb::chunkenc::ValuesEncoder;
using std::operator""sv;

constexpr std::array kAllocationSizesTable = {AllocationSize{0U}, AllocationSize{128U}};

struct TimestampEncoderCase {
  std::vector<int64_t> timestamps;
  std::vector<uint8_t> expected;
};

class PrometheusGorillaTimestampEncoderFixture : public testing::TestWithParam<TimestampEncoderCase> {
 protected:
  BStream<kAllocationSizesTable> stream_;
  BareBones::Encoding::Gorilla::TimestampEncoderState state_;

  void encode() {
    uint32_t count = 0;
    for (auto timestamp : GetParam().timestamps) {
      if (count == 0) {
        TimestampEncoder::encode(state_, timestamp, stream_);
      } else if (count == 1) {
        TimestampEncoder::encode_delta(state_, timestamp, stream_);
      } else {
        TimestampEncoder::encode_delta_of_delta(state_, timestamp, stream_);
      }

      ++count;
    }
  }
};

TEST_P(PrometheusGorillaTimestampEncoderFixture, EncodeTest) {
  // Arrange

  // Act
  encode();

  // Assert
  EXPECT_TRUE(std::ranges::equal(GetParam().expected, stream_.bytes()));
}

INSTANTIATE_TEST_SUITE_P(Encode,
                         PrometheusGorillaTimestampEncoderFixture,
                         testing::Values(TimestampEncoderCase{.timestamps = {1000}, .expected = {0xD0, 0x0F}},
                                         TimestampEncoderCase{.timestamps = {-1000}, .expected = {0xCF, 0x0F}}));

INSTANTIATE_TEST_SUITE_P(EncodeDelta,
                         PrometheusGorillaTimestampEncoderFixture,
                         testing::Values(TimestampEncoderCase{.timestamps = {1000, 1001}, .expected = {0xD0, 0x0F, 0x01}},
                                         TimestampEncoderCase{.timestamps = {1000, 999},
                                                              .expected = {0xD0, 0x0F, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x01}}));

INSTANTIATE_TEST_SUITE_P(EncodeDeltaOfDelta,
                         PrometheusGorillaTimestampEncoderFixture,
                         testing::Values(TimestampEncoderCase{.timestamps = {1000, 1001, 1002}, .expected = {0xD0, 0x0F, 0x01, 0x00}},
                                         TimestampEncoderCase{.timestamps = {1000, 1001, 1002, 1003}, .expected = {0xD0, 0x0F, 0x01, 0x00}},
                                         TimestampEncoderCase{.timestamps = {1000, 1001, 2001}, .expected = {0xD0, 0x0F, 0x01, 0x83, 0xE7}},
                                         TimestampEncoderCase{.timestamps = {1000, 1001, 20001}, .expected = {0xD0, 0x0F, 0x01, 0xC4, 0xA3, 0x70}},
                                         TimestampEncoderCase{.timestamps = {1000, 1001, 150001}, .expected = {0xD0, 0x0F, 0x01, 0xE2, 0x46, 0x07}},
                                         TimestampEncoderCase{.timestamps = {1000, 1001, 2000001},
                                                              .expected = {0xD0, 0x0F, 0x01, 0xF0, 0x00, 0x00, 0x00, 0x00, 0x01, 0xE8, 0x09, 0x70}},
                                         TimestampEncoderCase{.timestamps = {1000, 1001, -2000001},
                                                              .expected = {0xD0, 0x0F, 0x01, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFE, 0x17, 0x79, 0x50}},
                                         TimestampEncoderCase{.timestamps = {1000, 1001, 0}, .expected = {0xD0, 0x0F, 0x01, 0xBC, 0x16}},
                                         TimestampEncoderCase{.timestamps = {1000, 1001, -10001}, .expected = {0xD0, 0x0F, 0x01, 0xDD, 0x50, 0x50}},
                                         TimestampEncoderCase{.timestamps = {1000, 1001, -70001}, .expected = {0xD0, 0x0F, 0x01, 0xEE, 0xEA, 0xA5}}));

struct ValuesEncoderCase {
  std::vector<double> values;
  std::vector<uint8_t> expected;
};

class PrometheusGorillaValuesEncoderFixture : public testing::TestWithParam<ValuesEncoderCase> {
 protected:
  BStream<kAllocationSizesTable> stream_;
  BareBones::Encoding::Gorilla::ValuesEncoderState state_;

  void encode() {
    uint32_t count = 0;
    for (auto value : GetParam().values) {
      if (count == 0) {
        ValuesEncoder::encode_first(state_, value, stream_);
      } else {
        ValuesEncoder::encode(state_, value, stream_);
      }

      ++count;
    }
  }
};

TEST_P(PrometheusGorillaValuesEncoderFixture, EncodeTest) {
  // Arrange

  // Act
  encode();

  // Assert
  EXPECT_TRUE(std::ranges::equal(GetParam().expected, stream_.bytes()));
}

INSTANTIATE_TEST_SUITE_P(
    EncodeFirst,
    PrometheusGorillaValuesEncoderFixture,
    testing::Values(ValuesEncoderCase{.values = {1.0}, .expected = {0x3F, 0xF0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
                    ValuesEncoderCase{.values = {1.0, 2.0}, .expected = {0x3F, 0xF0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xC2, 0x5F, 0xFF}},
                    ValuesEncoderCase{.values = {1.0, 2.0, 2.0}, .expected = {0x3F, 0xF0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xC2, 0x5F, 0xFF, 0x00}},
                    ValuesEncoderCase{.values = {0.0, 10.0, 2.0},
                                      .expected = {0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xC2, 0x6C, 0x02, 0x60, 0x04, 0x80}}));

struct EncoderCase {
  BareBones::Vector<Sample> samples;
  std::string_view expected;
};

class PrometheusGorillaEncoderFixture : public testing::TestWithParam<EncoderCase> {
 protected:
  BStream<kAllocationSizesTable> stream_;
  BareBones::Encoding::Gorilla::StreamEncoder<TimestampEncoder, ValuesEncoder> encoder_;

  void encode() {
    stream_.write_bits(GetParam().samples.size(), BareBones::Bit::to_bits(sizeof(uint16_t)));

    for (auto& sample : GetParam().samples) {
      encoder_.encode(sample.timestamp(), sample.value(), stream_, stream_);
    }
  }
};

TEST_P(PrometheusGorillaEncoderFixture, Test) {
  // Arrange

  // Act
  encode();

  // Assert
  auto result = stream_.bytes();
  EXPECT_EQ(GetParam().expected, std::string_view(reinterpret_cast<const char*>(result.data()), result.size()));
}

INSTANTIATE_TEST_SUITE_P(
    WastedBitsOptimization,
    PrometheusGorillaEncoderFixture,
    testing::Values(EncoderCase{
        .samples =
            {
                {1000, 0.0},
                {1001, 100000.},
                {1002, 99999.999},
                {1003, 99998.998},
                {1004, 99997.997},
                {1005, 99996.996},
                {1006, 99995.995},
                {1007, 99994.994},
                {1008, 99993.993},
                {1009, 99992.992},
                {1010, 99989.989},
                {1011, 99987.987},
                {1012, 99985.985},
                {1013, 99984.984},
                {1014, 99983.983},
                {1015, 99982.982},
            },
        .expected = "\x00\x10\xD0\x0F\x00\x00\x00\x00\x00\x00\x00\x00\x01\xC2\xB4\x0F\x86\xAE\xD5\x7F\xFF\x7C\xED\x91\x68\x10\x0C\x29\xB5\x9C\x41\x80\x23\xC4"
                    "\xE5\xAA\x04\x07\x0A\xFD\xE3\xD0\xE0\x08\x31\x59\x2A\x81\x00\xCE\x99\x5F\xC4\x18\x02\x1F\xBE\x45\xA0\x40\xF1\xA2\x4E\x55\x1A\x01\xB9\x59"
                    "\xB4\xC8\x60\x18\x31\x2A\xEE\x41\x00\x42\x7E\xF9\xDA\x04\x1F\x3E\x65\x27\x5D\xE9\xF8\x02\x14\x7E\xC5\xA0\x80\x60\xDC\xA7\xA0"sv}));

}  // namespace