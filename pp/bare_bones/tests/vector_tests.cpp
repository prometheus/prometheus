#include <gtest/gtest.h>

#include "bare_bones/vector.h"

namespace {

using BareBones::Vector;

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
  const auto allocated_memory = vector.allocated_memory();

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
  const auto allocated_memory = vector.allocated_memory();

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
  const auto allocated_memory = vector.allocated_memory();

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
  const auto allocated_memory = vector.allocated_memory();

  // Assert
  EXPECT_EQ(vector.capacity() * sizeof(Vector::value_type) + vector.size() * kObjectAllocatedMemory, allocated_memory);
}

TEST(BareBonesVector, InitializerListConstructor) {
  // Arrange

  // Act
  Vector<std::string> vector{"123", "456", "789"};

  // Assert
  EXPECT_EQ(3U, vector.size());
  EXPECT_EQ("123", vector[0]);
  EXPECT_EQ("456", vector[1]);
  EXPECT_EQ("789", vector[2]);
}

}  // namespace
