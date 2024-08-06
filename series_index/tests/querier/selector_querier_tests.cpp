#include <gtest/gtest.h>

#include "primitives/primitives.h"
#include "primitives/snug_composites.h"
#include "printer.h"
#include "series_index/querier/selector_querier.h"
#include "series_index/queryable_encoding_bimap.h"
#include "series_index/trie/cedarpp_tree.h"

namespace {

using PromPP::Primitives::LabelViewSet;
using PromPP::Prometheus::LabelMatchers;
using PromPP::Prometheus::MatcherType;
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
  LabelMatchers label_matchers{};
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
  Selector selector;

  // Act
  auto status = selector_querier_.query(GetParam().label_matchers, selector);

  // Assert
  EXPECT_EQ(GetParam().expected.status, status);
  EXPECT_EQ(GetParam().expected.selector, selector);
}

INSTANTIATE_TEST_SUITE_P(NoPositiveMatchers,
                         SelectorQuerierFixture,
                         testing::Values(SelectorQuerierTestCase{.expected = {.status = QuerierStatus::kNoPositiveMatchers}},
                                         SelectorQuerierTestCase{.label_matchers = {{.name = "job", .value = "cron", .type = MatcherType::kExactNotMatch}},
                                                                 .expected = {.status = QuerierStatus::kNoPositiveMatchers,
                                                                              .selector = {.matchers = {{.type = MatcherType::kExactNotMatch}}}}}));

INSTANTIATE_TEST_SUITE_P(
    LabelNameNotFound,
    SelectorQuerierFixture,
    testing::Values(SelectorQuerierTestCase{.label_matchers = {{.name = "job", .value = "cron", .type = MatcherType::kExactMatch}},
                                            .expected = {.status = QuerierStatus::kNoMatch,
                                                         .selector = {.matchers = {{.status = MatchStatus::kEmptyMatch, .type = MatcherType::kExactMatch}}}}},
                    SelectorQuerierTestCase{.label_matchers = {{.name = "job", .value = "c.*", .type = MatcherType::kRegexpMatch}},
                                            .expected = {.status = QuerierStatus::kNoMatch,
                                                         .selector = {.matchers = {{.status = MatchStatus::kEmptyMatch, .type = MatcherType::kRegexpMatch}}}}},
                    SelectorQuerierTestCase{
                        .label_matchers = {{.name = "job", .value = ".+", .type = MatcherType::kRegexpMatch}},
                        .expected = {.status = QuerierStatus::kNoMatch,
                                     .selector = {.matchers = {{.status = MatchStatus::kEmptyMatch, .type = MatcherType::kRegexpMatch}}}}}));

INSTANTIATE_TEST_SUITE_P(
    LabelNameNotFoundInNegativeMatcher,
    SelectorQuerierFixture,
    testing::Values(SelectorQuerierTestCase{
        .label_sets = {{{"job", "cron"}}},
        .label_matchers = {{.name = "job", .value = "cron", .type = MatcherType::kExactMatch},
                           {.name = "non_existing_label", .value = "value", .type = MatcherType::kExactNotMatch}},
        .expected = {.status = QuerierStatus::kMatch,
                     .selector = {.matchers = {{.matches = {0}, .label_name_id = 0, .status = MatchStatus::kPartialMatch, .type = MatcherType::kExactMatch},
                                               {.status = MatchStatus::kEmptyMatch, .type = MatcherType::kExactNotMatch}}}}}));

INSTANTIATE_TEST_SUITE_P(
    InvalidRegexp,
    SelectorQuerierFixture,
    testing::Values(
        SelectorQuerierTestCase{
            .label_sets = {{{"job", "cron"}}},
            .label_matchers = {{.name = "job", .value = "[", .type = MatcherType::kRegexpMatch}},
            .expected = {.status = QuerierStatus::kRegexpError,
                         .selector = {.matchers = {{.label_name_id = 0, .status = MatchStatus::kError, .type = MatcherType::kRegexpMatch}}}}},
        SelectorQuerierTestCase{
            .label_sets = {{{"job", "cron"}}},
            .label_matchers = {{.name = "job", .value = "cron", .type = MatcherType::kExactMatch},
                               {.name = "job", .value = "[", .type = MatcherType::kRegexpNotMatch}},
            .expected = {.status = QuerierStatus::kRegexpError,
                         .selector = {.matchers = {{.matches = {0}, .label_name_id = 0, .status = MatchStatus::kPartialMatch, .type = MatcherType::kExactMatch},
                                                   {.label_name_id = 0, .status = MatchStatus::kError, .type = MatcherType::kRegexpNotMatch}}}}}));

