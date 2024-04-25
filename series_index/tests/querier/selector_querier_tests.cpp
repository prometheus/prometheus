#include <gtest/gtest.h>

#include "primitives/primitives.h"
#include "primitives/snug_composites.h"
#include "printer.h"
#include "series_index/querier/selector_querier.h"
#include "series_index/queryable_encoding_bimap.h"
#include "series_index/trie/cedarpp_tree.h"

namespace {

using PromPP::Primitives::LabelViewSet;
using PromPP::Prometheus::LabelMatcher;
using PromPP::Prometheus::MatchStatus;
using PromPP::Prometheus::Selector;
using series_index::QueryableEncodingBimap;
using series_index::querier::QuerierStatus;
using series_index::querier::SelectorQuerier;
using series_index::trie::CedarMatchesList;
using series_index::trie::CedarTrie;

struct SelectorQuerierTestCase {
  struct Expected {
    QuerierStatus status;
    Selector selector{};
  };

  std::vector<LabelViewSet> label_sets{};
  Selector selector{};
  Expected expected;
};

class SelectorQuerierFixture : public testing::TestWithParam<SelectorQuerierTestCase> {
 protected:
  using TrieIndex = series_index::TrieIndex<CedarTrie, CedarMatchesList>;

  QueryableEncodingBimap<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimapFilament, TrieIndex> index_;
  SelectorQuerier<TrieIndex> selector_querier_{index_.trie_index()};

  void SetUp() override {
    for (auto& label_set : GetParam().label_sets) {
      index_.find_or_emplace(label_set);
    }
  }
};

TEST_P(SelectorQuerierFixture, Test) {
  // Arrange
  Selector selector = GetParam().selector;

  // Act
  auto status = selector_querier_.query(selector);

  // Assert
  EXPECT_EQ(GetParam().expected.status, status);
  EXPECT_EQ(GetParam().expected.selector, selector);
}

INSTANTIATE_TEST_SUITE_P(
    NoPositiveMatchers,
    SelectorQuerierFixture,
    testing::Values(SelectorQuerierTestCase{.expected = {.status = QuerierStatus::kNoPositiveMatchers}},
                    SelectorQuerierTestCase{
                        .selector = {.matchers = {{.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactNotMatch}}}},
                        .expected = {.status = QuerierStatus::kNoPositiveMatchers,
                                     .selector = {.matchers = {{.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactNotMatch}}}}}}));

INSTANTIATE_TEST_SUITE_P(
    LabelNameNotFound,
    SelectorQuerierFixture,
    testing::Values(
        SelectorQuerierTestCase{.selector = {.matchers = {{.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactMatch}}}},
                                .expected = {.status = QuerierStatus::kNoMatch,
                                             .selector = {.matchers = {{.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactMatch},
                                                                        .result = {.status = MatchStatus::kEmptyMatch}}}}}},
        SelectorQuerierTestCase{.selector = {.matchers = {{.matcher = {.name = "job", .value = "c.*", .type = LabelMatcher::Type::kRegexpMatch}}}},
                                .expected = {.status = QuerierStatus::kNoMatch,
                                             .selector = {.matchers = {{.matcher = {.name = "job", .value = "c.*", .type = LabelMatcher::Type::kRegexpMatch},
                                                                        .result = {.status = MatchStatus::kEmptyMatch}}}}}},
        SelectorQuerierTestCase{.selector = {.matchers = {{.matcher = {.name = "job", .value = ".+", .type = LabelMatcher::Type::kRegexpMatch}}}},
                                .expected = {.status = QuerierStatus::kNoMatch,
                                             .selector = {.matchers = {{.matcher = {.name = "job", .value = ".+", .type = LabelMatcher::Type::kRegexpMatch},
                                                                        .result = {.status = MatchStatus::kEmptyMatch}}}}}}));

