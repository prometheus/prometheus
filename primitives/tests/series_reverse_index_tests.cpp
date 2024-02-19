#include <gmock/gmock.h>

#include "primitives/series_reverse_index.h"

namespace {

using PromPP::Primitives::CompactSeriesIdSequence;
using PromPP::Primitives::LabelReverseIndex;
using PromPP::Primitives::SeriesReverseIndex;

class CompactSeriesIdSequenceFixture : public testing::Test {};

TEST_F(CompactSeriesIdSequenceFixture, SwitchToSequence) {
  // Arrange
  CompactSeriesIdSequence sequence{CompactSeriesIdSequence::Type::kArray};
  for (uint32_t i = 0; i < CompactSeriesIdSequence::kMaxElementsInArray; ++i) {
    sequence.push_back(i);
  }

  // Act
  auto type_before = sequence.type();
  sequence.push_back(CompactSeriesIdSequence::kMaxElementsInArray);
  auto type_after = sequence.type();

  // Assert
  EXPECT_EQ(CompactSeriesIdSequence::Type::kArray, type_before);
  EXPECT_EQ(CompactSeriesIdSequence::Type::kSequence, type_after);
  EXPECT_EQ(CompactSeriesIdSequence::kMaxElementsInArray + 1, sequence.count());
}

TEST_F(CompactSeriesIdSequenceFixture, IterateOverEmptyArray) {
  // Arrange
  CompactSeriesIdSequence sequence{CompactSeriesIdSequence::Type::kArray};

  // Act

  // Assert
  EXPECT_THAT(sequence.array(), testing::ElementsAre());
}

TEST_F(CompactSeriesIdSequenceFixture, IterateOverFilledArray) {
  // Arrange
  CompactSeriesIdSequence sequence{CompactSeriesIdSequence::Type::kArray};

  // Act
  sequence.push_back(0);
  sequence.push_back(1);

  // Assert
  EXPECT_THAT(sequence.array(), testing::ElementsAre(0U, 1U));
}

TEST_F(CompactSeriesIdSequenceFixture, IterateOverEmptySequence) {
  // Arrange
  CompactSeriesIdSequence sequence{CompactSeriesIdSequence::Type::kSequence};

  // Act

  // Assert
  EXPECT_THAT(sequence.sequence(), testing::ElementsAre());
}

TEST_F(CompactSeriesIdSequenceFixture, IterateOverFilledSequence) {
  // Arrange
  CompactSeriesIdSequence sequence{CompactSeriesIdSequence::Type::kSequence};

  // Act
  sequence.push_back(0);
  sequence.push_back(1);

  // Assert
  EXPECT_THAT(sequence.sequence(), testing::ElementsAre(0U, 1U));
}

class LabelReverseIndexFixture : public testing::Test {
 protected:
  LabelReverseIndex index_;
};

TEST_F(LabelReverseIndexFixture, GetNonExistingLabelValue) {
  // Arrange

  // Act
  auto item = index_.get(0);

  // Assert
  ASSERT_EQ(nullptr, item);
}

TEST_F(LabelReverseIndexFixture, AddIntoNewLabelValue) {
  // Arrange

  // Act
  index_.add(0, 0);
  auto item = index_.get(0);

  // Assert
  ASSERT_NE(nullptr, item);
  ASSERT_EQ(CompactSeriesIdSequence::Type::kArray, item->type());
  EXPECT_THAT(item->array(), testing::ElementsAre(0U));
  EXPECT_THAT(index_.get_all()->sequence(), testing::ElementsAre(0U));
}

TEST_F(LabelReverseIndexFixture, AddIntoExistingLabelValue) {
  // Arrange

  // Act
  index_.add(0, 0);
  index_.add(0, 1);
  auto item = index_.get(0);

  // Assert
  ASSERT_NE(nullptr, item);
  ASSERT_EQ(CompactSeriesIdSequence::Type::kArray, item->type());
  EXPECT_THAT(item->array(), testing::ElementsAre(0U, 1U));
  EXPECT_THAT(index_.get_all()->sequence(), testing::ElementsAre(0U, 1U));
}

TEST_F(LabelReverseIndexFixture, AddMultipleLabelValues) {
  // Arrange

  // Act
  index_.add(0, 0);
  index_.add(1, 1);
  auto item0 = index_.get(0);
  auto item1 = index_.get(1);

  // Assert
  ASSERT_NE(nullptr, item0);
  ASSERT_EQ(CompactSeriesIdSequence::Type::kArray, item0->type());
  EXPECT_THAT(item0->array(), testing::ElementsAre(0U));

  ASSERT_NE(nullptr, item1);
  ASSERT_EQ(CompactSeriesIdSequence::Type::kArray, item1->type());
  EXPECT_THAT(item1->array(), testing::ElementsAre(1U));

  EXPECT_THAT(index_.get_all()->sequence(), testing::ElementsAre(0U, 1U));
}

class SeriesReverseIndexFixture : public testing::Test {
 protected:
  struct Label {
    uint32_t label_name_id;
    uint32_t label_value_id;

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t name_id() const noexcept { return label_name_id; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t value_id() const noexcept { return label_value_id; }
  };

