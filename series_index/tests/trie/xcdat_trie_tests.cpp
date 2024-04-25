#include <gtest/gtest.h>

#include "regexp_searcher_test_cases.h"
#include "series_index/querier/regexp_searcher.h"
#include "series_index/trie/xcdat_tree.h"

namespace {

using series_index::querier::RegexpParser;
using series_index::querier::RegexpSearcher;
using series_index::querier::regexp_tests::RegexpSearcherTestCase;
using series_index::trie::XcdatMatchesList;

class XcdatTrieRegexpSearcherFixture : public testing::TestWithParam<RegexpSearcherTestCase> {
 protected:
  XcdatMatchesList::SeriesIdList matches_;
  XcdatMatchesList matches_list_{matches_};
  RegexpSearcher<xcdat::trie_15_type, XcdatMatchesList> searcher_{matches_list_};
};

TEST_P(XcdatTrieRegexpSearcherFixture, Test) {
  // Arrange
  std::sort(const_cast<ParamType&>(GetParam()).trie_values.begin(), const_cast<ParamType&>(GetParam()).trie_values.end());
  xcdat::trie_15_type trie(GetParam().trie_values);
  XcdatMatchesList::SeriesIdList expected_matches;
  for (auto& key : GetParam().matches) {
    expected_matches.push_back(trie.lookup(key).value_or(std::numeric_limits<uint64_t>::max()));
  }

  // Act
  std::ignore = searcher_.search(trie, RegexpParser::parse(GetParam().regexp));

  // Assert
  std::sort(expected_matches.begin(), expected_matches.end());
  std::sort(matches_.begin(), matches_.end());
  EXPECT_EQ(expected_matches, matches_);
}

INSTANTIATE_REGEXP_SEARCHER_TEST_SUITE_P(XcdatTrieRegexpSearcherFixture);

}  // namespace