INSTANTIATE_TEST_SUITE_P(
    LabelNameNotFoundInNegativeMatcher,
    SelectorQuerierFixture,
    testing::Values(SelectorQuerierTestCase{
        .label_sets = {{{"job", "cron"}}},
        .selector = {.matchers = {{.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactMatch}},
                                  {.matcher = {.name = "non_existing_label", .value = "value", .type = LabelMatcher::Type::kExactNotMatch}}}},
        .expected = {.status = QuerierStatus::kMatch,
                     .selector = {.matchers = {{.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactMatch},
                                                .result = {.matches = {0}, .label_name_id = 0, .status = MatchStatus::kPartialMatch}},
                                               {.matcher = {.name = "non_existing_label", .value = "value", .type = LabelMatcher::Type::kExactNotMatch},
                                                .result = {.status = MatchStatus::kEmptyMatch}}}}}}));

INSTANTIATE_TEST_SUITE_P(
    InvalidRegexp,
    SelectorQuerierFixture,
    testing::Values(
        SelectorQuerierTestCase{.label_sets = {{{"job", "cron"}}},
                                .selector = {.matchers = {{.matcher = {.name = "job", .value = "[", .type = LabelMatcher::Type::kRegexpMatch}}}},
                                .expected = {.status = QuerierStatus::kRegexpError,
                                             .selector = {.matchers = {{.matcher = {.name = "job", .value = "[", .type = LabelMatcher::Type::kRegexpMatch},
                                                                        .result = {.label_name_id = 0, .status = MatchStatus::kError}}}}}},
        SelectorQuerierTestCase{.label_sets = {{{"job", "cron"}}},
                                .selector = {.matchers = {{.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactMatch}},
                                                          {.matcher = {.name = "job", .value = "[", .type = LabelMatcher::Type::kRegexpNotMatch}}}},
                                .expected = {.status = QuerierStatus::kRegexpError,
                                             .selector = {.matchers = {{.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactMatch},
                                                                        .result = {.matches{0}, .label_name_id = 0, .status = MatchStatus::kPartialMatch}},
                                                                       {.matcher = {.name = "job", .value = "[", .type = LabelMatcher::Type::kRegexpNotMatch},
                                                                        .result = {.label_name_id = 0, .status = MatchStatus::kError}}}}}}));

INSTANTIATE_TEST_SUITE_P(
    InvertEmptyPositiveMatcher,
    SelectorQuerierFixture,
    testing::Values(
        SelectorQuerierTestCase{.label_sets = {{{"job", "cron"}}},
                                .selector = {.matchers = {{.matcher = {.name = "job", .value = "", .type = LabelMatcher::Type::kExactMatch}}}},
                                .expected = {.status = QuerierStatus::kNoPositiveMatchers,
                                             .selector = {.matchers = {{.matcher = {.name = "job", .value = "", .type = LabelMatcher::Type::kExactNotMatch},
                                                                        .result = {.label_name_id = 0, .status = MatchStatus::kAllMatch}}}}}},
        SelectorQuerierTestCase{.label_sets = {{{"job", "cron"}}},
                                .selector = {.matchers = {{.matcher = {.name = "job", .value = "", .type = LabelMatcher::Type::kRegexpMatch}}}},
                                .expected = {.status = QuerierStatus::kNoPositiveMatchers,
                                             .selector = {.matchers = {{.matcher = {.name = "job", .value = "", .type = LabelMatcher::Type::kRegexpNotMatch},
                                                                        .result = {.label_name_id = 0, .status = MatchStatus::kAllMatch}}}}}},
        SelectorQuerierTestCase{.label_sets = {{{"job", "cron"}}},
                                .selector = {.matchers = {{.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactMatch}},
                                                          {.matcher = {.name = "job", .value = "", .type = LabelMatcher::Type::kExactNotMatch}}}},
                                .expected = {.status = QuerierStatus::kMatch,
                                             .selector = {.matchers = {{.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactMatch},
                                                                        .result = {.matches{0}, .label_name_id = 0, .status = MatchStatus::kPartialMatch}},
                                                                       {.matcher = {.name = "job", .value = "", .type = LabelMatcher::Type::kExactNotMatch},
                                                                        .result = {.matches{}, .label_name_id = 0, .status = MatchStatus::kAllMatch}}}}}},
        SelectorQuerierTestCase{.label_sets = {{{"job", "cron"}}},
                                .selector = {.matchers = {{.matcher = {.name = "job", .value = "^$", .type = LabelMatcher::Type::kRegexpMatch}}}},
                                .expected = {.status = QuerierStatus::kNoPositiveMatchers,
                                             .selector = {.matchers = {{.matcher = {.name = "job", .value = "^$", .type = LabelMatcher::Type::kRegexpNotMatch},
                                                                        .result = {.label_name_id = 0, .status = MatchStatus::kAllMatch}}}}}}));

