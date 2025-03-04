#include <gtest/gtest.h>

#include <algorithm>

#include "series_index/trie/cedarpp_tree.h"
#include "series_index/trie_index.h"

namespace {

using TrieIndex = series_index::TrieIndex<series_index::trie::CedarTrie, series_index::trie::CedarMatchesList>;

struct TrieIndexItem {
  std::string name;
  uint32_t name_id;
  std::string value;
  uint32_t value_id;

  bool operator==(const TrieIndexItem&) const noexcept = default;
};

struct TrieIndexIteratorCase {
  std::vector<TrieIndexItem> items;
  std::vector<TrieIndexItem> expected;
};

class TrieIndexIteratorFixture : public testing::TestWithParam<TrieIndexIteratorCase> {
 protected:
  TrieIndex index_;

  void SetUp() final {
    for (auto& item : GetParam().items) {
      index_.insert(item.name, item.name_id, item.value, item.value_id);
    }
  }
};

TEST_P(TrieIndexIteratorFixture, Test) {
  // Arrange
  std::vector<TrieIndexItem> actual;

  // Act
  std::ranges::transform(index_, std::back_inserter(actual), [](const auto& item) PROMPP_LAMBDA_INLINE {
    return TrieIndexItem{.name = std::string(item.name()), .name_id = item.name_id(), .value = std::string(item.value()), .value_id = item.value_id()};
  });

  // Assert
  EXPECT_EQ(GetParam().expected, actual);
}

INSTANTIATE_TEST_SUITE_P(EmptyTrie, TrieIndexIteratorFixture, testing::Values(TrieIndexIteratorCase{}));
INSTANTIATE_TEST_SUITE_P(NameWithOneValue,
                         TrieIndexIteratorFixture,
                         testing::Values(TrieIndexIteratorCase{.items = {{.name = "name", .name_id = 0, .value = "value", .value_id = 0}},
                                                               .expected = {{{.name = "name", .name_id = 0, .value = "value", .value_id = 0}}}}));
INSTANTIATE_TEST_SUITE_P(NameWithTwoValues,
                         TrieIndexIteratorFixture,
                         testing::Values(TrieIndexIteratorCase{.items =
                                                                   {
                                                                       {.name = "name", .name_id = 0, .value = "value1", .value_id = 0},
                                                                       {.name = "name", .name_id = 0, .value = "value2", .value_id = 1},
                                                                   },
                                                               .expected = {
                                                                   {{.name = "name", .name_id = 0, .value = "value1", .value_id = 0},
                                                                    {.name = "name", .name_id = 0, .value = "value2", .value_id = 1}},
                                                               }}));
INSTANTIATE_TEST_SUITE_P(TwoNames,
                         TrieIndexIteratorFixture,
                         testing::Values(TrieIndexIteratorCase{.items =
                                                                   {
                                                                       {.name = "name1", .name_id = 0, .value = "value", .value_id = 0},
                                                                       {.name = "name2", .name_id = 1, .value = "value2", .value_id = 0},
                                                                   },
                                                               .expected = {
                                                                   {.name = "name1", .name_id = 0, .value = "value", .value_id = 0},
                                                                   {.name = "name2", .name_id = 1, .value = "value2", .value_id = 0},
                                                               }}));
INSTANTIATE_TEST_SUITE_P(SortingNameAndValues,
                         TrieIndexIteratorFixture,
                         testing::Values(TrieIndexIteratorCase{.items =
                                                                   {
                                                                       {.name = "name2", .name_id = 0, .value = "value2", .value_id = 0},
                                                                       {.name = "name2", .name_id = 0, .value = "value1", .value_id = 1},
                                                                       {.name = "name1", .name_id = 1, .value = "value2", .value_id = 0},
                                                                       {.name = "name1", .name_id = 1, .value = "value1", .value_id = 1},
                                                                   },
                                                               .expected = {
                                                                   {.name = "name1", .name_id = 1, .value = "value1", .value_id = 1},
                                                                   {.name = "name1", .name_id = 1, .value = "value2", .value_id = 0},
                                                                   {.name = "name2", .name_id = 0, .value = "value1", .value_id = 1},
                                                                   {.name = "name2", .name_id = 0, .value = "value2", .value_id = 0},
                                                               }}));

}  // namespace