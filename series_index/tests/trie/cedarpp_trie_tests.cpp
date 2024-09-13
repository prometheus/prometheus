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
using std::operator""sv;
using std::operator""s;

class CedarTrieFixture : public testing::Test {
 protected:
  CedarTrie trie_;
};

TEST_F(CedarTrieFixture, InsertAndLookupStringWithZeroBytes) {
  // Arrange
  static constexpr auto kKey = "HetznerFinland:\x00:nova"sv;
  static constexpr auto kValue = 123U;

  // Act
  trie_.insert(kKey, kValue);
  auto value1 = trie_.lookup(kKey);
  auto value2 = trie_.lookup("HetznerFinland:");

  // Assert
  EXPECT_EQ(kValue, value1.value_or(0));
  EXPECT_FALSE(value2);
}

struct TrieItem {
  std::string key;
  uint32_t value;

  bool operator==(const TrieItem&) const noexcept = default;
};

struct CedarEnumerativeIteratorCase {
  std::vector<TrieItem> items;
  std::vector<TrieItem> expected;
};

class CedarEnumerativeIteratorFixture : public testing::TestWithParam<CedarEnumerativeIteratorCase> {
 protected:
  CedarTrie trie_;

  void SetUp() final {
    for (auto& item : GetParam().items) {
      trie_.insert(item.key, item.value);
    }
  }

  std::vector<TrieItem> items() {
    std::vector<TrieItem> actual;
    for (auto it = trie_.make_enumerative_iterator(); it.is_valid(); ++it) {
      actual.emplace_back(TrieItem{.key = std::string(it.key()), .value = it.value()});
    }
    return actual;
  }
};

TEST_P(CedarEnumerativeIteratorFixture, Test) {
  // Arrange

  // Act
  auto actual = items();

  // Assert
  EXPECT_EQ(GetParam().expected, actual);
}

INSTANTIATE_TEST_SUITE_P(ValueWithZeroByte,
                         CedarEnumerativeIteratorFixture,
                         testing::Values(CedarEnumerativeIteratorCase{.items =
                                                                          {
                                                                              {.key = "HetznerFinland:\x00:nova"s, .value = 0},
                                                                              {.key = "HetznerFinland:\x00:mova"s, .value = 1},
                                                                              {.key = "HetznerAlaska:\x00:kova"s, .value = 2},
                                                                          },
                                                                      .expected =
                                                                          {
                                                                              {.key = "HetznerAlaska:\x00:kova"s, .value = 2},
                                                                              {.key = "HetznerFinland:\x00:mova"s, .value = 1},
                                                                              {.key = "HetznerFinland:\x00:nova"s, .value = 0},
                                                                          }},
                                         CedarEnumerativeIteratorCase{.items =
                                                                          {
                                                                              {.key = "\x00\x00"s, .value = 0},
                                                                              {.key = "\x00"s, .value = 1},
                                                                          },
                                                                      .expected =
                                                                          {
                                                                              {.key = "\x00"s, .value = 1},
                                                                              {.key = "\x00\x00"s, .value = 0},
                                                                          }},
                                         CedarEnumerativeIteratorCase{.items =
                                                                          {
                                                                              {.key = "\x01\x01"s, .value = 0},
                                                                              {.key = "\x01\x00"s, .value = 1},
                                                                          },
                                                                      .expected =
                                                                          {
                                                                              {.key = "\x01\x00"s, .value = 1},
                                                                              {.key = "\x01\x01"s, .value = 0},
                                                                          }},
                                         CedarEnumerativeIteratorCase{.items =
                                                                          {
                                                                              {.key = "\x01"s, .value = 0},
                                                                              {.key = "\x00"s, .value = 1},
                                                                          },
                                                                      .expected = {
                                                                          {.key = "\x00"s, .value = 1},
                                                                          {.key = "\x01"s, .value = 0},
                                                                      }}));

class CedarTrieRegexpSearcherFixture : public testing::TestWithParam<RegexpSearcherTestCase> {
 protected:
  CedarTrie trie_;
  CedarMatchesList::SeriesIdList matches_;
  CedarMatchesList matches_list_{matches_};
  RegexpSearcher<CedarTrie, CedarMatchesList> searcher_{matches_list_};

  void SetUp() final {
    uint32_t id = 0;
    for (auto& key : GetParam().trie_values) {
      trie_.insert(key, id++);
    }
  }

  CedarMatchesList::SeriesIdList get_expected_matches() {
    CedarMatchesList::SeriesIdList expected_matches;
    for (auto& key : GetParam().matches) {
      expected_matches.push_back(trie_.lookup(key).value_or(std::numeric_limits<uint32_t>::max()));
    }

    return expected_matches;
  }
};

TEST_P(CedarTrieRegexpSearcherFixture, Test) {
  // Arrange
  auto expected_matches = get_expected_matches();

  // Act
  std::ignore = searcher_.search(trie_, RegexpParser::parse(GetParam().regexp));

  // Assert
  std::sort(expected_matches.begin(), expected_matches.end());
  std::sort(matches_.begin(), matches_.end());
  EXPECT_EQ(expected_matches, matches_);
}

INSTANTIATE_REGEXP_SEARCHER_TEST_SUITE_P(CedarTrieRegexpSearcherFixture);

}  // namespace