INSTANTIATE_TEST_SUITE_P(
    ExactMatchers,
    SelectorQuerierFixture,
    testing::Values(
        SelectorQuerierTestCase{.label_sets = {{{"job", "cron"}}},
                                .selector = {.matchers = {{.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactMatch}}}},
                                .expected = {.status = QuerierStatus::kMatch,
                                             .selector = {.matchers = {{.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactMatch},
                                                                        .result = {.matches{0}, .label_name_id = 0, .status = MatchStatus::kPartialMatch}}}}}},
        SelectorQuerierTestCase{.label_sets = {{{"job", "cron"}}},
                                .selector = {.matchers = {{.matcher = {.name = "job", .value = "cron1", .type = LabelMatcher::Type::kExactMatch}}}},
                                .expected = {.status = QuerierStatus::kNoMatch,
                                             .selector = {.matchers = {{.matcher = {.name = "job", .value = "cron1", .type = LabelMatcher::Type::kExactMatch},
                                                                        .result = {.matches{}, .label_name_id = 0, .status = MatchStatus::kEmptyMatch}}}}}},
        SelectorQuerierTestCase{
            .label_sets = {{{"job", "cron"}}},
            .selector = {.matchers = {{.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactMatch}},
                                      {.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactNotMatch}}}},
            .expected = {.status = QuerierStatus::kMatch,
                         .selector = {.matchers = {{.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactMatch},
                                                    .result = {.matches{0}, .label_name_id = 0, .status = MatchStatus::kPartialMatch}},
                                                   {.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactNotMatch},
                                                    .result = {.matches{0}, .label_name_id = 0, .status = MatchStatus::kPartialMatch}}}}}}));

INSTANTIATE_TEST_SUITE_P(
    RegexpMatchers,
    SelectorQuerierFixture,
    testing::Values(
        SelectorQuerierTestCase{.label_sets = {{{"job", "cron"}}},
                                .selector = {.matchers = {{.matcher = {.name = "job", .value = "cro.*", .type = LabelMatcher::Type::kRegexpMatch}}}},
                                .expected = {.status = QuerierStatus::kMatch,
                                             .selector = {.matchers = {{.matcher = {.name = "job", .value = "cro.*", .type = LabelMatcher::Type::kRegexpMatch},
                                                                        .result = {.matches{0}, .label_name_id = 0, .status = MatchStatus::kPartialMatch}}}}}},
        SelectorQuerierTestCase{.label_sets = {{{"job", "cron"}}},
                                .selector = {.matchers = {{.matcher = {.name = "job", .value = "cron1", .type = LabelMatcher::Type::kRegexpMatch}}}},
                                .expected = {.status = QuerierStatus::kNoMatch,
                                             .selector = {.matchers = {{.matcher = {.name = "job", .value = "cron1", .type = LabelMatcher::Type::kRegexpMatch},
                                                                        .result = {.matches{}, .label_name_id = 0, .status = MatchStatus::kEmptyMatch}}}}}},
        SelectorQuerierTestCase{
            .label_sets = {{{"job", "cron"}}, {{"job", "crond"}}},
            .selector = {.matchers = {{.matcher = {.name = "job", .value = "cro.*", .type = LabelMatcher::Type::kRegexpMatch}},
                                      {.matcher = {.name = "job", .value = "cro.*", .type = LabelMatcher::Type::kRegexpNotMatch}}}},
            .expected = {.status = QuerierStatus::kMatch,
                         .selector = {.matchers = {{.matcher = {.name = "job", .value = "cro.*", .type = LabelMatcher::Type::kRegexpMatch},
                                                    .result = {.matches{0, 1}, .label_name_id = 0, .status = MatchStatus::kPartialMatch}},
                                                   {.matcher = {.name = "job", .value = "cro.*", .type = LabelMatcher::Type::kRegexpNotMatch},
                                                    .result = {.matches{0, 1}, .label_name_id = 0, .status = MatchStatus::kPartialMatch}}}}}},
        SelectorQuerierTestCase{.label_sets = {{{"job", "cron"}}},
                                .selector = {.matchers = {{.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactMatch}},
                                                          {.matcher = {.name = "job", .value = "^$", .type = LabelMatcher::Type::kRegexpNotMatch}}}},
                                .expected = {.status = QuerierStatus::kMatch,
                                             .selector = {.matchers = {{.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactMatch},
                                                                        .result = {.matches{0}, .label_name_id = 0, .status = MatchStatus::kPartialMatch}},
                                                                       {.matcher = {.name = "job", .value = "^$", .type = LabelMatcher::Type::kRegexpNotMatch},
                                                                        .result = {.label_name_id = 0, .status = MatchStatus::kAllMatch}}}}}},
        SelectorQuerierTestCase{.label_sets = {{{"job", "cron"}}},
                                .selector = {.matchers = {{.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactMatch}},
                                                          {.matcher = {.name = "job1", .value = "^$", .type = LabelMatcher::Type::kRegexpNotMatch}}}},
                                .expected = {.status = QuerierStatus::kNoMatch,
                                             .selector = {.matchers = {{.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactMatch},
                                                                        .result = {.matches{0}, .label_name_id = 0, .status = MatchStatus::kPartialMatch}},
                                                                       {.matcher = {.name = "job1", .value = "^$", .type = LabelMatcher::Type::kUnknown},
                                                                        .result = {.status = MatchStatus::kEmptyMatch}}}}}}));

