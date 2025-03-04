#include <numeric>

#include <gtest/gtest.h>

#include "bare_bones/memory.h"

namespace {

using BareBones::AllocationSizeCalculator;
using BareBones::Memory;
using BareBones::MemoryControlBlock;
using BareBones::SharedMemory;
using BareBones::SharedPtr;

struct GetAllocationSizeCase {
  uint32_t needed_size;
  uint32_t expected_size;
};

class AllocationSizeCalculatorFixture : public testing::TestWithParam<GetAllocationSizeCase> {
 protected:
  using Calculator = AllocationSizeCalculator<char, uint32_t>;
};

TEST_P(AllocationSizeCalculatorFixture, TestCalculate) {
  // Arrange

  // Act
  const auto size = Calculator::calculate(GetParam().needed_size);

  // Assert
  EXPECT_EQ(GetParam().expected_size, size);
}

INSTANTIATE_TEST_SUITE_P(
    UpTo50PercentAlignedToMinAllocationSize,
    AllocationSizeCalculatorFixture,
    testing::Values(GetAllocationSizeCase{.needed_size = 0, .expected_size = AllocationSizeCalculator<char, uint32_t>::kMinAllocationSize},
                    GetAllocationSizeCase{.needed_size = static_cast<uint32_t>(AllocationSizeCalculator<char, uint32_t>::kMinAllocationSize * 0.66),
                                          .expected_size = AllocationSizeCalculator<char, uint32_t>::kMinAllocationSize},
                    GetAllocationSizeCase{.needed_size = static_cast<uint32_t>(AllocationSizeCalculator<char, uint32_t>::kMinAllocationSize * 0.66) + 1,
                                          .expected_size = AllocationSizeCalculator<char, uint32_t>::kMinAllocationSize * 2},
                    GetAllocationSizeCase{.needed_size = 170, .expected_size = 256},
                    GetAllocationSizeCase{.needed_size = 171, .expected_size = 512}));

INSTANTIATE_TEST_SUITE_P(UpTo50PercentAlignedTo256,
                         AllocationSizeCalculatorFixture,
                         testing::Values(GetAllocationSizeCase{.needed_size = 171, .expected_size = 512},
                                         GetAllocationSizeCase{.needed_size = 2730, .expected_size = 4096},
                                         GetAllocationSizeCase{.needed_size = 2731, .expected_size = 4096},
                                         GetAllocationSizeCase{.needed_size = 2732, .expected_size = 4096}));

INSTANTIATE_TEST_SUITE_P(UpTo10PercentAlignedTo4096,
                         AllocationSizeCalculatorFixture,
                         testing::Values(GetAllocationSizeCase{.needed_size = 4096, .expected_size = 8192}));

class MemoryFixture : public ::testing::Test {
 protected:
  Memory<MemoryControlBlock, uint8_t> memory_;
};

TEST_F(MemoryFixture, Empty) {
  // Arrange

  // Act

  // Assert
  EXPECT_EQ(0U, memory_.size());
  EXPECT_EQ(nullptr, memory_);
}

TEST_F(MemoryFixture, CopyConstructor) {
  // Arrange
  memory_.resize_to_fit_at_least(1);
  std::iota(memory_.begin(), memory_.end(), uint8_t{});

  // Act
  const auto memory2 = memory_;

  // Assert
  EXPECT_NE(memory2, memory_);
  ASSERT_EQ(memory_.size(), memory2.size());
  EXPECT_TRUE(std::ranges::equal(memory2, memory_));
}

TEST_F(MemoryFixture, CopyOperator) {
  // Arrange
  memory_.resize_to_fit_at_least(1);
  std::iota(memory_.begin(), memory_.end(), uint8_t{});

  // Act
  Memory<MemoryControlBlock, uint8_t> memory2;
  memory2.resize_to_fit_at_least(1);
  memory2 = memory_;

  // Assert
  EXPECT_NE(memory2, memory_);
  ASSERT_EQ(memory_.size(), memory2.size());
  EXPECT_TRUE(std::ranges::equal(memory2, memory_));
}

TEST_F(MemoryFixture, MoveConstructor) {
  // Arrange
  memory_.resize_to_fit_at_least(1);
  const auto memory_ptr = memory_.begin();
  const auto memory_size = memory_.size();

  // Act
  const auto memory2 = std::move(memory_);

  // Assert
  EXPECT_EQ(0U, memory_.size());
  EXPECT_EQ(nullptr, memory_);

  EXPECT_EQ(memory_ptr, memory2);
  EXPECT_EQ(memory_size, memory2.size());
}

TEST_F(MemoryFixture, MoveOperator) {
  // Arrange
  memory_.resize_to_fit_at_least(1);
  const auto memory_ptr = memory_.begin();
  const auto memory_size = memory_.size();

  // Act
  Memory<MemoryControlBlock, uint8_t> memory2;
  memory2.resize_to_fit_at_least(1);
  memory2 = std::move(memory_);

  // Assert
  EXPECT_EQ(0U, memory_.size());
  EXPECT_EQ(nullptr, memory_);

  EXPECT_EQ(memory_ptr, memory2);
  EXPECT_EQ(memory_size, memory2.size());
}

class SharedPtrFixture : public ::testing::Test {};

TEST_F(SharedPtrFixture, Empty) {
  // Arrange

  // Act
  const SharedPtr<uint8_t> ptr;

  // Assert
  EXPECT_TRUE(ptr.non_atomic_is_unique());
  EXPECT_EQ(0U, ptr.non_atomic_ref_count());
  EXPECT_EQ(nullptr, ptr.get());
}

TEST_F(SharedPtrFixture, ConstructorWithSize) {
  // Arrange

  // Act
  const SharedPtr<uint8_t> ptr(16);

  // Assert
  EXPECT_TRUE(ptr.non_atomic_is_unique());
  EXPECT_EQ(1U, ptr.non_atomic_ref_count());
  EXPECT_NE(nullptr, ptr.get());
}

TEST_F(SharedPtrFixture, NonAtomicReallocate) {
  // Arrange
  SharedPtr<uint8_t> ptr(16);

  // Act
  ptr.non_atomic_reallocate(32);
  ptr.get()[31] = '\x00';

  // Assert
  EXPECT_TRUE(ptr.non_atomic_is_unique());
  EXPECT_EQ(1U, ptr.non_atomic_ref_count());
}

TEST_F(SharedPtrFixture, CopyConstructor) {
  // Arrange
  const SharedPtr<uint8_t> ptr(16);

  // Act
  // NOLINTNEXTLINE(performance-unnecessary-copy-initialization)
  const auto ptr2 = ptr;

  // Assert
  EXPECT_FALSE(ptr.non_atomic_is_unique());
  EXPECT_FALSE(ptr2.non_atomic_is_unique());
  EXPECT_EQ(2U, ptr.non_atomic_ref_count());
  EXPECT_EQ(ptr.get(), ptr2.get());
}

TEST_F(SharedPtrFixture, CopyOperator) {
  // Arrange
  const SharedPtr<uint8_t> ptr(16);

  // Act
  SharedPtr<uint8_t> ptr2(16);
  ptr2 = ptr;

  // Assert
  EXPECT_FALSE(ptr.non_atomic_is_unique());
  EXPECT_FALSE(ptr2.non_atomic_is_unique());
  EXPECT_EQ(2U, ptr.non_atomic_ref_count());
  EXPECT_EQ(ptr.get(), ptr2.get());
}

TEST_F(SharedPtrFixture, MoveConstructor) {
  // Arrange
  SharedPtr<uint8_t> ptr(16);

  // Act
  const auto ptr2 = std::move(ptr);

  // Assert
  EXPECT_EQ(1U, ptr2.non_atomic_ref_count());
  EXPECT_NE(nullptr, ptr2.get());
}

TEST_F(SharedPtrFixture, MoveOperator) {
  // Arrange
  SharedPtr<uint8_t> ptr(16);
  const auto ptr_memory = ptr.get();

  // Act
  SharedPtr<uint8_t> ptr2(16);
  ptr2 = std::move(ptr);

  // Assert
  EXPECT_EQ(1U, ptr2.non_atomic_ref_count());
  EXPECT_EQ(ptr_memory, ptr2.get());
}

TEST_F(SharedPtrFixture, DestructItems) {
  // Arrange
  SharedPtr<std::string> ptr(1);

  // Act
  const auto string = std::construct_at(ptr.get());
  ptr.set_constructed_item_count(1);
  string->resize(32);

  // Assert
}

TEST_F(SharedPtrFixture, ReallocateAtExistingMemory) {
  // Arrange
  SharedPtr<std::string> ptr(1);

  // Act
  const auto string = std::construct_at(ptr.get());
  ptr.set_constructed_item_count(1);
  string->resize(32);

  ptr.non_atomic_reallocate(2);

  // Assert
  EXPECT_EQ(1U, ptr.constructed_item_count());
}

class SharedMemoryFixture : public ::testing::Test {
 protected:
  SharedMemory<uint8_t> memory_;
};

TEST_F(SharedMemoryFixture, Empty) {
  // Arrange

  // Act

  // Assert
  EXPECT_EQ(0U, memory_.size());
  EXPECT_EQ(nullptr, memory_);
}

TEST_F(SharedMemoryFixture, CopyConstructor) {
  // Arrange
  memory_.resize_to_fit_at_least(1);

  // Act
  const auto memory2 = memory_;

  // Assert
  EXPECT_EQ(memory2, memory_);
}

TEST_F(SharedMemoryFixture, CopyOperator) {
  // Arrange
  memory_.resize_to_fit_at_least(1);

  // Act
  SharedMemory<uint8_t> memory2;
  memory2.resize_to_fit_at_least(1);
  memory2 = memory_;

  // Assert
  EXPECT_EQ(memory2, memory_);
}

TEST_F(SharedMemoryFixture, MoveConstructor) {
  // Arrange
  memory_.resize_to_fit_at_least(1);
  const auto memory_ptr = memory_.begin();
  const auto memory_size = memory_.size();

  // Act
  const auto memory2 = std::move(memory_);

  // Assert
  EXPECT_EQ(0U, memory_.size());
  EXPECT_EQ(nullptr, memory_);

  EXPECT_EQ(memory_ptr, memory2);
  EXPECT_EQ(memory_size, memory2.size());
}

TEST_F(SharedMemoryFixture, MoveOperator) {
  // Arrange
  memory_.resize_to_fit_at_least(1);
  const auto memory_ptr = memory_.begin();
  const auto memory_size = memory_.size();

  // Act
  SharedMemory<uint8_t> memory2;
  memory2.resize_to_fit_at_least(1);
  memory2 = std::move(memory_);

  // Assert
  EXPECT_EQ(0U, memory_.size());
  EXPECT_EQ(nullptr, memory_);

  EXPECT_EQ(memory_ptr, memory2);
  EXPECT_EQ(memory_size, memory2.size());
}

TEST_F(SharedMemoryFixture, ResizeOnNonUniqueOwner) {
  // Arrange
  memory_.resize_to_fit_at_least(1);
  auto memory2 = memory_;

  // Act
  memory2.grow_to_fit_at_least(memory2.size() + 1);

  // Assert
  EXPECT_NE(memory_.size(), memory2.size());
  EXPECT_NE(memory_.begin(), memory2.begin());
}

}  // namespace