#include <gtest/gtest.h>

#include "primitives/primitives.h"
#include "primitives/snug_composites.h"
#include "series_index/querier/label_values_querier.h"
#include "series_index/queryable_encoding_bimap.h"
#include "series_index/trie/cedarpp_tree.h"

namespace {

using PromPP::Primitives::LabelViewSet;
using PromPP::Prometheus::LabelMatchers;
using PromPP::Prometheus::MatcherType;
using PromPP::Prometheus::Selector;
using series_index::QueryableEncodingBimap;
using series_index::querier::LabelValuesQuerier;
using series_index::querier::QuerierStatus;
using series_index::trie::CedarMatchesList;
using series_index::trie::CedarTrie;
using TrieIndex = series_index::TrieIndex<CedarTrie, CedarMatchesList>;
using Index = QueryableEncodingBimap<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimapFilament, TrieIndex>;

struct LabelValuesQuerierCase {
  std::string_view label_name;
  LabelMatchers label_matchers{};
  QuerierStatus expected_status;
  std::vector<std::string_view> expected_names;
};

class LabelValuesQuerierFixture : public testing::TestWithParam<LabelValuesQuerierCase> {
 protected:
  Index index_;
  LabelValuesQuerier<Index> querier_{index_};

  void SetUp() final {
    for (auto& label_set : label_sets_) {
      index_.find_or_emplace(label_set);
    }
  }

 private:
  const std::vector<LabelViewSet> label_sets_{
      {{"job", "cron"}, {"task", "php"}, {"script", "index.php"}, {"invalid_label", "custom value"}},
      {{"pod", "pricing-ggd6p"}, {"goversion", "go1.21.4"}},
      {{"job", "cron"}, {"task", "php"}, {"script", "about.php"}, {"invalid_label", ""}},
  };
};

TEST_P(LabelValuesQuerierFixture, TestQuery) {
  // Arrange
  std::vector<std::string_view> values;

  // Act
  auto status =
      querier_.query(GetParam().label_name, GetParam().label_matchers, [&values](std::string_view value) PROMPP_LAMBDA_INLINE { values.emplace_back(value); });

  // Assert
  EXPECT_EQ(GetParam().expected_status, status);
  EXPECT_EQ(GetParam().expected_names, values);
}

INSTANTIATE_TEST_SUITE_P(
    EmptyLabelName,
    LabelValuesQuerierFixture,
    testing::Values(LabelValuesQuerierCase{.label_name = "", .label_matchers = {}, .expected_status = QuerierStatus::kNoMatch, .expected_names = {}}));

INSTANTIATE_TEST_SUITE_P(LabelNameNotExists,
                         LabelValuesQuerierFixture,
                         testing::Values(LabelValuesQuerierCase{.label_name = "not_existing_label",
                                                                .label_matchers = {},
                                                                .expected_status = QuerierStatus::kNoMatch,
                                                                .expected_names = {}}));

INSTANTIATE_TEST_SUITE_P(InvalidRegexpr,
                         LabelValuesQuerierFixture,
                         testing::Values(LabelValuesQuerierCase{.label_name = "script",
                                                                .label_matchers = {{.name = "job", .value = "cr[on", .type = MatcherType::kRegexpMatch}},
                                                                .expected_status = QuerierStatus::kRegexpError,
                                                                .expected_names = {}}));

INSTANTIATE_TEST_SUITE_P(
    QueryAllValues,
    LabelValuesQuerierFixture,
    testing::Values(LabelValuesQuerierCase{.label_name = "script", .expected_status = QuerierStatus::kMatch, .expected_names = {"about.php", "index.php"}}));

INSTANTIATE_TEST_SUITE_P(QueryBySelector,
                         LabelValuesQuerierFixture,
                         testing::Values(LabelValuesQuerierCase{.label_name = "script",
                                                                .label_matchers = {{.name = "script", .value = ".*php", .type = MatcherType::kRegexpMatch}},
                                                                .expected_status = QuerierStatus::kMatch,
                                                                .expected_names = {"about.php", "index.php"}}));

INSTANTIATE_TEST_SUITE_P(NoMatches,
                         LabelValuesQuerierFixture,
                         testing::Values(LabelValuesQuerierCase{.label_name = "not_existing_label",
                                                                .label_matchers = {},
                                                                .expected_status = QuerierStatus::kNoMatch,
                                                                .expected_names = {}}));

INSTANTIATE_TEST_SUITE_P(LabelWithEmptyWalue,
                         LabelValuesQuerierFixture,
                         testing::Values(LabelValuesQuerierCase{.label_name = "invalid_label",
                                                                .label_matchers = {{.name = "job", .value = "cron", .type = MatcherType::kExactMatch}},
                                                                .expected_status = QuerierStatus::kMatch,
                                                                .expected_names = {"custom value"}}));

}  // namespace