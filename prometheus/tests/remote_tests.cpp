#include <gtest/gtest.h>

#include "prometheus/remote.h"

namespace {

using PromPP::Prometheus::LabelValuesQuery;
using PromPP::Prometheus::Query;
using PromPP::Prometheus::Selector;
using PromPP::Prometheus::Remote::ProtobufReader;
using PromPP::Prometheus::Remote::ProtobufWriter;
using std::operator""sv;

using MatcherType = PromPP::Prometheus::LabelMatcher::Type;

struct ReadQueryFromProtobufCase {
  std::string_view buffer{};
  bool expected;
  Query query{};
};

class ReadQueryFromProtobufFixture : public testing::TestWithParam<ReadQueryFromProtobufCase> {};

TEST_P(ReadQueryFromProtobufFixture, Test) {
  // Arrange
  Query query;

  // Act
  auto result = ProtobufReader::read_query(protozero::pbf_reader(GetParam().buffer.data(), GetParam().buffer.size()), query);

  // Assert
  ASSERT_EQ(GetParam().expected, result);
  EXPECT_EQ(GetParam().query, query);
}

INSTANTIATE_TEST_SUITE_P(EmptyBuffer, ReadQueryFromProtobufFixture, testing::Values(ReadQueryFromProtobufCase{.expected = true}));
INSTANTIATE_TEST_SUITE_P(
    OneMatcher,
    ReadQueryFromProtobufFixture,
    testing::Values(ReadQueryFromProtobufCase{
        .buffer =
            "\x08\x01\x10\x02\x1a\x0f\x08\x00\x12\x04\x6e\x61\x6d\x65\x1a\x05\x76\x61\x6c\x75\x65\x22\x0e\x08\x00\x12\x00\x18\x00\x20\x00\x2a\x00\x30\x00\x38\x00"sv,
        .expected = true,
        .query = {.selector = {.matchers = {{.matcher = {.name = "name", .value = "value", .type = MatcherType::kExactMatch}}}},
                  .start_timestamp_ms = 1,
                  .end_timestamp_ms = 2}}));
INSTANTIATE_TEST_SUITE_P(InvalidBuffer, ReadQueryFromProtobufFixture, testing::Values(ReadQueryFromProtobufCase{.buffer = "\x08"sv, .expected = false}));
INSTANTIATE_TEST_SUITE_P(
    MatcherValueIsEmpty,
    ReadQueryFromProtobufFixture,
    testing::Values(ReadQueryFromProtobufCase{
        .buffer = "\x08\x01\x10\x02\x1a\x0a\x08\x00\x12\x04\x6e\x61\x6d\x65\x1a\x00\x22\x0e\x08\x00\x12\x00\x18\x00\x20\x00\x2a\x00\x30\x00\x38\x00"sv,
        .expected = false,
        .query = {
            .selector = {.matchers = {{.matcher = {.name = "name", .value = "", .type = MatcherType::kExactMatch}}}},
            .start_timestamp_ms = 1,
            .end_timestamp_ms = 2,
        }}));
INSTANTIATE_TEST_SUITE_P(
    TwoMatchers,
    ReadQueryFromProtobufFixture,
    testing::Values(ReadQueryFromProtobufCase{
        .buffer =
            "\x08\x01\x10\x02\x1A\x0F\x08\x00\x12\x04\x6E\x61\x6D\x65\x1A\x05\x76\x61\x6C\x75\x65\x1A\x11\x08\x03\x12\x05\x6E\x61\x6D\x65\x32\x1A\x06\x76\x61\x6C\x75\x65\x32"sv,
        .expected = true,
        .query = {.selector = {.matchers = {{.matcher = {.name = "name", .value = "value", .type = MatcherType::kExactMatch}},
                                            {.matcher = {.name = "name2", .value = "value2", .type = MatcherType::kRegexpNotMatch}}}},
                  .start_timestamp_ms = 1,
                  .end_timestamp_ms = 2}}));

struct WriteQueryToProtobufCase {
  Query query{};
  std::string_view expected{};
};

class WriteQueryToProtobufFixture : public testing::TestWithParam<WriteQueryToProtobufCase> {};

TEST_P(WriteQueryToProtobufFixture, Test) {
  // Arrange

  // Act
  auto protobuf = ProtobufWriter::write_query(GetParam().query);

  // Assert
  EXPECT_EQ(GetParam().expected, protobuf);
}

INSTANTIATE_TEST_SUITE_P(
    TwoMatchers,
    WriteQueryToProtobufFixture,
    testing::Values(WriteQueryToProtobufCase{
        .query = {.selector = {.matchers = {{.matcher = {.name = "name", .value = "value", .type = MatcherType::kExactMatch}},
                                            {.matcher = {.name = "name2", .value = "value2", .type = MatcherType::kRegexpNotMatch}}}},
                  .start_timestamp_ms = 1,
                  .end_timestamp_ms = 2},
        .expected =
            "\x08\x01\x10\x02\x1A\x0F\x08\x00\x12\x04\x6E\x61\x6D\x65\x1A\x05\x76\x61\x6C\x75\x65\x1A\x11\x08\x03\x12\x05\x6E\x61\x6D\x65\x32\x1A\x06\x76\x61\x6C\x75\x65\x32"sv}));

struct ReadLabelValuesQueryFromProtobufCase {
  std::string_view buffer{};
  bool expected;
  LabelValuesQuery query{};
};

class ReadLabelValuesQueryFromProtobufFixture : public testing::TestWithParam<ReadLabelValuesQueryFromProtobufCase> {};

TEST_P(ReadLabelValuesQueryFromProtobufFixture, Test) {
  // Arrange
  LabelValuesQuery query;

  // Act
  auto result = ProtobufReader::read_label_values_query(protozero::pbf_reader(GetParam().buffer.data(), GetParam().buffer.size()), query);

  // Assert
  ASSERT_EQ(GetParam().expected, result);
  EXPECT_EQ(GetParam().query, query);
}

INSTANTIATE_TEST_SUITE_P(EmptyBuffer, ReadLabelValuesQueryFromProtobufFixture, testing::Values(ReadLabelValuesQueryFromProtobufCase{.expected = false}));
INSTANTIATE_TEST_SUITE_P(
    EmptyLabelName,
    ReadLabelValuesQueryFromProtobufFixture,
    testing::Values(ReadLabelValuesQueryFromProtobufCase{
        .buffer =
            "\x0a\x23\x08\x01\x10\x02\x1a\x0d\x08\x00\x12\x04\x63\x72\x6f\x6e\x1a\x03\x6a\x6f\x62\x22\x0e\x08\x00\x12\x00\x18\x00\x20\x00\x2a\x00\x30\x00\x38\x00\x12\x00"sv,
        .expected = false,
        .query = {.query = {.selector = {.matchers = {{.matcher = {.name = "cron", .value = "job", .type = MatcherType::kExactMatch}}}},
                            .start_timestamp_ms = 1,
                            .end_timestamp_ms = 2},
                  .label_name = {}}}));
INSTANTIATE_TEST_SUITE_P(
    ValidQuery,
    ReadLabelValuesQueryFromProtobufFixture,
    testing::Values(ReadLabelValuesQueryFromProtobufCase{
        .buffer =
            "\x0a\x23\x08\x01\x10\x02\x1a\x0d\x08\x00\x12\x04\x63\x72\x6f\x6e\x1a\x03\x6a\x6f\x62\x22\x0e\x08\x00\x12\x00\x18\x00\x20\x00\x2a\x00\x30\x00\x38\x00\x12\x04\x6e\x61\x6d\x65"sv,
        .expected = true,
        .query = {.query = {.selector = {.matchers = {{.matcher = {.name = "cron", .value = "job", .type = MatcherType::kExactMatch}}}},
                            .start_timestamp_ms = 1,
                            .end_timestamp_ms = 2},
                  .label_name = "name"}}));

struct WriteLabelValuesQueryToProtobufCase {
  LabelValuesQuery query{};
  std::string_view expected{};
};

class WriteLabelValuesQueryToProtobufFixture : public testing::TestWithParam<WriteLabelValuesQueryToProtobufCase> {};

TEST_P(WriteLabelValuesQueryToProtobufFixture, Test) {
  // Arrange

  // Act
  auto protobuf = ProtobufWriter::write_label_values_query(GetParam().query);

  // Assert
  EXPECT_EQ(GetParam().expected, protobuf);
}

INSTANTIATE_TEST_SUITE_P(Test,
                         WriteLabelValuesQueryToProtobufFixture,
                         testing::Values(WriteLabelValuesQueryToProtobufCase{
                             .query = {.query = {.selector = {.matchers = {{.matcher = {.name = "cron", .value = "job", .type = MatcherType::kExactMatch}}}},
                                                 .start_timestamp_ms = 1,
                                                 .end_timestamp_ms = 2},
                                       .label_name = "name"},
                             .expected = "\x0A\x13\x08\x01\x10\x02\x1A\x0D\x08\x00\x12\x04\x63\x72\x6F\x6E\x1A\x03\x6A\x6F\x62\x12\x04\x6E\x61\x6D\x65"sv}));

}  // namespace