#include <gtest/gtest.h>

#include "primitives/primitives.h"
#include "primitives/snug_composites.h"
#include "printer.h"
#include "series_index/querier/querier.h"
#include "series_index/trie/cedarpp_tree.h"

namespace {

using PromPP::Primitives::LabelViewSet;
using PromPP::Prometheus::LabelMatchers;
using PromPP::Prometheus::MatcherType;
using PromPP::Prometheus::Selector;
using series_index::QueryableEncodingBimap;
using series_index::querier::Querier;
using series_index::querier::QuerierStatus;
using series_index::trie::CedarMatchesList;
using series_index::trie::CedarTrie;
using TrieIndex = series_index::TrieIndex<CedarTrie, CedarMatchesList>;
using Index = QueryableEncodingBimap<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimapFilament, TrieIndex>;

struct MatchersComparatorByTypeAndCardinalityCase {
  Selector selector;
  Selector expected;
};

class MatchersComparatorByTypeAndCardinalityFixture : public testing::TestWithParam<MatchersComparatorByTypeAndCardinalityCase> {
 protected:
  Querier<Index>::MatchersComparatorByTypeAndCardinality comparator_;

  void sort(Selector& selector) { std::sort(selector.matchers.begin(), selector.matchers.end(), comparator_); }
};

TEST_P(MatchersComparatorByTypeAndCardinalityFixture, Test) {
  // Arrange
  Selector selector = GetParam().selector;

  // Act
  sort(selector);

  // Assert
  EXPECT_EQ(GetParam().expected, selector);
}

INSTANTIATE_TEST_SUITE_P(
    Cases,
    MatchersComparatorByTypeAndCardinalityFixture,
    testing::Values(
        MatchersComparatorByTypeAndCardinalityCase{
            .selector = {.matchers = {{.cardinality = 1, .type = MatcherType::kExactNotMatch}, {.cardinality = 0, .type = MatcherType::kExactNotMatch}}},
            .expected = {.matchers = {{.cardinality = 1, .type = MatcherType::kExactNotMatch}, {.cardinality = 0, .type = MatcherType::kExactNotMatch}}}},
        MatchersComparatorByTypeAndCardinalityCase{.selector = {.matchers = {{.type = MatcherType::kExactNotMatch}, {.type = MatcherType::kExactMatch}}},
                                                   .expected = {.matchers = {{.type = MatcherType::kExactMatch}, {.type = MatcherType::kExactNotMatch}}}},
        MatchersComparatorByTypeAndCardinalityCase{.selector = {.matchers = {{.type = MatcherType::kExactNotMatch},
                                                                             {.cardinality = 2, .type = MatcherType::kExactMatch},
                                                                             {.cardinality = 1, .type = MatcherType::kExactMatch}}},
                                                   .expected = {.matchers = {{.cardinality = 1, .type = MatcherType::kExactMatch},
                                                                             {.cardinality = 2, .type = MatcherType::kExactMatch},
                                                                             {.type = MatcherType::kExactNotMatch}}}}));

struct QuerierTestCase {
  struct Expected {
    std::vector<uint32_t> series_id_list{};
    QuerierStatus status{QuerierStatus::kNoMatch};
  };

  LabelMatchers label_matchers;
  Expected expected;
};

class QuerierFixture : public testing::TestWithParam<QuerierTestCase> {
 protected:
  Index index_;
  Querier<Index> querier_{index_};

  void SetUp() override {
    for (auto& label_set : label_sets_) {
      index_.find_or_emplace(label_set);
    }
  }

