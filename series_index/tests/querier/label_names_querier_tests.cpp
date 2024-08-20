#include <gtest/gtest.h>

#include "primitives/primitives.h"
#include "primitives/snug_composites.h"
#include "series_index/querier/label_names_querier.h"
#include "series_index/queryable_encoding_bimap.h"
#include "series_index/trie/cedarpp_tree.h"

namespace {

using PromPP::Primitives::LabelViewSet;
using PromPP::Prometheus::LabelMatchers;
using PromPP::Prometheus::MatcherType;
using PromPP::Prometheus::Selector;
using series_index::QueryableEncodingBimap;
using series_index::querier::LabelNamesQuerier;
using series_index::querier::QuerierStatus;
using series_index::trie::CedarMatchesList;
using series_index::trie::CedarTrie;
using TrieIndex = series_index::TrieIndex<CedarTrie, CedarMatchesList>;
using Index = QueryableEncodingBimap<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimapFilament, TrieIndex>;

struct LabelNamesQuerierCase {
  LabelMatchers label_matchers{};
  QuerierStatus expected_status;
  std::vector<std::string_view> expected_names;
};

class LabelNamesQuerierFixture : public testing::TestWithParam<LabelNamesQuerierCase> {
 protected:
  Index index_;
  LabelNamesQuerier<Index> querier_{index_};

  void SetUp() final {
    for (auto& label_set : label_sets_) {
      index_.find_or_emplace(label_set);
    }
  }

 private:
  const std::vector<LabelViewSet> label_sets_{
      {{"job", "cron"}, {"task", "php"}},
      {{"job", "cron"}, {"script", "index.php"}},
      {{"pod", "pricing-ggd6p"}, {"goversion", "go1.21.4"}},
      {{"job", "cron"}, {"task", "php"}, {"script", "about.php"}, {"invalid_label", ""}},
  };
};

TEST_P(LabelNamesQuerierFixture, TestQuery) {
  // Arrange
  std::vector<std::string_view> names;

  // Act
  auto status = querier_.query(GetParam().label_matchers, [&names](std::string_view name) PROMPP_LAMBDA_INLINE { names.emplace_back(name); });

  // Assert
  EXPECT_EQ(GetParam().expected_status, status);
  EXPECT_EQ(GetParam().expected_names, names);
}

INSTANTIATE_TEST_SUITE_P(InvalidRegexpr,
                         LabelNamesQuerierFixture,
                         testing::Values(LabelNamesQuerierCase{.label_matchers = {{.name = "job", .value = "cr[on", .type = MatcherType::kRegexpMatch}},
                                                               .expected_status = QuerierStatus::kRegexpError,
                                                               .expected_names = {}}));

INSTANTIATE_TEST_SUITE_P(QueryAllNames,
                         LabelNamesQuerierFixture,
                         testing::Values(LabelNamesQuerierCase{.expected_status = QuerierStatus::kMatch,
                                                               .expected_names = {"goversion", "job", "pod", "script", "task"}}));

INSTANTIATE_TEST_SUITE_P(QueryBySelector,
                         LabelNamesQuerierFixture,
                         testing::Values(LabelNamesQuerierCase{.label_matchers = {{.name = "job", .value = "cron", .type = MatcherType::kExactMatch}},
                                                               .expected_status = QuerierStatus::kMatch,
                                                               .expected_names = {"job", "script", "task"}}));

INSTANTIATE_TEST_SUITE_P(NoMatches,
                         LabelNamesQuerierFixture,
                         testing::Values(LabelNamesQuerierCase{
                             .label_matchers = {{.name = "not_existing_label", .value = "cron", .type = MatcherType::kExactMatch}},
                             .expected_status = QuerierStatus::kNoMatch,
                             .expected_names = {}}));

}  // namespace