INSTANTIATE_TEST_SUITE_P(
    AnythingMatchers,
    SelectorQuerierFixture,
    testing::Values(
        SelectorQuerierTestCase{.label_sets = {{{"job", "cron"}}},
                                .selector = {.matchers = {{.matcher = {.name = "job", .value = ".*", .type = LabelMatcher::Type::kRegexpMatch}}}},
                                .expected = {.status = QuerierStatus::kNoPositiveMatchers,
                                             .selector = {.matchers = {{.matcher = {.name = "job", .value = ".*", .type = LabelMatcher::Type::kUnknown},
                                                                        .result = {.label_name_id = 0, .status = MatchStatus::kAllMatch}}}}}},
        SelectorQuerierTestCase{
            .label_sets = {{{"job", "cron"}}},
            .selector = {.matchers = {{.matcher = {.name = "non_existing_label", .value = ".*", .type = LabelMatcher::Type::kRegexpMatch}}}},
            .expected = {.status = QuerierStatus::kNoPositiveMatchers,
                         .selector = {.matchers = {{.matcher = {.name = "non_existing_label", .value = ".*", .type = LabelMatcher::Type::kUnknown},
                                                    .result = {.status = MatchStatus::kAllMatch}}}}}},
        SelectorQuerierTestCase{
            .label_sets = {{{"job", "cron"}}},
            .selector = {.matchers = {{.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactMatch}},
                                      {.matcher = {.name = "non_existing_label", .value = ".*", .type = LabelMatcher::Type::kRegexpMatch}}}},
            .expected = {.status = QuerierStatus::kMatch,
                         .selector = {.matchers = {{.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactMatch},
                                                    .result = {.matches{0}, .label_name_id = 0, .status = MatchStatus::kPartialMatch}},
                                                   {.matcher = {.name = "non_existing_label", .value = ".*", .type = LabelMatcher::Type::kUnknown},
                                                    .result = {.status = MatchStatus::kAllMatch}}}}}},
        SelectorQuerierTestCase{
            .label_sets = {{{"job", "cron"}}},
            .selector = {.matchers = {{.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactMatch}},
                                      {.matcher = {.name = "non_existing_label", .value = ".*", .type = LabelMatcher::Type::kRegexpNotMatch}}}},
            .expected = {.status = QuerierStatus::kNoMatch,
                         .selector = {.matchers = {{.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactMatch},
                                                    .result = {.matches{0}, .label_name_id = 0, .status = MatchStatus::kPartialMatch}},
                                                   {.matcher = {.name = "non_existing_label", .value = ".*", .type = LabelMatcher::Type::kUnknown},
                                                    .result = {.status = MatchStatus::kAllMatch}}}}}}));

}  // namespace