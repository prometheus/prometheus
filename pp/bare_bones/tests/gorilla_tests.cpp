#include <cmath>

#include <gtest/gtest.h>

#include "bare_bones/gorilla.h"

namespace {

using BareBones::BitSequence;
using BareBones::BitSequenceReader;
using BareBones::Encoding::Gorilla::StreamDecoder;
using BareBones::Encoding::Gorilla::StreamEncoder;
using BareBones::Encoding::Gorilla::TimestampDecoder;
using BareBones::Encoding::Gorilla::TimestampEncoder;
using BareBones::Encoding::Gorilla::ValuesDecoder;
using BareBones::Encoding::Gorilla::ValuesEncoder;
using BareBones::Encoding::Gorilla::ValuesNullDecoder;
using BareBones::Encoding::Gorilla::ZigZagTimestampDecoder;
using BareBones::Encoding::Gorilla::ZigZagTimestampEncoder;
using BareBones::Encoding::Gorilla::ZigZagTimestampNullDecoder;

const size_t NUM_SAMPLES = 1000;
const uint64_t START_TS = 1660828400000;
const uint64_t STEP_TS = 10000;
const double START_VALUE = 1000;
const double STEP_VALUE = 10.5;

using samples_sequence_type = std::array<std::pair<uint64_t, double>, NUM_SAMPLES>;

samples_sequence_type generate_consistent_samples() {
  samples_sequence_type samples;

  for (size_t i = 0; i < NUM_SAMPLES; ++i) {
    samples[i] = {
        START_TS + (STEP_TS * i),
        START_VALUE + (STEP_VALUE * i),
    };
  }

  return samples;
};

samples_sequence_type generate_samples_with_negative_delta_values() {
  samples_sequence_type samples;

  for (size_t i = 0; i < NUM_SAMPLES; ++i) {
    if (i % 2 == 0) {
      samples[i] = {
          START_TS + (STEP_TS * i),
          START_VALUE - (STEP_VALUE * i),
      };
    } else {
      samples[i] = {
          START_TS + (STEP_TS * i),
          START_VALUE + (STEP_VALUE * i),
      };
    }
  }

  return samples;
};

samples_sequence_type generate_samples_with_negative_delta_timestamp() {
  samples_sequence_type samples;

  for (size_t i = 0; i < NUM_SAMPLES; ++i) {
    if (i % 2 == 0) {
      samples[i] = {
          START_TS - (STEP_TS * i),
          START_VALUE + (STEP_VALUE * i),
      };
    } else {
      samples[i] = {
          START_TS + (STEP_TS * i),
          START_VALUE + (STEP_VALUE * i),
      };
    }
  }

  return samples;
};

samples_sequence_type generate_samples_with_negative_delta() {
  samples_sequence_type samples;

  for (size_t i = 0; i < NUM_SAMPLES; ++i) {
    if (i % 2 == 0) {
      samples[i] = {
          START_TS - (STEP_TS * i),
          START_VALUE - (STEP_VALUE * i),
      };
    } else {
      samples[i] = {
          START_TS + (STEP_TS * i),
          START_VALUE + (STEP_VALUE * i),
      };
    }
  }

  return samples;
};

samples_sequence_type generate_samples_with_nan() {
  samples_sequence_type samples;

  for (size_t i = 0; i < NUM_SAMPLES; ++i) {
    if (i % 3 == 0) {
      samples[i] = {
          START_TS + (STEP_TS * i),
          std::nan("1"),
      };
    } else if (i % 2 == 0) {
      samples[i] = {
          START_TS + (STEP_TS * i),
          BareBones::Encoding::Gorilla::STALE_NAN,
      };
    } else {
      samples[i] = {
          START_TS + (STEP_TS * i),
          START_VALUE + (STEP_VALUE * i),
      };
    }
  }

  return samples;
};

struct Gorilla : public testing::TestWithParam<samples_sequence_type> {};

TEST_P(Gorilla, EncodeDecode) {
  StreamEncoder<ZigZagTimestampEncoder<>, ValuesEncoder> encoder;
  StreamDecoder<ZigZagTimestampDecoder<>, ValuesDecoder> decoder;
  BitSequence ts_bitseq;
  BitSequence v_bitseq;

  ASSERT_TRUE(ts_bitseq.empty());
  ASSERT_TRUE(v_bitseq.empty());

  const samples_sequence_type samples = GetParam();

  for (const auto& sample : samples) {
    encoder.encode(sample.first, sample.second, ts_bitseq, v_bitseq);
  }

  EXPECT_FALSE(ts_bitseq.empty());
  EXPECT_FALSE(v_bitseq.empty());

  auto ts_reader = ts_bitseq.reader();
  auto v_reader = v_bitseq.reader();

  for (const auto& sample : samples) {
    decoder.decode(ts_reader, v_reader);

    EXPECT_EQ(std::bit_cast<uint64_t>(decoder.last_timestamp()), sample.first);

    if (!std::isnan(sample.second)) {
      EXPECT_EQ(decoder.last_value(), sample.second);
    } else if (BareBones::Encoding::Gorilla::isstalenan(sample.second)) {
      EXPECT_TRUE(BareBones::Encoding::Gorilla::isstalenan(decoder.last_value()));
    } else {
      EXPECT_TRUE(std::isnan(decoder.last_value()));
    }
  }

  EXPECT_EQ(ts_reader.left(), 0U);
  EXPECT_EQ(v_reader.left(), 0U);
}

INSTANTIATE_TEST_SUITE_P(Consistent, Gorilla, testing::Values(generate_consistent_samples()));
INSTANTIATE_TEST_SUITE_P(NegativeDeltaValues, Gorilla, testing::Values(generate_samples_with_negative_delta_values()));
INSTANTIATE_TEST_SUITE_P(NegativeDeltaTimestamp, Gorilla, testing::Values(generate_samples_with_negative_delta_timestamp()));
INSTANTIATE_TEST_SUITE_P(NegativeDelta, Gorilla, testing::Values(generate_samples_with_negative_delta()));
INSTANTIATE_TEST_SUITE_P(IsNaN, Gorilla, testing::Values(generate_samples_with_nan()));

struct Sample {
  int64_t timestamp;
  double value;

