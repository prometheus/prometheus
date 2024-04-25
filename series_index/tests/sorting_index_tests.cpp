#include <gmock/gmock.h>

#include "series_index/sorting_index.h"

namespace {

using series_index::SortingIndex;
using std::string_view_literals::operator""sv;

class SortingIndexFixture : public testing::Test {
 public:
  static constexpr std::array kItems = {"b"sv, "d"sv, "a"sv, "c"sv};

 protected:
  struct LessComparator {
    PROMPP_ALWAYS_INLINE bool operator()(uint32_t a, uint32_t b) const noexcept { return SortingIndexFixture::kItems[a] < SortingIndexFixture::kItems[b]; }
  };
  using Set = phmap::btree_set<uint32_t, LessComparator>;

  Set set_{{}, LessComparator{}};
  SortingIndex<Set, kItems.size() + 1> index_{set_};
};

static constexpr uint32_t operator""_idx(const char* value, size_t len) noexcept {
  return std::ranges::find(SortingIndexFixture::kItems, std::string_view(value, len)) - SortingIndexFixture::kItems.begin();
}

TEST_F(SortingIndexFixture, BuildAndSort) {
  // Arrange
  set_.emplace(0);
  set_.emplace(1);
  set_.emplace(2);
  set_.emplace(3);
  std::array series_ids{"d"_idx, "b"_idx, "c"_idx, "a"_idx};

  // Act
  index_.build();
  index_.sort(series_ids.begin(), series_ids.end());

  // Assert
  EXPECT_FALSE(index_.empty());
  EXPECT_THAT(series_ids, testing::ElementsAre("a"_idx, "b"_idx, "c"_idx, "d"_idx));
}

TEST_F(SortingIndexFixture, UpdateAndSort) {
  // Arrange
  index_.update(set_.emplace(0).first);
  index_.update(set_.emplace(1).first);
  index_.update(set_.emplace(2).first);
  std::array series_ids{"d"_idx, "b"_idx, "a"_idx};

  // Act
  index_.sort(series_ids.begin(), series_ids.end());

  // Assert
  EXPECT_FALSE(index_.empty());
  EXPECT_THAT(series_ids, testing::ElementsAre("a"_idx, "b"_idx, "d"_idx));
}

TEST_F(SortingIndexFixture, RebuildIndex) {
  // Arrange
  index_.update(set_.emplace(0).first);
  index_.update(set_.emplace(1).first);
  index_.update(set_.emplace(2).first);
  index_.update(set_.emplace(3).first);
  std::array series_ids{"d"_idx, "b"_idx, "c"_idx, "a"_idx};

  // Act
  index_.sort(series_ids.begin(), series_ids.end());

  // Assert
  EXPECT_FALSE(index_.empty());
  EXPECT_THAT(series_ids, testing::ElementsAre("a"_idx, "b"_idx, "c"_idx, "d"_idx));
}

}  // namespace