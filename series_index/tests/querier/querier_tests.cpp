#include <gtest/gtest.h>

#include "primitives/primitives.h"
#include "primitives/snug_composites.h"
#include "printer.h"
#include "series_index/querier/querier.h"
#include "series_index/trie/cedarpp_tree.h"

namespace {

using PromPP::Primitives::LabelViewSet;
using PromPP::Prometheus::LabelMatcher;
using PromPP::Prometheus::Selector;
using series_index::QueryableEncodingBimap;
using series_index::querier::Querier;
using series_index::querier::QuerierResult;
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
            .selector = {.matchers = {{.matcher = {.type = LabelMatcher::Type::kExactNotMatch}, .result = {.cardinality = 1}},
                                      {.matcher = {.type = LabelMatcher::Type::kExactNotMatch}, .result = {.cardinality = 0}}}},
            .expected = {.matchers = {{.matcher = {.type = LabelMatcher::Type::kExactNotMatch}, .result = {.cardinality = 1}},
                                      {.matcher = {.type = LabelMatcher::Type::kExactNotMatch}, .result = {.cardinality = 0}}}}},
        MatchersComparatorByTypeAndCardinalityCase{
            .selector = {.matchers = {{.matcher = {.type = LabelMatcher::Type::kExactNotMatch}}, {.matcher = {.type = LabelMatcher::Type::kExactMatch}}}},
            .expected = {.matchers = {{.matcher = {.type = LabelMatcher::Type::kExactMatch}}, {.matcher = {.type = LabelMatcher::Type::kExactNotMatch}}}}},
        MatchersComparatorByTypeAndCardinalityCase{
            .selector = {.matchers = {{.matcher = {.type = LabelMatcher::Type::kExactNotMatch}},
                                      {.matcher = {.type = LabelMatcher::Type::kExactMatch}, .result = {.cardinality = 2}},
                                      {.matcher = {.type = LabelMatcher::Type::kExactMatch}, .result = {.cardinality = 1}}}},
            .expected = {.matchers = {{.matcher = {.type = LabelMatcher::Type::kExactMatch}, .result = {.cardinality = 1}},
                                      {.matcher = {.type = LabelMatcher::Type::kExactMatch}, .result = {.cardinality = 2}},
                                      {.matcher = {.type = LabelMatcher::Type::kExactNotMatch}}}}}));

struct QuerierTestCase {
  struct Expected {
    std::vector<uint32_t> series_id_list{};
    QuerierStatus status{QuerierStatus::kNoMatch};
  };

  Selector selector{};
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
  Selector selector = GetParam().selector;

  // Act
  auto result = querier_.query(selector);

  // Assert
  EXPECT_EQ(GetParam().expected.status, result.status);
  EXPECT_TRUE(std::ranges::equal(GetParam().expected.series_id_list, result.series_ids));
}

INSTANTIATE_TEST_SUITE_P(NoMatches,
                         QuerierFixture,
                         testing::Values(QuerierTestCase{
                             .selector = {.matchers = {{.matcher = {.name = "job", .value = "cro", .type = LabelMatcher::Type::kExactMatch}}}},
                             .expected = {}}));

INSTANTIATE_TEST_SUITE_P(
    SingleMatcher,
    QuerierFixture,
    testing::Values(QuerierTestCase{.selector = {.matchers = {{.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactMatch}}}},
                                    .expected = {.series_id_list = {0, 1, 2}, .status = QuerierStatus::kMatch}},
                    QuerierTestCase{.selector = {.matchers = {{.matcher = {.name = "job", .value = ".+", .type = LabelMatcher::Type::kRegexpMatch}}}},
                                    .expected = {.series_id_list = {0, 1, 2}, .status = QuerierStatus::kMatch}}));

