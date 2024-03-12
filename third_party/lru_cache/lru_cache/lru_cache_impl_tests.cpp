#include <gtest/gtest.h>

#include "lru_cache/default_providers.h"
#include "lru_cache/lru_cache.h"

namespace {

class NodeLruCacheFixture : public testing::Test {
 protected:
  using Key = std::string;
  using Value = std::string;
  using ValueProvider = decltype(&lru_cache::internal::throwing_value_producer<Key, Value>);
  using DropCallback = void (*)(Key, Value);
  using Cache = lru_cache::NodeLruCache<Key, Value, ValueProvider, DropCallback>;
};

TEST_F(NodeLruCacheFixture, RemoveItem) {
  // Arrange
  Cache cache{2};
  cache.insert("1", "1");

  // Act
  cache.erase("1");

  // Assert
  EXPECT_EQ(0U, cache.size());
  EXPECT_EQ(cache.end(), cache.begin());
}

class NodeLruCacheFixtureWith3Items : public NodeLruCacheFixture {
 protected:
  Cache cache_{3};

  void SetUp() override {
    cache_.insert("key_1", "value_1");
    cache_.insert("key_2", "value_2");
    cache_.insert("key_3", "value_3");
  }
};

TEST_F(NodeLruCacheFixtureWith3Items, EraseNonExistingElement) {
  // Arrange

  // Act
  cache_.erase("key_4");

  // Assert
  EXPECT_EQ(3U, cache_.size());
}

TEST_F(NodeLruCacheFixtureWith3Items, EraseOldestElement) {
  // Arrange

  // Act
  cache_.erase("key_1");
  auto it = cache_.rbegin();

  // Assert
  EXPECT_EQ(2U, cache_.size());
  EXPECT_EQ("value_2", it++->second);
  EXPECT_EQ("value_3", it++->second);
  EXPECT_EQ(cache_.rend(), it);
}

TEST_F(NodeLruCacheFixtureWith3Items, EraseLatestElement) {
  // Arrange

  // Act
  cache_.erase("key_3");
  auto it = cache_.rbegin();

  // Assert
  EXPECT_EQ(2U, cache_.size());
  EXPECT_EQ("value_1", it++->second);
  EXPECT_EQ("value_2", it++->second);
  EXPECT_EQ(cache_.rend(), it);
}

TEST_F(NodeLruCacheFixtureWith3Items, EraseMiddleElement) {
  // Arrange

  // Act
  cache_.erase("key_2");
  auto it = cache_.rbegin();

  // Assert
  EXPECT_EQ(2U, cache_.size());
  EXPECT_EQ("value_1", it++->second);
  EXPECT_EQ("value_3", it++->second);
  EXPECT_EQ(cache_.rend(), it);
}

TEST_F(NodeLruCacheFixtureWith3Items, AppendAfterEraseLatestElement) {
  // Arrange

  // Act
  cache_.erase("key_3");
  cache_.insert("key_4", "value_4");
  auto it = cache_.rbegin();

  // Assert
  EXPECT_EQ(3U, cache_.size());
  EXPECT_EQ("value_1", it++->second);
  EXPECT_EQ("value_2", it++->second);
  EXPECT_EQ("value_4", it++->second);
  EXPECT_EQ(cache_.rend(), it);
}

TEST_F(NodeLruCacheFixtureWith3Items, OldestTest) {
  // Arrange

  // Act
  auto oldest = cache_.oldest();

  // Assert
  EXPECT_EQ("key_1", oldest.first);
  EXPECT_EQ("value_1", oldest.second);
}

}  // namespace