#pragma once

#include <string_view>

#include "bare_bones/compiler.h"

#define XXH_INLINE_ALL
#include "xxHash/xxhash.h"

namespace BareBones {

class XXHash {
 public:
  template <class Number>
    requires std::is_arithmetic_v<Number>
  PROMPP_ALWAYS_INLINE void extend(Number value) noexcept {
    // if value here is a constant, compiler break the code with optimisations
    compiler::do_not_optimize(value);
    extend(&value, sizeof(value));
  }
  PROMPP_ALWAYS_INLINE void extend(const void* buffer, size_t size) noexcept { hash_ = XXH3_64bits_withSeed(buffer, size, hash_); }
  PROMPP_ALWAYS_INLINE void extend(std::string_view buffer) noexcept { extend(buffer.data(), buffer.size()); }
  PROMPP_ALWAYS_INLINE void extend(std::string_view label_name, std::string_view label_value) noexcept {
    hash_ = XXH3_64bits_withSeed(label_name.data(), label_name.size(), hash_) ^ XXH3_64bits_withSeed(label_value.data(), label_value.size(), hash_);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint64_t hash() const noexcept { return hash_; }
  [[nodiscard]] explicit PROMPP_ALWAYS_INLINE operator uint64_t() const noexcept { return hash_; }

  auto operator<=>(const XXHash& other) const noexcept = default;

  XXHash& operator=(uint64_t hash) noexcept {
    hash_ = hash;
    return *this;
  }

 private:
  uint64_t hash_{};
};

}  // namespace BareBones
