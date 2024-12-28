#include <gtest/gtest.h>

#include <algorithm>

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

struct TrieItem {
  std::string key;
  uint32_t value;

  bool operator==(const TrieItem&) const noexcept = default;
  bool operator<(const TrieItem& other) const noexcept { return key < other.key; }
};

class CedarTrieFixture {
 protected:
  CedarTrie trie_;

  [[nodiscard]] static std::vector<TrieItem> items(const CedarTrie& trie) {
    std::vector<TrieItem> actual;
    for (auto it = trie.make_enumerative_iterator(); it.is_valid(); ++it) {
      actual.emplace_back(TrieItem{.key = std::string(it.key()), .value = it.value()});
    }
    return actual;
  }
};

class CedarTrieZeroByteSupportFixture : public CedarTrieFixture, public testing::Test {};

TEST_F(CedarTrieZeroByteSupportFixture, InsertAndLookupStringWithZeroBytes) {
  // Arrange
  static constexpr auto kKey = "HetznerFinland:\x00:nova"sv;
  static constexpr auto kValue = 123U;

  // Act
  trie_.insert(kKey, kValue);
  const auto value1 = trie_.lookup(kKey);
  const auto value2 = trie_.lookup("HetznerFinland:");

  // Assert
  EXPECT_EQ(kValue, value1.value_or(0));
  EXPECT_FALSE(value2);
}

struct CedarEnumerativeIteratorCase {
  std::vector<TrieItem> items;
  std::vector<TrieItem> expected;
};

class CedarEnumerativeIteratorFixture : public CedarTrieFixture, public testing::TestWithParam<CedarEnumerativeIteratorCase> {
 protected:
  void SetUp() final {
    for (auto& item : GetParam().items) {
      trie_.insert(item.key, item.value);
    }
  }
};

TEST_P(CedarEnumerativeIteratorFixture, Test) {
  // Arrange

  // Act
  const auto actual = items(trie_);

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

class CedarTrieRegexpSearcherFixture : public CedarTrieFixture, public testing::TestWithParam<RegexpSearcherTestCase> {
 protected:
  CedarMatchesList::SeriesIdList matches_;
  CedarMatchesList matches_list_{matches_};
  RegexpSearcher<CedarTrie, CedarMatchesList> searcher_{matches_list_};

  void SetUp() final {
    uint32_t id = 0;
    for (auto& key : GetParam().trie_values) {
      trie_.insert(key, id++);
    }
  }

  [[nodiscard]] CedarMatchesList::SeriesIdList get_expected_matches() const {
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
  std::ranges::sort(expected_matches);
  std::ranges::sort(matches_);
  EXPECT_EQ(expected_matches, matches_);
}

INSTANTIATE_REGEXP_SEARCHER_TEST_SUITE_P(CedarTrieRegexpSearcherFixture);

struct SerializeDeserializeCase {
  std::vector<TrieItem> items;
};

class CedarTrieSerializeDeserializeFixture : public CedarTrieFixture, public ::testing::TestWithParam<SerializeDeserializeCase> {
 protected:
  void SetUp() final {
    for (const auto& [key, value] : GetParam().items) {
      trie_.insert(key, value);
    }
  }
};

TEST_P(CedarTrieSerializeDeserializeFixture, Test) {
  // Arrange
  std::stringstream stream;
  CedarTrie trie2;

  // Act
  stream << trie_;
  stream >> trie2;

  // Assert
  auto& sorted_items = const_cast<SerializeDeserializeCase&>(GetParam()).items;
  std::sort(sorted_items.begin(), sorted_items.end());
  EXPECT_EQ(GetParam().items, items(trie2));
}

INSTANTIATE_TEST_SUITE_P(EmptyTrie, CedarTrieSerializeDeserializeFixture, testing::Values(SerializeDeserializeCase{}));
INSTANTIATE_TEST_SUITE_P(Cases,
                         CedarTrieSerializeDeserializeFixture,
                         testing::Values(SerializeDeserializeCase{.items = {{.key = "key", .value = 1}}},
                                         SerializeDeserializeCase{.items = {
                                                                      {.key = "key1", .value = 1},
                                                                      {.key = "key2", .value = 2},
                                                                      {.key = "key3", .value = 3},
                                                                  }}));

}  // namespace