INSTANTIATE_TEST_SUITE_P(
    InvertEmptyPositiveMatcher,
    SelectorQuerierFixture,
    testing::Values(
        SelectorQuerierTestCase{
            .label_sets = {{{"job", "cron"}}},
            .label_matchers = {{.name = "job", .value = "", .type = MatcherType::kExactMatch}},
            .expected = {.status = QuerierStatus::kNoPositiveMatchers,
                         .selector = {.matchers = {{.label_name_id = 0, .status = MatchStatus::kAllMatch, .type = MatcherType::kExactNotMatch}}}}},
        SelectorQuerierTestCase{
            .label_sets = {{{"job", "cron"}}},
            .label_matchers = {{.name = "job", .value = "", .type = MatcherType::kRegexpMatch}},
            .expected = {.status = QuerierStatus::kNoPositiveMatchers,
                         .selector = {.matchers = {{.label_name_id = 0, .status = MatchStatus::kAllMatch, .type = MatcherType::kRegexpNotMatch}}}}},
        SelectorQuerierTestCase{
            .label_sets = {{{"job", "cron"}}},
            .label_matchers = {{.name = "job", .value = "cron", .type = MatcherType::kExactMatch},
                               {.name = "job", .value = "", .type = MatcherType::kExactNotMatch}},
            .expected = {.status = QuerierStatus::kMatch,
                         .selector = {.matchers = {{.matches{0}, .label_name_id = 0, .status = MatchStatus::kPartialMatch, .type = MatcherType::kExactMatch},
                                                   {.label_name_id = 0, .status = MatchStatus::kAllMatch, .type = MatcherType::kExactNotMatch}}}}},
        SelectorQuerierTestCase{
            .label_sets = {{{"job", "cron"}}},
            .label_matchers = {{.name = "job", .value = "^$", .type = MatcherType::kRegexpMatch}},
            .expected = {.status = QuerierStatus::kNoPositiveMatchers,
                         .selector = {.matchers = {{.label_name_id = 0, .status = MatchStatus::kAllMatch, .type = MatcherType::kRegexpNotMatch}}}}}));

INSTANTIATE_TEST_SUITE_P(
    ExactMatchers,
    SelectorQuerierFixture,
    testing::Values(
        SelectorQuerierTestCase{
            .label_sets = {{{"job", "cron"}}},
            .label_matchers = {{.name = "job", .value = "cron", .type = MatcherType::kExactMatch}},
            .expected =
                {.status = QuerierStatus::kMatch,
                 .selector = {.matchers = {{.matches = {0}, .label_name_id = 0, .status = MatchStatus::kPartialMatch, .type = MatcherType::kExactMatch}}}}},
        SelectorQuerierTestCase{
            .label_sets = {{{"job", "cron"}}},
            .label_matchers = {{.name = "job", .value = "cron1", .type = MatcherType::kExactMatch}},
            .expected = {.status = QuerierStatus::kNoMatch,
                         .selector = {.matchers = {{.label_name_id = 0, .status = MatchStatus::kEmptyMatch, .type = MatcherType::kExactMatch}}}}},
        SelectorQuerierTestCase{
            .label_sets = {{{"job", "cron"}}},
            .label_matchers = {{.name = "job", .value = "cron", .type = MatcherType::kExactMatch},
                               {.name = "job", .value = "cron", .type = MatcherType::kExactNotMatch}},
            .expected = {
                .status = QuerierStatus::kMatch,
                .selector = {.matchers = {{.matches = {0}, .label_name_id = 0, .status = MatchStatus::kPartialMatch, .type = MatcherType::kExactMatch},
                                          {.matches = {0}, .label_name_id = 0, .status = MatchStatus::kPartialMatch, .type = MatcherType::kExactNotMatch}}}}}));

