#include <gtest/gtest.h>

#include "primitives/primitives.h"
#include "series_index/querier/set_operations.h"

namespace {

using PromPP::Primitives::LabelViewSet;
using series_index::querier::SeriesIdSpan;
using series_index::querier::SeriesSliceList;
using series_index::querier::SetMerger;
using series_index::querier::SetSubstractor;

struct SetMergerCase {
  std::vector<uint32_t> ids;
  SeriesSliceList offsets;
};

class SetMergerFixture : public testing::TestWithParam<SetMergerCase> {};

TEST_P(SetMergerFixture, Test) {
  // Arrange
  auto expected = GetParam().ids;
  std::sort(expected.begin(), expected.end());

  auto offsets = GetParam().offsets;
  auto temp_memory_ptr = std::make_unique<uint32_t[]>(expected.size());
  auto memory = const_cast<uint32_t*>(&GetParam().ids[0]);
  auto temp_memory = temp_memory_ptr.get();

  // Act
  auto merged = SetMerger::merge(offsets, memory, temp_memory);

  // Assert
  EXPECT_TRUE(std::ranges::equal(expected, merged));
}

INSTANTIATE_TEST_SUITE_P(TestCases,
                         SetMergerFixture,
                         testing::Values(SetMergerCase{.ids = {}, .offsets = {}},
                                         SetMergerCase{.ids = {0}, .offsets = {{.begin = 0, .end = 1}}},
                                         SetMergerCase{.ids = {1, 0}, .offsets = {{.begin = 0, .end = 1}, {.begin = 1, .end = 2}}},
                                         SetMergerCase{.ids = {3, 2, 1}, .offsets = {{.begin = 0, .end = 1}, {.begin = 1, .end = 2}, {.begin = 2, .end = 3}}},
                                         SetMergerCase{.ids = {4, 5, 2, 3, 1},
                                                       .offsets = {{.begin = 0, .end = 2}, {.begin = 2, .end = 4}, {.begin = 4, .end = 5}}}));

struct SetSubstracterCase {
  std::vector<uint32_t> set1;
  std::vector<uint32_t> set2;
  std::vector<uint32_t> expected;
};

class SetSubstractorFixture : public testing::TestWithParam<SetSubstracterCase> {};

TEST_P(SetSubstractorFixture, Test) {
  // Arrange
  auto set1 = GetParam().set1;

  // Act
  auto result = SetSubstractor::substract(SeriesIdSpan(set1.data(), set1.size()), GetParam().set2);

  // Assert
  EXPECT_TRUE(std::ranges::equal(GetParam().expected, result));
}

INSTANTIATE_TEST_SUITE_P(TestCases,
                         SetSubstractorFixture,
                         testing::Values(SetSubstracterCase{.set1 = {}, .set2 = {0, 1, 2}, .expected = {}},
                                         SetSubstracterCase{.set1 = {0, 1, 2, 3}, .set2 = {}, .expected = {0, 1, 2, 3}},
                                         SetSubstracterCase{.set1 = {0, 1, 2, 3}, .set2 = {4}, .expected = {0, 1, 2, 3}},
                                         SetSubstracterCase{.set1 = {0, 1, 2, 3}, .set2 = {0, 1, 2, 3}, .expected = {}},
                                         SetSubstracterCase{.set1 = {0, 1, 2, 3}, .set2 = {0, 1, 2, 3}, .expected = {}},
                                         SetSubstracterCase{.set1 = {5, 6, 7}, .set2 = {0, 1, 2, 3, 4, 5}, .expected = {6, 7}}));

}  // namespace