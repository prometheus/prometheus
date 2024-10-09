#include <gtest/gtest.h>

#include "primitives/primitives.h"
#include "primitives/snug_composites.h"
#include "series_index/queryable_encoding_bimap.h"
#include "series_index/trie/cedarpp_tree.h"

namespace {

using PromPP::Primitives::LabelViewSet;
using series_index::QueryableEncodingBimap;
using series_index::trie::CedarMatchesList;
using series_index::trie::CedarTrie;

class QueryableEncodingBimapFixture : public testing::Test {
 protected:
  using TrieIndex = series_index::TrieIndex<CedarTrie, CedarMatchesList>;
  using Index = QueryableEncodingBimap<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimapFilament, TrieIndex>;

  Index index_;
};

TEST_F(QueryableEncodingBimapFixture, EmplaceLabelSet) {
  // Arrange

  // Act
  index_.find_or_emplace(LabelViewSet{{"job", "cron"}});

  // Assert
  auto name_id = index_.trie_index().names_trie().lookup("job");
  EXPECT_TRUE(name_id);
  EXPECT_NE(nullptr, index_.reverse_index().get(*name_id));

  auto values_trie = index_.trie_index().values_trie(*name_id);
  ASSERT_NE(nullptr, values_trie);
  EXPECT_TRUE(values_trie->lookup("cron"));
}

TEST_F(QueryableEncodingBimapFixture, EmplaceInvalidLabel) {
  // Arrange

  // Act
  LabelViewSet ls{{"key", "value"}};
  for (auto& label : ls) {
    label.second = "";
    break;
  }
  auto ls_id = index_.find_or_emplace(ls);

  // Assert
  auto label = index_[ls_id].begin();
  EXPECT_FALSE(index_.trie_index().names_trie().lookup("key"));
  EXPECT_EQ(nullptr, index_.reverse_index().get(label.name_id()));
  EXPECT_EQ(nullptr, index_.trie_index().values_trie(label.name_id()));
}

TEST_F(QueryableEncodingBimapFixture, EmplaceLabelSetWithInvalidLabel) {
  // Arrange

  // Act
  LabelViewSet ls{{"job", "cron"}, {"key", "value"}, {"process", "php"}};
  for (auto& label : ls) {
    if (label.first == "key") {
      label.second = "";
      break;
    }
  }
  auto ls_id = index_.find_or_emplace(ls);

  // Assert
  {
    auto name_id = index_.trie_index().names_trie().lookup("job");
    EXPECT_TRUE(name_id);
    EXPECT_NE(nullptr, index_.reverse_index().get(*name_id));

    auto values_trie = index_.trie_index().values_trie(*name_id);
    ASSERT_NE(nullptr, values_trie);
    EXPECT_TRUE(values_trie->lookup("cron"));
  }

  {
    auto second_label = std::next(index_[ls_id].begin());
    auto series_ids = index_.reverse_index().get(second_label.name_id());

    EXPECT_FALSE(index_.trie_index().names_trie().lookup("key"));
    ASSERT_NE(nullptr, series_ids);
    EXPECT_TRUE(series_ids->is_empty());
    EXPECT_EQ(nullptr, index_.trie_index().values_trie(second_label.name_id()));
  }

  {
    auto name_id = index_.trie_index().names_trie().lookup("process");
    EXPECT_TRUE(name_id);
    EXPECT_NE(nullptr, index_.reverse_index().get(*name_id));

    auto values_trie = index_.trie_index().values_trie(*name_id);
    ASSERT_NE(nullptr, values_trie);
    EXPECT_TRUE(values_trie->lookup("php"));
  }
}

TEST_F(QueryableEncodingBimapFixture, EmplaceDuplicatedLabelSet) {
  // Arrange
  const auto label_set = LabelViewSet{{"job", "cron"}, {"key", ""}, {"process", "php"}};
  const auto label_set2 = LabelViewSet{{"job", "cron"}, {"key", ""}, {"process", "php1"}};

  // Act
  const auto ls_id1 = index_.find_or_emplace(label_set);
  const auto existing_ls_id = index_.find_or_emplace(label_set);
  const auto ls_id2 = index_.find_or_emplace(label_set2);

  // Assert
  EXPECT_EQ(ls_id1, existing_ls_id);
  EXPECT_NE(ls_id1, ls_id2);
}

}  // namespace