INSTANTIATE_TEST_SUITE_P(
    RegexpMatchers,
    SelectorQuerierFixture,
    testing::Values(
        SelectorQuerierTestCase{
            .label_sets = {{{"job", "cron"}}},
            .label_matchers = {{.name = "job", .value = "cro.*", .type = MatcherType::kRegexpMatch}},
            .expected =
                {.status = QuerierStatus::kMatch,
                 .selector = {.matchers = {{.matches{0}, .label_name_id = 0, .status = MatchStatus::kPartialMatch, .type = MatcherType::kRegexpMatch}}}}},
        SelectorQuerierTestCase{
            .label_sets = {{{"job", "cron"}}},
            .label_matchers = {{.name = "job", .value = "cron1", .type = MatcherType::kRegexpMatch}},
            .expected = {.status = QuerierStatus::kNoMatch,
                         .selector = {.matchers = {{.matches{}, .label_name_id = 0, .status = MatchStatus::kEmptyMatch, .type = MatcherType::kRegexpMatch}}}}},
        SelectorQuerierTestCase{
            .label_sets = {{{"job", "cron"}}, {{"job", "crond"}}},
            .label_matchers = {{.name = "job", .value = "cro.*", .type = MatcherType::kRegexpMatch},
                               {.name = "job", .value = "cro.*", .type = MatcherType::kRegexpNotMatch}},
            .expected =
                {.status = QuerierStatus::kMatch,
                 .selector = {.matchers = {{.matches{0, 1}, .label_name_id = 0, .status = MatchStatus::kPartialMatch, .type = MatcherType::kRegexpMatch},
                                           {.matches{0, 1}, .label_name_id = 0, .status = MatchStatus::kPartialMatch, .type = MatcherType::kRegexpNotMatch}}}}},
        SelectorQuerierTestCase{
            .label_sets = {{{"job", "cron"}}},
            .label_matchers = {{.name = "job", .value = "cron", .type = MatcherType::kExactMatch},
                               {.name = "job", .value = "^$", .type = MatcherType::kRegexpNotMatch}},
            .expected = {.status = QuerierStatus::kMatch,
                         .selector = {.matchers = {{.matches = {0}, .label_name_id = 0, .status = MatchStatus::kPartialMatch, .type = MatcherType::kExactMatch},
                                                   {.label_name_id = 0, .status = MatchStatus::kAllMatch, .type = MatcherType::kRegexpNotMatch}}}}},
        SelectorQuerierTestCase{
            .label_sets = {{{"job", "cron"}}},
            .label_matchers = {{.name = "job", .value = "cron", .type = MatcherType::kExactMatch},
                               {.name = "job1", .value = "^$", .type = MatcherType::kRegexpNotMatch}},
            .expected = {.status = QuerierStatus::kNoMatch,
                         .selector = {.matchers = {{.matches{0}, .label_name_id = 0, .status = MatchStatus::kPartialMatch, .type = MatcherType::kExactMatch},
                                                   {.status = MatchStatus::kEmptyMatch, .type = MatcherType::kUnknown}}}}}));

INSTANTIATE_TEST_SUITE_P(
    AnythingMatchers,
    SelectorQuerierFixture,
    testing::Values(
        SelectorQuerierTestCase{
            .label_sets = {{{"job", "cron"}}},
            .label_matchers = {{.name = "job", .value = ".*", .type = MatcherType::kRegexpMatch}},
            .expected = {.status = QuerierStatus::kNoPositiveMatchers,
                         .selector = {.matchers = {{.label_name_id = 0, .status = MatchStatus::kAllMatch, .type = MatcherType::kUnknown}}}}},
        SelectorQuerierTestCase{.label_sets = {{{"job", "cron"}}},
                                .label_matchers = {{.name = "non_existing_label", .value = ".*", .type = MatcherType::kRegexpMatch}},
                                .expected = {.status = QuerierStatus::kNoPositiveMatchers,
                                             .selector = {.matchers = {{.status = MatchStatus::kAllMatch, .type = MatcherType::kUnknown}}}}},
        SelectorQuerierTestCase{
            .label_sets = {{{"job", "cron"}}},
            .label_matchers = {{.name = "job", .value = "cron", .type = MatcherType::kExactMatch},
                               {.name = "non_existing_label", .value = ".*", .type = MatcherType::kRegexpMatch}},
            .expected = {.status = QuerierStatus::kMatch,
                         .selector = {.matchers = {{.matches = {0}, .label_name_id = 0, .status = MatchStatus::kPartialMatch, .type = MatcherType::kExactMatch},
                                                   {.status = MatchStatus::kAllMatch, .type = MatcherType::kUnknown}}}}},
        SelectorQuerierTestCase{
            .label_sets = {{{"job", "cron"}}},
            .label_matchers = {{.name = "job", .value = "cron", .type = MatcherType::kExactMatch},
                               {.name = "non_existing_label", .value = ".*", .type = MatcherType::kRegexpNotMatch}},
            .expected = {.status = QuerierStatus::kNoMatch,
                         .selector = {.matchers = {{.matches = {0}, .label_name_id = 0, .status = MatchStatus::kPartialMatch, .type = MatcherType::kExactMatch},
                                                   {.status = MatchStatus::kAllMatch, .type = MatcherType::kUnknown}}}}}));

}  // namespace