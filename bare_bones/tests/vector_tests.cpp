#include <iterator>

#include <gtest/gtest.h>

#include "bare_bones/vector.h"

namespace {

using BareBones::Vector;

TEST(BareBonesVector, should_increase_capacity_up_50_percent) {
  Vector<uint8_t> vec;
  size_t prev_cap = 0;
  size_t curr_cap = 0;

  for (int i = 0; i <= 64; ++i) {
    vec.push_back(1);
    if (vec.capacity() > curr_cap) {
      prev_cap = curr_cap;
      curr_cap = vec.capacity();
    }
  }

  ASSERT_EQ(vec.capacity() % 32, 0);
  ASSERT_GE((((vec.capacity() - prev_cap) / (prev_cap * 1.0)) * 100), 50);
}

TEST(BareBonesVector, should_increase_capacity_up_10_percent) {
  Vector<uint8_t> vec;
  size_t prev_cap = 0, curr_cap = 0;

  for (int i = 0; i <= 81920; ++i) {
    vec.push_back(1);
    if (vec.capacity() > curr_cap) {
      prev_cap = curr_cap;
      curr_cap = vec.capacity();
    }
  }

  ASSERT_EQ(vec.capacity() % 4096, 0);
  ASSERT_GE((((vec.capacity() - prev_cap) / (prev_cap * 1.0)) * 100), 10);
}

class BareBonesVectorAllocatedMemoryFixture : public testing::Test {
 protected:
  struct Object {
    [[nodiscard]] static size_t allocated_memory() noexcept { return kObjectAllocatedMemory; }
  };

  static constexpr size_t kObjectAllocatedMemory = 1;
};

TEST_F(BareBonesVectorAllocatedMemoryFixture, ObjectWithoutAllocatedMemoryMethod) {
  // Arrange
  using Vector = Vector<uint8_t>;

  Vector vector;
  vector.emplace_back(1);
  vector.emplace_back(2);
  vector.emplace_back(3);

  // Act
  auto allocated_memory = vector.allocated_memory();

  // Assert
  EXPECT_EQ(vector.capacity() * sizeof(Vector::value_type), allocated_memory);
}

TEST_F(BareBonesVectorAllocatedMemoryFixture, ObjectWithAllocatedMemoryMethod) {
  // Arrange
  using Vector = Vector<Object>;

  Vector vector;
  vector.emplace_back();
  vector.emplace_back();
  vector.emplace_back();

  // Act
  auto allocated_memory = vector.allocated_memory();

  // Assert
  EXPECT_EQ(vector.capacity() * sizeof(Vector::value_type) + vector.size() * kObjectAllocatedMemory, allocated_memory);
}

TEST_F(BareBonesVectorAllocatedMemoryFixture, PointerWithAllocatedMemoryMethod) {
  // Arrange
  using Vector = Vector<Object*>;

  Object object;

  Vector vector;
  vector.emplace_back(&object);
  vector.emplace_back(&object);
  vector.emplace_back(&object);

  // Act
  auto allocated_memory = vector.allocated_memory();

  // Assert
  EXPECT_EQ(vector.capacity() * sizeof(Vector::value_type) + vector.size() * kObjectAllocatedMemory, allocated_memory);
}

TEST_F(BareBonesVectorAllocatedMemoryFixture, DereferencableWithAllocatedMemoryMethod) {
  // Arrange
  using Vector = Vector<std::unique_ptr<Object>>;

  Vector vector;
  vector.emplace_back(std::make_unique<Object>());
  vector.emplace_back(std::make_unique<Object>());
  vector.emplace_back(std::make_unique<Object>());

  // Act
  auto allocated_memory = vector.allocated_memory();

  // Assert
  EXPECT_EQ(vector.capacity() * sizeof(Vector::value_type) + vector.size() * kObjectAllocatedMemory, allocated_memory);
}

}  // namespace
