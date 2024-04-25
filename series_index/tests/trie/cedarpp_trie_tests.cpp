#include <gtest/gtest.h>

#include "regexp_searcher_test_cases.h"
#include "series_index/querier/regexp_searcher.h"
#include "series_index/trie/cedarpp_tree.h"

namespace {

using series_index::querier::RegexpParser;
using series_index::querier::RegexpSearcher;
using series_index::querier::regexp_tests::RegexpSearcherTestCase;
using series_index::trie::CedarMatchesList;
using series_index::trie::CedarTrie;

class CedarTrieRegexpSearcherFixture : public testing::TestWithParam<RegexpSearcherTestCase> {
 protected:
  CedarTrie trie_;
  CedarMatchesList::SeriesIdList matches_;
  CedarMatchesList matches_list_{matches_};
  RegexpSearcher<CedarTrie, CedarMatchesList> searcher_{matches_list_};
};

TEST_P(CedarTrieRegexpSearcherFixture, Test) {
  // Arrange
  uint32_t id = 0;
  for (auto& key : GetParam().trie_values) {
    trie_.insert(key, id++);
  }
  CedarMatchesList::SeriesIdList expected_matches;
  for (auto& key : GetParam().matches) {
    expected_matches.push_back(trie_.lookup(key).value_or(std::numeric_limits<uint32_t>::max()));
  }

  // Act
  std::ignore = searcher_.search(trie_, RegexpParser::parse(GetParam().regexp));

  // Assert
  std::sort(expected_matches.begin(), expected_matches.end());
  std::sort(matches_.begin(), matches_.end());
  EXPECT_EQ(expected_matches, matches_);
}

INSTANTIATE_REGEXP_SEARCHER_TEST_SUITE_P(CedarTrieRegexpSearcherFixture);

}  // namespace