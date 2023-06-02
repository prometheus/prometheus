#include <cmath>

#include <gtest/gtest.h>

#include "bare_bones/gorilla.h"

namespace {

using samples_sequence_type = std::vector<std::pair<uint64_t, double>>;

const size_t NUM_SAMPLES = 100000;
const uint64_t START_TS = 1660828400000;
const uint64_t STEP_TS = 10000;
const double START_VALUE = 1000;
const double STEP_VALUE = 10.5;

samples_sequence_type generate_consistent_samples() {
  samples_sequence_type samples;

  for (size_t i = 0; i < NUM_SAMPLES; ++i) {
    samples.push_back({
        START_TS + (STEP_TS * i),
        START_VALUE + (STEP_VALUE * i),
    });
  }

  return samples;
};

samples_sequence_type generate_samples_with_negative_delta_values() {
  samples_sequence_type samples;

  for (size_t i = 0; i < NUM_SAMPLES; ++i) {
    if (i % 2 == 0) {
      samples.push_back({
          START_TS + (STEP_TS * i),
          START_VALUE - (STEP_VALUE * i),
      });
    } else {
      samples.push_back({
          START_TS + (STEP_TS * i),
          START_VALUE + (STEP_VALUE * i),
      });
    }
  }

  return samples;
};

samples_sequence_type generate_samples_with_negative_delta_timestamp() {
  samples_sequence_type samples;

  for (size_t i = 0; i < NUM_SAMPLES; ++i) {
    if (i % 2 == 0) {
      samples.push_back({
          START_TS - (STEP_TS * i),
          START_VALUE + (STEP_VALUE * i),
      });
    } else {
      samples.push_back({
          START_TS + (STEP_TS * i),
          START_VALUE + (STEP_VALUE * i),
      });
    }
  }

  return samples;
};

samples_sequence_type generate_samples_with_negative_delta() {
  samples_sequence_type samples;

  for (size_t i = 0; i < NUM_SAMPLES; ++i) {
    if (i % 2 == 0) {
      samples.push_back({
          START_TS - (STEP_TS * i),
          START_VALUE - (STEP_VALUE * i),
      });
    } else {
      samples.push_back({
          START_TS + (STEP_TS * i),
          START_VALUE + (STEP_VALUE * i),
      });
    }
  }

  return samples;
};

samples_sequence_type generate_samples_with_nan() {
  samples_sequence_type samples;

  for (size_t i = 0; i < NUM_SAMPLES; ++i) {
    if (i % 3 == 0) {
      samples.push_back({
          START_TS + (STEP_TS * i),
          std::nan("1"),
      });
    } else if (i % 2 == 0) {
      samples.push_back({
          START_TS + (STEP_TS * i),
          BareBones::Encoding::Gorilla::STALE_NAN,
      });
    } else {
      samples.push_back({
          START_TS + (STEP_TS * i),
          START_VALUE + (STEP_VALUE * i),
      });
    }
  }

  return samples;
};

struct Gorilla : public testing::TestWithParam<samples_sequence_type> {};

TEST_P(Gorilla, EncodeDecode) {
  BareBones::Encoding::Gorilla::StreamEncoder encoder;
  BareBones::Encoding::Gorilla::StreamDecoder decoder;
  BareBones::BitSequence ts_bitseq;
  BareBones::BitSequence v_bitseq;

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

    EXPECT_EQ(decoder.last_timestamp(), sample.first);

    if (!std::isnan(sample.second)) {
      EXPECT_EQ(decoder.last_value(), sample.second);
    } else if (BareBones::Encoding::Gorilla::isstalenan(sample.second)) {
      EXPECT_TRUE(BareBones::Encoding::Gorilla::isstalenan(decoder.last_value()));
    } else {
      EXPECT_TRUE(std::isnan(decoder.last_value()));
    }
  }

  EXPECT_EQ(ts_reader.left(), 0);
  EXPECT_EQ(v_reader.left(), 0);
}

INSTANTIATE_TEST_SUITE_P(Consistent, Gorilla, testing::Values(generate_consistent_samples()));
INSTANTIATE_TEST_SUITE_P(NegativeDeltaValues, Gorilla, testing::Values(generate_samples_with_negative_delta_values()));
INSTANTIATE_TEST_SUITE_P(NegativeDeltaTimestamp, Gorilla, testing::Values(generate_samples_with_negative_delta_timestamp()));
INSTANTIATE_TEST_SUITE_P(NegativeDelta, Gorilla, testing::Values(generate_samples_with_negative_delta()));
INSTANTIATE_TEST_SUITE_P(IsNaN, Gorilla, testing::Values(generate_samples_with_nan()));
}  // namespace