INSTANTIATE_TEST_SUITE_P(
    TwoPositiveMatchers,
    QuerierFixture,
    testing::Values(QuerierTestCase{.selector = {.matchers = {{.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactMatch}},
                                                              {.matcher = {.name = "task", .value = "php", .type = LabelMatcher::Type::kExactMatch}}}},
                                    .expected = {.series_id_list = {1}, .status = QuerierStatus::kMatch}},
                    QuerierTestCase{.selector = {.matchers = {{.matcher = {.name = "task", .value = "php|python", .type = LabelMatcher::Type::kRegexpMatch}},
                                                              {.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactMatch}}}},
                                    .expected = {.series_id_list = {1, 2}, .status = QuerierStatus::kMatch}}));

INSTANTIATE_TEST_SUITE_P(EmptyMatcher,
                         QuerierFixture,
                         testing::Values(QuerierTestCase{
                             .selector = {.matchers = {{.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactMatch}},
                                                       {.matcher = {.name = "job1", .value = "", .type = LabelMatcher::Type::kRegexpMatch}}}},
                             .expected = {.series_id_list = {0, 1, 2}, .status = QuerierStatus::kMatch}}));

INSTANTIATE_TEST_SUITE_P(
    NegativeMatcher,
    QuerierFixture,
    testing::Values(QuerierTestCase{.selector = {.matchers = {{.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactMatch}},
                                                              {.matcher = {.name = "task", .value = "php", .type = LabelMatcher::Type::kExactNotMatch}}}},
                                    .expected = {.series_id_list = {0, 2}, .status = QuerierStatus::kMatch}},
                    QuerierTestCase{.selector = {.matchers = {{.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactMatch}},
                                                              {.matcher = {.name = "task", .value = "", .type = LabelMatcher::Type::kExactMatch}}}},
                                    .expected = {.series_id_list = {}, .status = QuerierStatus::kNoMatch}},
                    QuerierTestCase{.selector = {.matchers = {{.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactMatch}},
                                                              {.matcher = {.name = "task", .value = "unknown", .type = LabelMatcher::Type::kExactNotMatch}}}},
                                    .expected = {.series_id_list = {0, 1, 2}, .status = QuerierStatus::kMatch}},
                    QuerierTestCase{.selector = {.matchers = {{.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactMatch}},
                                                              {.matcher = {.name = "job1", .value = "^$", .type = LabelMatcher::Type::kRegexpNotMatch}}}},
                                    .expected = {.series_id_list = {}, .status = QuerierStatus::kNoMatch}},
                    QuerierTestCase{.selector = {.matchers = {{.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactMatch}},
                                                              {.matcher = {.name = "job1", .value = ".+", .type = LabelMatcher::Type::kRegexpNotMatch}}}},
                                    .expected = {.series_id_list = {0, 1, 2}, .status = QuerierStatus::kMatch}},
                    QuerierTestCase{.selector = {.matchers = {{.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactMatch}},
                                                              {.matcher = {.name = "job", .value = ".+", .type = LabelMatcher::Type::kRegexpNotMatch}}}},
                                    .expected = {.series_id_list = {}, .status = QuerierStatus::kNoMatch}}));

INSTANTIATE_TEST_SUITE_P(
    AnythingMatcher,
    QuerierFixture,
    testing::Values(QuerierTestCase{.selector = {.matchers = {{.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactMatch}},
                                                              {.matcher = {.name = "job1", .value = ".*", .type = LabelMatcher::Type::kRegexpMatch}}}},
                                    .expected = {.series_id_list = {0, 1, 2}, .status = QuerierStatus::kMatch}},
                    QuerierTestCase{.selector = {.matchers = {{.matcher = {.name = "job", .value = "cron", .type = LabelMatcher::Type::kExactMatch}},
                                                              {.matcher = {.name = "job1", .value = ".*", .type = LabelMatcher::Type::kRegexpMatch}},
                                                              {.matcher = {.name = "task", .value = "php", .type = LabelMatcher::Type::kExactNotMatch}}}},
                                    .expected = {.series_id_list = {0, 2}, .status = QuerierStatus::kMatch}}));

}  // namespace