 private:
  std::vector<LabelViewSet> label_sets_{
      {{"job", "cron"}, {"task", "nodejs"}},
      {{"job", "cron"}, {"task", "php"}},
      {{"job", "cron"}, {"task", "python"}},
  };
};

TEST_P(QuerierFixture, Test) {
  // Arrange

  // Act
  auto result = querier_.query(GetParam().label_matchers);

  // Assert
  EXPECT_EQ(GetParam().expected.status, result.status);
  EXPECT_TRUE(std::ranges::equal(GetParam().expected.series_id_list, result.series_ids));
}

INSTANTIATE_TEST_SUITE_P(NoMatches,
                         QuerierFixture,
                         testing::Values(QuerierTestCase{.label_matchers = {{.name = "job", .value = "cro", .type = MatcherType::kExactMatch}},
                                                         .expected = {}}));

INSTANTIATE_TEST_SUITE_P(SingleMatcher,
                         QuerierFixture,
                         testing::Values(QuerierTestCase{.label_matchers = {{.name = "job", .value = "cron", .type = MatcherType::kExactMatch}},
                                                         .expected = {.series_id_list = {0, 1, 2}, .status = QuerierStatus::kMatch}},
                                         QuerierTestCase{.label_matchers = {{.name = "job", .value = ".+", .type = MatcherType::kRegexpMatch}},
                                                         .expected = {.series_id_list = {0, 1, 2}, .status = QuerierStatus::kMatch}}));

INSTANTIATE_TEST_SUITE_P(TwoPositiveMatchers,
                         QuerierFixture,
                         testing::Values(QuerierTestCase{.label_matchers = {{.name = "job", .value = "cron", .type = MatcherType::kExactMatch},
                                                                            {.name = "task", .value = "php", .type = MatcherType::kExactMatch}},
                                                         .expected = {.series_id_list = {1}, .status = QuerierStatus::kMatch}},
                                         QuerierTestCase{.label_matchers = {{.name = "task", .value = "php|python", .type = MatcherType::kRegexpMatch},
                                                                            {.name = "job", .value = "cron", .type = MatcherType::kExactMatch}},
                                                         .expected = {.series_id_list = {1, 2}, .status = QuerierStatus::kMatch}}));

INSTANTIATE_TEST_SUITE_P(EmptyMatcher,
                         QuerierFixture,
                         testing::Values(QuerierTestCase{.label_matchers = {{.name = "job", .value = "cron", .type = MatcherType::kExactMatch},
                                                                            {.name = "job1", .value = "", .type = MatcherType::kRegexpMatch}},
                                                         .expected = {.series_id_list = {0, 1, 2}, .status = QuerierStatus::kMatch}}));

INSTANTIATE_TEST_SUITE_P(NegativeMatcher,
                         QuerierFixture,
                         testing::Values(QuerierTestCase{.label_matchers = {{.name = "job", .value = "cron", .type = MatcherType::kExactMatch},
                                                                            {.name = "task", .value = "php", .type = MatcherType::kExactNotMatch}},
                                                         .expected = {.series_id_list = {0, 2}, .status = QuerierStatus::kMatch}},
                                         QuerierTestCase{.label_matchers = {{.name = "job", .value = "cron", .type = MatcherType::kExactMatch},
                                                                            {.name = "task", .value = "", .type = MatcherType::kExactMatch}},
                                                         .expected = {.series_id_list = {}, .status = QuerierStatus::kNoMatch}},
                                         QuerierTestCase{.label_matchers = {{.name = "job", .value = "cron", .type = MatcherType::kExactMatch},
                                                                            {.name = "task", .value = "unknown", .type = MatcherType::kExactNotMatch}},
                                                         .expected = {.series_id_list = {0, 1, 2}, .status = QuerierStatus::kMatch}},
                                         QuerierTestCase{.label_matchers = {{.name = "job", .value = "cron", .type = MatcherType::kExactMatch},
                                                                            {.name = "job1", .value = "^$", .type = MatcherType::kRegexpNotMatch}},
                                                         .expected = {.series_id_list = {}, .status = QuerierStatus::kNoMatch}},
                                         QuerierTestCase{.label_matchers = {{.name = "job", .value = "cron", .type = MatcherType::kExactMatch},
                                                                            {.name = "job1", .value = ".+", .type = MatcherType::kRegexpNotMatch}},
                                                         .expected = {.series_id_list = {0, 1, 2}, .status = QuerierStatus::kMatch}},
                                         QuerierTestCase{.label_matchers = {{.name = "job", .value = "cron", .type = MatcherType::kExactMatch},
                                                                            {.name = "job", .value = ".+", .type = MatcherType::kRegexpNotMatch}},
                                                         .expected = {.series_id_list = {}, .status = QuerierStatus::kNoMatch}}));

INSTANTIATE_TEST_SUITE_P(AnythingMatcher,
                         QuerierFixture,
                         testing::Values(QuerierTestCase{.label_matchers = {{.name = "job", .value = "cron", .type = MatcherType::kExactMatch},
                                                                            {.name = "job1", .value = ".*", .type = MatcherType::kRegexpMatch}},
                                                         .expected = {.series_id_list = {0, 1, 2}, .status = QuerierStatus::kMatch}},
                                         QuerierTestCase{.label_matchers = {{.name = "job", .value = "cron", .type = MatcherType::kExactMatch},
                                                                            {.name = "job1", .value = ".*", .type = MatcherType::kRegexpMatch},
                                                                            {.name = "task", .value = "php", .type = MatcherType::kExactNotMatch}},
                                                         .expected = {.series_id_list = {0, 2}, .status = QuerierStatus::kMatch}}));

}  // namespace