  auto operator<=>(const Sample&) const noexcept = default;
};

struct StreamEncoderCase {
  std::vector<Sample> samples;
  std::vector<uint8_t> expected;
  size_t expected_size;
};

class StreamEncoderDecoderFixture : public testing::TestWithParam<StreamEncoderCase> {
 protected:
  BitSequence sequence_;
  StreamEncoder<TimestampEncoder, ValuesEncoder> encoder_;
  StreamDecoder<TimestampDecoder, ValuesDecoder> decoder_;
};

TEST_P(StreamEncoderDecoderFixture, Encode) {
  // Arrange

  // Act
  for (auto& sample : GetParam().samples) {
    encoder_.encode(sample.timestamp, sample.value, sequence_, sequence_);
  }

  // Assert
  EXPECT_TRUE(std::ranges::equal(GetParam().expected, sequence_.filled_bytes()));
  EXPECT_EQ(GetParam().expected_size, sequence_.size());
}

TEST_P(StreamEncoderDecoderFixture, EncodeDecode) {
  // Arrange
  std::vector<Sample> decoded_samples;

  // Act
  for (auto& sample : GetParam().samples) {
    encoder_.encode(sample.timestamp, sample.value, sequence_, sequence_);
  }

  auto reader = sequence_.reader();
  while (!reader.eof()) {
    decoder_.decode(reader, reader);
    decoded_samples.emplace_back(Sample{.timestamp = decoder_.last_timestamp(), .value = decoder_.last_value()});
  }

  // Assert
  EXPECT_EQ(GetParam().samples, decoded_samples);
}

INSTANTIATE_TEST_SUITE_P(
    OneSample,
    StreamEncoderDecoderFixture,
    testing::Values(
        StreamEncoderCase{.samples = {{.timestamp = 0, .value = 0}}, .expected = {0x00}, .expected_size = 10},
        StreamEncoderCase{.samples = {{.timestamp = std::numeric_limits<int64_t>::max(), .value = 0}},
                          .expected = {0xFE, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x01},
                          .expected_size = 82},
        StreamEncoderCase{.samples = {{.timestamp = -1000, .value = -1000}}, .expected = {0xCF, 0x0F, 0x02, 0x00, 0x00, 0x00, 0x3D, 0x02}, .expected_size = 66},
        StreamEncoderCase{.samples = {{.timestamp = 1000, .value = 1000}}, .expected = {0xD0, 0x0F, 0x02, 0x00, 0x00, 0x00, 0x3D, 0x02}, .expected_size = 66}));

INSTANTIATE_TEST_SUITE_P(TwoSamples,
                         StreamEncoderDecoderFixture,
                         testing::Values(StreamEncoderCase{.samples = {{.timestamp = 0, .value = 0}, {.timestamp = 1000, .value = 1000}},
                                                           .expected = {0x00, 0xA0, 0x1F, 0x1C, 0xA2, 0x1E, 0x81},
                                                           .expected_size = 56}));

INSTANTIATE_TEST_SUITE_P(
    ThreeSamples,
    StreamEncoderDecoderFixture,
    testing::Values(StreamEncoderCase{.samples = {{.timestamp = 0, .value = 0}, {.timestamp = 1000, .value = 1000}, {.timestamp = 2000, .value = 2000}},
                                      .expected = {0x00, 0xA0, 0x1F, 0x1C, 0xA2, 0x1E, 0x81, 0x5E},
                                      .expected_size = 71},
                    StreamEncoderCase{.samples = {{.timestamp = 0, .value = 0}, {.timestamp = 1000, .value = 1000}, {.timestamp = 2001, .value = 2001}},
                                      .expected = {0x00, 0xA0, 0x1F, 0x1C, 0xA2, 0x1E, 0x81, 0xC5, 0x6B, 0x09},
                                      .expected_size = 86},
                    StreamEncoderCase{.samples = {{.timestamp = 0, .value = 0}, {.timestamp = 1000, .value = 1000}, {.timestamp = 3000, .value = 3000}},
                                      .expected = {0x00, 0xA0, 0x1F, 0x1C, 0xA2, 0x1E, 0x81, 0x43, 0x1F, 0x56, 0xCA, 0xA0},
                                      .expected_size = 96},
                    StreamEncoderCase{.samples = {{.timestamp = 0, .value = 0}, {.timestamp = 1000, .value = 1000}, {.timestamp = 20000, .value = 20000}},
                                      .expected = {0x00, 0xA0, 0x1F, 0x1C, 0xA2, 0x1E, 0x81, 0x07, 0x65, 0xE4, 0xC4, 0x64},
                                      .expected_size = 102},
                    StreamEncoderCase{.samples = {{.timestamp = 0, .value = 0}, {.timestamp = 1000, .value = 1000}, {.timestamp = 1000, .value = 1000}},
                                      .expected = {0x00, 0xA0, 0x1F, 0x1C, 0xA2, 0x1E, 0x81, 0x8F, 0xC1, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
                                      .expected_size = 125}));

TEST(EncoderDecoder, ResetState) {
  using Encoder = StreamEncoder<TimestampEncoder, ValuesEncoder>;
  using Decoder = StreamDecoder<TimestampDecoder, ValuesDecoder>;

  // Arrange
  Encoder encoder;
  Decoder decoder;

  BitSequence ts_seq;
  BitSequence val_seq;

  const auto encode_and_decode = [&decoder, &ts_seq, &val_seq](Encoder& encoder, int64_t timestamp, double value) {
    encoder.encode(timestamp, value, ts_seq, val_seq);

    auto ts_reader = ts_seq.reader();
    auto val_reader = val_seq.reader();
    decoder.decode(ts_reader, val_reader);

    ts_seq.clear();
    val_seq.clear();
  };

  encode_and_decode(encoder, 0, std::bit_cast<double>(4524633156059818833ULL));
  encode_and_decode(encoder, 1, std::bit_cast<double>(4524633156059818833ULL));
  encode_and_decode(encoder, 2, std::bit_cast<double>(4524871635567203750ULL));
  encode_and_decode(encoder, 3, std::bit_cast<double>(4524871635567203750ULL));

  // Act
  Encoder encoder2{decoder.state()};
  encode_and_decode(encoder2, 4, std::bit_cast<double>(4524193975976911956ULL));

  // Assert
  EXPECT_EQ(4, decoder.last_timestamp());
  EXPECT_EQ(4524193975976911956ULL, std::bit_cast<uint64_t>(decoder.last_value()));
}

struct NullDecoderCase {
  std::vector<Sample> samples_for_skip;
  std::vector<Sample> samples;
};

class NullDecoderFixture : public ::testing::TestWithParam<NullDecoderCase> {
 protected:
  BitSequence sequence_;
  StreamDecoder<ZigZagTimestampNullDecoder<>, ValuesNullDecoder> null_decoder_;
  StreamDecoder<ZigZagTimestampDecoder<>, ValuesDecoder> decoder_;

  void encode_samples(const std::vector<Sample>& samples) {
    StreamEncoder<ZigZagTimestampEncoder<>, ValuesEncoder> encoder;
    for (auto& sample : samples) {
      encoder.encode(sample.timestamp, sample.value, sequence_, sequence_);
    }
  }

  void skip_samples(BitSequenceReader& reader) {
    for ([[maybe_unused]] auto& _ : GetParam().samples_for_skip) {
      null_decoder_.decode(reader, reader);
    }
  }

  std::vector<Sample> decode_samples(BitSequenceReader& reader) {
    std::vector<Sample> samples;

    while (!reader.eof()) {
      decoder_.decode(reader, reader);
      samples.emplace_back(Sample{.timestamp = decoder_.last_timestamp(), .value = decoder_.last_value()});
    }

    return samples;
  }
};

TEST_P(NullDecoderFixture, Test) {
  // Arrange
  encode_samples(GetParam().samples_for_skip);
  encode_samples(GetParam().samples);

  // Act
  auto reader = sequence_.reader();
  skip_samples(reader);
  const auto samples = decode_samples(reader);

  // Assert
  EXPECT_EQ(GetParam().samples, samples);
}

INSTANTIATE_TEST_SUITE_P(
    Cases,
    NullDecoderFixture,
    testing::Values(
        NullDecoderCase{.samples_for_skip = {{.timestamp = 0, .value = 0}, {.timestamp = 1000, .value = 1000}, {.timestamp = 2000, .value = 2000}},
                        .samples = {{.timestamp = 0, .value = 0}, {.timestamp = 1000, .value = 1000}, {.timestamp = 2000, .value = 2000}}},
        NullDecoderCase{.samples_for_skip = {{.timestamp = std::numeric_limits<int64_t>::max(), .value = 0}},
                        .samples = {{.timestamp = std::numeric_limits<int64_t>::max(), .value = 0}}},
        NullDecoderCase{.samples_for_skip = {{.timestamp = 0, .value = 0}, {.timestamp = 1000, .value = 1000}, {.timestamp = 2001, .value = 2001}},
                        .samples = {{.timestamp = 0, .value = 0}, {.timestamp = 1000, .value = 1000}, {.timestamp = 2001, .value = 2001}}},
        NullDecoderCase{.samples_for_skip = {{.timestamp = 0, .value = 0}, {.timestamp = 1000, .value = 1000}, {.timestamp = 3000, .value = 3000}},
                        .samples = {{.timestamp = 0, .value = 0}, {.timestamp = 1000, .value = 1000}, {.timestamp = 3000, .value = 3000}}},
        NullDecoderCase{.samples_for_skip = {{.timestamp = 0, .value = 0}, {.timestamp = 1000, .value = 1000}, {.timestamp = 20000, .value = 20000}},
                        .samples = {{.timestamp = 0, .value = 0}, {.timestamp = 1000, .value = 1000}, {.timestamp = 20000, .value = 20000}}},
        NullDecoderCase{.samples_for_skip = {{.timestamp = 0, .value = 0}, {.timestamp = 1000, .value = 1000}, {.timestamp = 200000, .value = 200000}},
                        .samples = {{.timestamp = 0, .value = 0}, {.timestamp = 1000, .value = 1000}, {.timestamp = 200000, .value = 200000}}},
        NullDecoderCase{.samples_for_skip = {{.timestamp = 0, .value = 0}, {.timestamp = 1000, .value = 1000}, {.timestamp = 1000, .value = 1000}},
                        .samples = {{.timestamp = 0, .value = 0}, {.timestamp = 1000, .value = 1000}, {.timestamp = 1000, .value = 1000}}}));

}  // namespace
