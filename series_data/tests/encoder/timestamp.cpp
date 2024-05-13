#include <gtest/gtest.h>

#include "series_data/encoder/encoder.h"

namespace {

using series_data::encoder::timestamp::Encoder;
using series_data::encoder::timestamp::kInvalidStateId;
using series_data::encoder::timestamp::StateId;

class TimestampEncoderFixture : public testing::Test {
 protected:
  Encoder encoder_;
};

TEST_F(TimestampEncoderFixture, OneStateForTwoSeries) {
  // Arrange

  // Act
  auto state_id1 = encoder_.encode(kInvalidStateId, 101);
  auto state_id2 = encoder_.encode(kInvalidStateId, 101);

  // Assert
  EXPECT_EQ(0U, state_id1);
  EXPECT_EQ(0U, state_id2);
  EXPECT_EQ(101, encoder_.get_encoder(state_id1).last_timestamp());
}

TEST_F(TimestampEncoderFixture, TransitionToNewStateWithSavingPreviousState) {
  // Arrange
  encoder_.encode(kInvalidStateId, 101);

  // Act
  auto first_state_id = encoder_.encode(kInvalidStateId, 101);
  auto state_id = encoder_.encode(first_state_id, 102);

  // Assert
  EXPECT_EQ(0U, first_state_id);
  EXPECT_EQ(1U, state_id);
  EXPECT_EQ(101, encoder_.get_encoder(first_state_id).last_timestamp());
  EXPECT_EQ(102, encoder_.get_encoder(state_id).last_timestamp());
}

TEST_F(TimestampEncoderFixture, TransitionToNewStateWithErasingPreviousState) {
  // Arrange

  // Act
  auto first_state_id = encoder_.encode(kInvalidStateId, 101);
  auto state_id = encoder_.encode(first_state_id, 102);

  // Assert
  EXPECT_EQ(0U, first_state_id);
  EXPECT_EQ(0U, state_id);
  EXPECT_EQ(102, encoder_.get_encoder(state_id).last_timestamp());
  EXPECT_EQ(1U, encoder_.encode(kInvalidStateId, 102));
}

TEST_F(TimestampEncoderFixture, TransitionToExistingStateWithErasingPreviousState) {
  // Arrange

  // Act

  // Assert
  EXPECT_EQ(0U, encoder_.encode(kInvalidStateId, 101));
  EXPECT_EQ(0U, encoder_.encode(kInvalidStateId, 101));
  EXPECT_EQ(1U, encoder_.encode(0, 102));
  EXPECT_EQ(1U, encoder_.encode(0, 102));
  EXPECT_EQ(0U, encoder_.encode(1, 103));
  EXPECT_EQ(0U, encoder_.encode(1, 103));

  EXPECT_EQ(103, encoder_.get_encoder(0).last_timestamp());
}

TEST_F(TimestampEncoderFixture, TransitionToExistingStateWithoutErasingPreviousState) {
  // Arrange

  // Act

  // Assert
  EXPECT_EQ(0U, encoder_.encode(kInvalidStateId, 101));
  EXPECT_EQ(0U, encoder_.encode(kInvalidStateId, 101));
  EXPECT_EQ(0U, encoder_.encode(kInvalidStateId, 101));
  EXPECT_EQ(1U, encoder_.encode(0, 102));
  EXPECT_EQ(1U, encoder_.encode(0, 102));
  EXPECT_EQ(2U, encoder_.encode(1, 103));
  EXPECT_EQ(2U, encoder_.encode(1, 103));
  EXPECT_EQ(1U, encoder_.encode(2, 104));

  EXPECT_EQ(101, encoder_.get_encoder(0).last_timestamp());
  EXPECT_EQ(103, encoder_.get_encoder(2).last_timestamp());
}

}  // namespace