  SeriesReverseIndex index_;
};

TEST_F(SeriesReverseIndexFixture, GetNonExistingLabelName) {
  // Arrange

  // Act
  auto item = index_.get(0);

  // Assert
  EXPECT_EQ(nullptr, item);
}

TEST_F(SeriesReverseIndexFixture, AddIntoNewLabelName) {
  // Arrange

  // Act
  index_.add(Label{.label_name_id = 0, .label_value_id = 0}, 0);
  auto item = index_.get(0);

  // Assert
  ASSERT_NE(nullptr, item);
  ASSERT_EQ(CompactSeriesIdSequence::Type::kSequence, item->type());
  EXPECT_THAT(item->sequence(), testing::ElementsAre(0U));
}

TEST_F(SeriesReverseIndexFixture, AddIntoExistingLabelName) {
  // Arrange

  // Act
  index_.add(Label{.label_name_id = 0, .label_value_id = 0}, 0);
  index_.add(Label{.label_name_id = 0, .label_value_id = 0}, 1);
  auto item = index_.get(0);

  // Assert
  ASSERT_NE(nullptr, item);
  ASSERT_EQ(CompactSeriesIdSequence::Type::kSequence, item->type());
  EXPECT_THAT(item->sequence(), testing::ElementsAre(0U, 1U));
}

TEST_F(SeriesReverseIndexFixture, AddMultipleLabelNames) {
  // Arrange

  // Act
  index_.add(Label{.label_name_id = 0, .label_value_id = 0}, 0);
  index_.add(Label{.label_name_id = 1, .label_value_id = 0}, 1);
  auto item0 = index_.get(0);
  auto item1 = index_.get(1);

  // Assert
  ASSERT_NE(nullptr, item0);
  ASSERT_EQ(CompactSeriesIdSequence::Type::kSequence, item0->type());
  EXPECT_THAT(item0->sequence(), testing::ElementsAre(0U));

  ASSERT_NE(nullptr, item1);
  ASSERT_EQ(CompactSeriesIdSequence::Type::kSequence, item1->type());
  EXPECT_THAT(item1->sequence(), testing::ElementsAre(1U));
}

TEST_F(SeriesReverseIndexFixture, GetByNameAndValueId) {
  // Arrange

  // Act
  index_.add(Label{.label_name_id = 0, .label_value_id = 0}, 0);
  index_.add(Label{.label_name_id = 0, .label_value_id = 1}, 1);
  auto item0 = index_.get(0, 0);
  auto item1 = index_.get(0, 1);
  auto item2 = index_.get(0, 2);

  // Assert
  ASSERT_NE(nullptr, item0);
  ASSERT_EQ(CompactSeriesIdSequence::Type::kArray, item0->type());
  EXPECT_THAT(item0->array(), testing::ElementsAre(0U));

  ASSERT_NE(nullptr, item1);
  ASSERT_EQ(CompactSeriesIdSequence::Type::kArray, item1->type());
  EXPECT_THAT(item1->array(), testing::ElementsAre(1U));

  EXPECT_EQ(nullptr, item2);
}

}  // namespace
