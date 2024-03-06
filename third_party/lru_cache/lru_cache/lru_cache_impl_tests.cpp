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
    cache_.insert("1", "1");
    cache_.insert("2", "2");
    cache_.insert("3", "3");
  }
};

TEST_F(NodeLruCacheFixtureWith3Items, RemoveNonExistingElement) {
  // Arrange

  // Act
  cache_.erase("4");

  // Assert
  EXPECT_EQ(3U, cache_.size());
}

TEST_F(NodeLruCacheFixtureWith3Items, RemoveOldestElement) {
  // Arrange

  // Act
  cache_.erase("1");
  auto it = cache_.rbegin();

  // Assert
  EXPECT_EQ(2U, cache_.size());
  EXPECT_EQ("2", it++->second);
  EXPECT_EQ("3", it++->second);
  EXPECT_EQ(cache_.rend(), it);
}

TEST_F(NodeLruCacheFixtureWith3Items, RemoveLatestElement) {
  // Arrange

  // Act
  cache_.erase("3");
  auto it = cache_.rbegin();

  // Assert
  EXPECT_EQ(2U, cache_.size());
  EXPECT_EQ("1", it++->second);
  EXPECT_EQ("2", it++->second);
  EXPECT_EQ(cache_.rend(), it);
}

TEST_F(NodeLruCacheFixtureWith3Items, RemoveMiddleElement) {
  // Arrange

  // Act
  cache_.erase("2");
  auto it = cache_.rbegin();

  // Assert
  EXPECT_EQ(2U, cache_.size());
  EXPECT_EQ("1", it++->second);
  EXPECT_EQ("3", it++->second);
  EXPECT_EQ(cache_.rend(), it);
}

TEST_F(NodeLruCacheFixtureWith3Items, AppendAfterRemoveLatestElement) {
  // Arrange

  // Act
  cache_.erase("3");
  cache_.insert("4", "4");
  auto it = cache_.rbegin();

  // Assert
  EXPECT_EQ(3U, cache_.size());
  EXPECT_EQ("1", it++->second);
  EXPECT_EQ("2", it++->second);
  EXPECT_EQ("4", it++->second);
  EXPECT_EQ(cache_.rend(), it);
}

}  // namespace