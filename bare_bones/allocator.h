#pragma once

#include <memory>

#include "preprocess.h"

namespace BareBones {

template <class T>
class Allocator {
 public:
  using value_type = T;

  explicit constexpr Allocator(size_t& allocated_memory) : allocated_memory_(allocated_memory) {}
  constexpr Allocator(const Allocator&) = default;
  template <class AnyType>
  explicit constexpr Allocator(const Allocator<AnyType>& other) : allocated_memory_(other.allocated_memory_) {}
  constexpr Allocator(Allocator&&) noexcept = default;

  constexpr Allocator& operator=(const Allocator&) = delete;
  constexpr Allocator& operator=(Allocator&&) noexcept = delete;
  constexpr bool operator==(const Allocator& other) const noexcept { return &allocated_memory_ == &other.allocated_memory_; };

  [[nodiscard]] PROMPP_ALWAYS_INLINE constexpr T* allocate(std::size_t n) {
    allocated_memory_ += n * sizeof(T);
    return std::allocator<T>{}.allocate(n);
  }
  PROMPP_ALWAYS_INLINE void deallocate(T* p, std::size_t n) {
    std::allocator<T>{}.deallocate(p, n);
    allocated_memory_ -= n * sizeof(T);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return allocated_memory_; }

 private:
  template <class AnyType>
  friend class Allocator;

  size_t& allocated_memory_;
};

}  // namespace BareBones
