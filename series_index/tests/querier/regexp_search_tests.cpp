#include <gtest/gtest.h>

#include "series_index/querier/regexp_searcher.h"

namespace {

using PromPP::Prometheus::MatchStatus;
using series_index::querier::RegexpMatchAnalyzer;
using series_index::querier::RegexpParser;
using series_index::querier::RegexpSearcher;

using Status = series_index::querier::RegexpMatchAnalyzer::Status;

struct RegexpMatchAnalyzerTestCase {
  std::string_view regexp;
  Status status;
};

class RegexpMatchAnalyzerFixture : public testing::TestWithParam<RegexpMatchAnalyzerTestCase> {};

TEST_P(RegexpMatchAnalyzerFixture, Test) {
  // Arrange

  // Act
  auto status = RegexpMatchAnalyzer::analyze(RegexpParser::parse(GetParam().regexp).get());

  // Assert
  EXPECT_EQ(GetParam().status, status);
}

INSTANTIATE_TEST_SUITE_P(InvalidRegexp, RegexpMatchAnalyzerFixture, testing::Values(RegexpMatchAnalyzerTestCase{.regexp = "[", .status = Status::kError}));

INSTANTIATE_TEST_SUITE_P(EmptyMatch,
                         RegexpMatchAnalyzerFixture,
                         testing::Values(RegexpMatchAnalyzerTestCase{.regexp = "", .status = Status::kEmptyMatch},
                                         RegexpMatchAnalyzerTestCase{.regexp = "^$", .status = Status::kEmptyMatch}));

INSTANTIATE_TEST_SUITE_P(AnythingMatchRegexp,
                         RegexpMatchAnalyzerFixture,
                         testing::Values(RegexpMatchAnalyzerTestCase{.regexp = ".*", .status = Status::kAnythingMatch},
                                         RegexpMatchAnalyzerTestCase{.regexp = "^.*", .status = Status::kAnythingMatch},
                                         RegexpMatchAnalyzerTestCase{.regexp = "^.*$", .status = Status::kAnythingMatch}));

INSTANTIATE_TEST_SUITE_P(AllMatchRegexp,
                         RegexpMatchAnalyzerFixture,
                         testing::Values(RegexpMatchAnalyzerTestCase{.regexp = ".+", .status = Status::kAllMatch},
                                         RegexpMatchAnalyzerTestCase{.regexp = "^.+", .status = Status::kAllMatch},
                                         RegexpMatchAnalyzerTestCase{.regexp = "^.+$", .status = Status::kAllMatch}));

INSTANTIATE_TEST_SUITE_P(PartialMatchRegexp,
                         RegexpMatchAnalyzerFixture,
                         testing::Values(RegexpMatchAnalyzerTestCase{.regexp = "a", .status = Status::kPartialMatch},
                                         RegexpMatchAnalyzerTestCase{.regexp = "^a.+", .status = Status::kPartialMatch},
                                         RegexpMatchAnalyzerTestCase{.regexp = "^a.+$", .status = Status::kPartialMatch},
                                         RegexpMatchAnalyzerTestCase{.regexp = "abc$", .status = Status::kPartialMatch}));

}  // namespace