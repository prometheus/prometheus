#include <gtest/gtest.h>

#include "series_data/querier/query.h"

namespace {

using Query = series_data::querier::Query<BareBones::Vector<PromPP::Primitives::LabelSetID>>;
using std::string_view_literals::operator""sv;

class QueryReadFromProtobufErrorFixture : public testing::TestWithParam<std::string_view> {};

TEST_P(QueryReadFromProtobufErrorFixture, Test) {
  // Arrange
  Query query;

  // Act
  const auto result = Query::read_from_protobuf(GetParam(), query);

  // Assert
  EXPECT_FALSE(result);
}

INSTANTIATE_TEST_SUITE_P(EmptyBuffer, QueryReadFromProtobufErrorFixture, testing::Values(""sv));
INSTANTIATE_TEST_SUITE_P(InvalidBuffer, QueryReadFromProtobufErrorFixture, testing::Values("\xFF"sv));
INSTANTIATE_TEST_SUITE_P(EmptyLabelSetIds, QueryReadFromProtobufErrorFixture, testing::Values("\x08\x01\x10\x02"sv));
INSTANTIATE_TEST_SUITE_P(EndTimestampLessThanStartTimestamp, QueryReadFromProtobufErrorFixture, testing::Values("\x08\x02\x10\x01\x1a\x01\x01"sv));

struct ReadFromProtobufCase {
  std::string_view protobuf;
  Query expected;
};

class QueryReadFromProtobufFixture : public testing::TestWithParam<ReadFromProtobufCase> {};

TEST_P(QueryReadFromProtobufFixture, Test) {
  // Arrange
  Query query;

  // Act
  const auto result = Query::read_from_protobuf(GetParam().protobuf, query);

  // Assert
  ASSERT_TRUE(result);
  EXPECT_EQ(GetParam().expected, query);
}

INSTANTIATE_TEST_SUITE_P(EndTimestampEqualsStartTimestamp,
                         QueryReadFromProtobufFixture,
                         testing::Values(ReadFromProtobufCase{.protobuf = "\x08\x01\x10\x01\x1a\x01\x01"sv,
                                                              .expected = {.time_interval{.min = 1, .max = 1}, .label_set_ids = {1}}}));
INSTANTIATE_TEST_SUITE_P(BufferWithUnknownField,
                         QueryReadFromProtobufFixture,
                         testing::Values(ReadFromProtobufCase{.protobuf = "\x08\x01\x10\x02\x1a\x02\x01\x02\xa0\x06\x7b",
                                                              .expected = {.time_interval{.min = 1, .max = 2}, .label_set_ids = {1, 2}}}));

}  // namespace